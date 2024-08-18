package kontoo

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

func DateVal(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}
func NewDate(year int, month time.Month, day int) *Date {
	return &Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

func (d *Date) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), "\"")
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return err
	}
	*d = Date{t}
	return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte("\"" + d.Format("2006-01-02") + "\""), nil
}

func (d Date) Equal(e Date) bool {
	return d.Time.Equal(e.Time)
}

type Store struct {
	L        *Ledger
	assetMap map[string]*Asset // Maps the ledger's assets by ID.
	path     string            // Path to the ledger JSON.
}

func (s *Store) BaseCurrency() Currency {
	return s.L.Header.BaseCurrency
}

func LoadStore(path string) (*Store, error) {
	l := &Ledger{}
	if err := l.Load(path); err != nil {
		return nil, fmt.Errorf("failed to load ledger: %w", err)
	}
	return NewStore(l, path)
}

func NewStore(ledger *Ledger, path string) (*Store, error) {
	m := make(map[string]*Asset)
	for _, asset := range ledger.Assets {
		id := asset.ID()
		if _, found := m[id]; found {
			return nil, fmt.Errorf("duplicate ID in ledger assets: %q", id)
		}
		m[asset.ID()] = asset
	}
	return &Store{
		L:        ledger,
		assetMap: m,
		path:     path,
	}, nil
}

func (s *Store) Save() error {
	return s.L.Save(s.path)
}

func (a *Asset) ID() string {
	if a.ISIN != "" {
		return a.ISIN
	}
	if a.IBAN != "" {
		return a.IBAN
	}
	if a.AccountNumber != "" {
		return a.AccountNumber
	}
	if a.WKN != "" {
		return a.WKN
	}
	if a.TickerSymbol != "" {
		return a.TickerSymbol
	}
	if a.CustomID != "" {
		return a.CustomID
	}
	return ""
}

func (a *Asset) MatchRef(ref string) bool {
	return a.IBAN == ref || a.ISIN == ref || a.WKN == ref ||
		a.AccountNumber == ref || a.TickerSymbol == ref ||
		a.ShortName == ref || a.Name == ref
}

func (s *Store) NextSequenceNum() int64 {
	if len(s.L.Entries) == 0 {
		return 0
	}
	return s.L.Entries[len(s.L.Entries)-1].SequenceNum + 1
}

func (s *Store) FindAssetByWKN(wkn string) (*Asset, bool) {
	if wkn == "" {
		return nil, false
	}
	for _, asset := range s.L.Assets {
		if asset.WKN == wkn {
			return asset, true
		}
	}
	return nil, false
}

func (s *Store) FindAssetByRef(ref string) (*Asset, bool) {
	var res *Asset
	for _, asset := range s.L.Assets {
		if asset.MatchRef(ref) {
			if res != nil {
				// Non-unique reference
				return nil, false
			}
			res = asset
		}
	}
	if res == nil {
		return nil, false
	}
	return res, true
}

func (s *Store) LookupAsset(e *LedgerEntry) (*Asset, bool) {
	if e.AssetID != "" {
		a, found := s.assetMap[e.AssetID]
		return a, found
	}
	return s.FindAssetByRef(e.AssetRef)
}

func (s *Store) FindAssetsForQuoteService(quoteService string) []*Asset {
	// TODO: only return assets still in possession at a given date.
	var assets []*Asset
	for _, a := range s.L.Assets {
		if _, ok := a.QuoteServiceSymbols[quoteService]; ok {
			assets = append(assets, a)
		}
	}
	return assets
}

func (s *Store) FindQuoteCurrencies() []Currency {
	var currencies []Currency
	seen := make(map[Currency]bool)
	for _, a := range s.L.Assets {
		if a.Currency == s.L.Header.BaseCurrency || seen[a.Currency] {
			continue
		}
		seen[a.Currency] = true
		currencies = append(currencies, a.Currency)
	}
	return currencies
}

func allZero(ms ...Micros) bool {
	for _, m := range ms {
		if m != 0 {
			return false
		}
	}
	return true
}

func (s *Store) append(e *LedgerEntry) {
	if e.Created.IsZero() {
		e.Created = time.Now()
	}
	e.SequenceNum = s.NextSequenceNum()
	s.L.Entries = append(s.L.Entries, e)
}

func (s *Store) Add(e *LedgerEntry) error {
	if e.ValueDate.IsZero() {
		return fmt.Errorf("ValueDate must be set")
	}
	if e.Type == ExchangeRate {
		if e.QuoteCurrency == "" {
			return fmt.Errorf("QuoteCurrency must not be empty")
		}
		if e.Currency == "" {
			e.Currency = s.L.Header.BaseCurrency
		}
		if e.PriceMicros == 0 {
			return fmt.Errorf("PriceMicros must be non-zero for ExchangeRate entry")
		}
		if !allZero(e.ValueMicros, e.CostMicros, e.QuantityMicros) {
			return fmt.Errorf("only PriceMicros may be specified for ExchangeRate entry")
		}
		s.append(e)
		return nil
	}
	if e.Type == UnspecifiedEntryType || !e.Type.Registered() {
		return fmt.Errorf("invalid EntryType: %q", e.Type)
	}
	// Must be an entry that refers to an asset.
	a, found := s.LookupAsset(e)
	if !found {
		return fmt.Errorf("no asset found: {AssetID:%q, AssetRef:%q})", e.AssetID, e.AssetRef)
	}
	if e.Currency != "" && e.Currency != a.Currency {
		return fmt.Errorf("wrong currency (%s) for asset %s (want: %s)", e.Currency, a.ID(), a.Currency)
	} else if e.Currency == "" {
		e.Currency = a.Currency
	}
	// Change soft-link to ID ref:
	e.AssetRef, e.AssetID = "", a.ID()
	// General validation
	if e.QuoteCurrency != "" {
		return fmt.Errorf("QuoteCurrency must only be specified for ExchangeRate entry, not %q", e.Type)
	}
	if e.PriceMicros < 0 {
		return fmt.Errorf("PriceMicros must not be negative")
	}
	if allZero(e.ValueMicros, e.QuantityMicros, e.PriceMicros) && e.Type != AssetMaturity {
		return fmt.Errorf("entry must have at least one non-zero value")
	}
	switch e.Type {
	case BuyTransaction, SellTransaction:
		if e.PriceMicros == 0 {
			return fmt.Errorf("PriceMicros must be specified for %s", e.Type)
		}
		if e.QuantityMicros == 0 {
			return fmt.Errorf("QuantityMicros must be specified for %s", e.Type)
		}
	case AssetPrice:
		if e.PriceMicros == 0 {
			return fmt.Errorf("PriceMicros must be specified for %s", e.Type)
		}
	case AccountBalance:
		if e.PriceMicros != 0 || e.QuantityMicros != 0 {
			return fmt.Errorf("PriceMicros and QuantityMicros must both be 0, was (%v, %v)", e.PriceMicros, e.QuantityMicros)
		}
	case AssetHolding:
		if e.QuantityMicros == 0 || e.PriceMicros == 0 {
			return fmt.Errorf("QuantityMicros and PriceMicros must be specified for %s", e.Type)
		}
	case InterestPayment, DividendPayment:
		if e.ValueMicros == 0 {
			return fmt.Errorf("ValueMicros must be specified for %s", e.Type)
		}
		if e.PriceMicros != 0 || e.QuantityMicros != 0 {
			return fmt.Errorf("PriceMicros and QuantityMicros must both be 0, was (%v, %v)", e.PriceMicros, e.QuantityMicros)
		}
	}
	s.append(e)
	return nil
}

func (s *Store) AddAsset(a *Asset) error {
	id := a.ID()
	if id == "" {
		return fmt.Errorf("Asset must have an ID")
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("Asset name must not be empty")
	}
	if a.MaturityDate != nil && a.MaturityDate.IsZero() {
		return fmt.Errorf("MaturityDate must be nil or non-zero")
	}
	if a.IssueDate != nil && a.IssueDate.IsZero() {
		return fmt.Errorf("IssueDate must be nil or non-zero")
	}
	if a.MaturityDate != nil && a.IssueDate != nil && a.MaturityDate.Before(a.IssueDate.Time) {
		return fmt.Errorf("MaturityDate must not be before IssueDate")
	}
	if ok, _ := regexp.MatchString("^[A-Z]{3}$", string(a.Currency)); !ok {
		return fmt.Errorf("Currency must use ISO code (3 uppercase letters)")
	}
	if _, ok := s.assetMap[id]; ok {
		return fmt.Errorf("duplicate asset ID %q", id)
	}
	s.assetMap[id] = a
	s.L.Assets = append(s.L.Assets, a)
	return nil
}

// AssetPositionItem tracks an individual purchase that is part of the
// accumulated asset position.
type AssetPositionItem struct {
	QuantityMicros Micros
	PriceMicros    Micros
	CostMicros     Micros
}

// AssetPosition represents the "current" asset position.
// It is typically calculated from ledger entries for the given asset.
type AssetPosition struct {
	Asset           *Asset
	LastPriceUpdate Date
	LastValueUpdate Date
	ValueMicros     Micros
	QuantityMicros  Micros
	PriceMicros     Micros
	// Items are the constituent parts of the accumulated asset position.
	// The are stored in chronological order (latest comes last) and can
	// be used to determine profits and losses (P&L) and to update the
	// accumulated values when an asset is partially sold.
	Items []AssetPositionItem
}

func (s *Store) AssetPositionsAt(t time.Time) []*AssetPosition {
	byAsset := make(map[string][]*LedgerEntry)
	// Create sorted lists (ascending by ValueDate) per asset.
	for _, e := range s.L.Entries {
		if e.AssetID == "" {
			// Ignore e.g. ExchangeRate
			continue
		}
		if !e.ValueDate.After(t) {
			byAsset[e.AssetID] = append(byAsset[e.AssetID], e)
		}
	}
	for _, es := range byAsset {
		slices.SortFunc(es, func(a, b *LedgerEntry) int {
			return a.ValueDate.Time.Compare(b.ValueDate.Time)
		})
	}
	// Calculate position values at date.
	var res []*AssetPosition
	for assetId, es := range byAsset {
		asset, ok := s.assetMap[assetId]
		if !ok {
			log.Fatalf("Program error: ledger entry with invalid AssetId: %q", assetId)
		}
		pos := &AssetPosition{
			Asset: asset,
		}
		for _, e := range es {
			pos.Update(e)
		}
		if pos.CalculatedValueMicros() != 0 {
			res = append(res, pos)
		}
	}
	return res
}

func (p *AssetPosition) Name() string {
	return p.Asset.Name
}
func (p *AssetPosition) Currency() Currency {
	return p.Asset.Currency
}

func (p *AssetPosition) Update(e *LedgerEntry) {
	switch e.Type {
	case BuyTransaction:
		p.QuantityMicros += e.QuantityMicros
		p.PriceMicros = e.PriceMicros
		p.LastPriceUpdate = e.ValueDate
		p.Items = append(p.Items, AssetPositionItem{
			QuantityMicros: e.QuantityMicros,
			PriceMicros:    e.PriceMicros,
			CostMicros:     e.CostMicros,
		})
	case SellTransaction:
		p.QuantityMicros -= e.QuantityMicros
		p.PriceMicros = e.PriceMicros
		p.LastPriceUpdate = e.ValueDate
		// Remove items
		qty := e.QuantityMicros
		for len(p.Items) > 0 {
			hd := &p.Items[0]
			if hd.QuantityMicros > qty {
				oldQ := hd.QuantityMicros
				hd.QuantityMicros -= qty
				hd.CostMicros = hd.CostMicros.Frac(hd.QuantityMicros, oldQ)
				break
			}
			qty -= hd.QuantityMicros
			p.Items = p.Items[1:]
		}
		if len(p.Items) == 0 {
			p.Items = nil // allow GC of Items
		}
	case AssetMaturity:
		p.ValueMicros = 0
		p.QuantityMicros = 0
		p.PriceMicros = 0
		p.LastValueUpdate = e.ValueDate
		p.Items = nil
	case AssetPrice:
		p.PriceMicros = e.PriceMicros
		p.LastPriceUpdate = e.ValueDate
	case AccountBalance:
		p.ValueMicros = e.ValueMicros
		p.LastValueUpdate = e.ValueDate
	case AssetHolding:
		if e.PriceMicros != 0 {
			p.PriceMicros = e.PriceMicros
			p.LastPriceUpdate = e.ValueDate
		}
		if e.QuantityMicros != p.QuantityMicros {
			// Only update position if the quantity has changed,
			// otherwise consider it an informational ledger entry.
			p.ValueMicros = e.ValueMicros
			p.QuantityMicros = e.QuantityMicros
			p.LastValueUpdate = e.ValueDate
			p.Items = nil
			if e.QuantityMicros > 0 {
				p.Items = append(p.Items, AssetPositionItem{
					QuantityMicros: e.QuantityMicros,
					PriceMicros:    e.PriceMicros,
					CostMicros:     e.CostMicros,
				})
			}
		}
	}
}

func (p *AssetPosition) CostMicros() Micros {
	var cost Micros
	for _, item := range p.Items {
		cost += item.CostMicros
	}
	return cost
}

func (p *AssetPosition) PurchasePrice() Micros {
	var price Micros
	for _, item := range p.Items {
		price += item.CostMicros
		price += item.QuantityMicros.Mul(item.PriceMicros)
	}
	return price
}

func (p *AssetPosition) CalculatedValueMicros() Micros {
	if p.QuantityMicros != 0 {
		return p.QuantityMicros.Mul(p.PriceMicros)
	}
	return p.ValueMicros
}

func (l *Ledger) Save(path string) error {
	data, err := l.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (l *Ledger) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, l)
}

func (l *Ledger) Marshal() ([]byte, error) {
	return json.MarshalIndent(l, "", "  ")
}
