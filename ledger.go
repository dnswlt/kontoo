package kontoo

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

func DateVal(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
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
	var a *Asset
	found := false
	if e.AssetID != "" {
		a, found = s.assetMap[e.AssetID]
	} else {
		a, found = s.FindAssetByRef(e.AssetRef)
	}
	if !found {
		return fmt.Errorf("no asset found for AssetRef %q", e.AssetRef)
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
	if allZero(e.ValueMicros, e.QuantityMicros, e.PriceMicros) {
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
	}
	s.append(e)
	return nil
}

type AssetPosition struct {
	// If Asset is nil, AssetGroup must be populated.
	Asset           *Asset
	AssetGroup      *AssetGroup
	LastPriceUpdate Date
	LastValueUpdate Date
	PVal
}

func (s *Store) AssetPositionsAt(t time.Time) []*AssetPosition {
	byAsset := make(map[string][]*LedgerEntry)
	// Create sorted lists (ascending by ValueDate) per asset.
	for _, e := range s.L.Entries {
		if !e.ValueDate.After(t) {
			byAsset[e.AssetID] = append(byAsset[e.AssetID], e)
		}
	}
	for _, es := range byAsset {
		slices.SortFunc(es, func(a, b *LedgerEntry) int {
			return a.ValueDate.Time.Compare(b.ValueDate.Time)
		})
	}
	var res []*AssetPosition
	for assetId, es := range byAsset {
		pos := &AssetPosition{
			Asset: s.assetMap[assetId],
		}
		for _, e := range es {
			pos.Update(e)
		}
		res = append(res, pos)
	}
	return res
}

func (p *AssetPosition) Update(e *LedgerEntry) {
	switch e.Type {
	case BuyTransaction:
		p.QuantityMicros += e.QuantityMicros
		p.PriceMicros = e.PriceMicros
		p.LastPriceUpdate = e.ValueDate
	case SellTransaction:
		p.QuantityMicros -= e.QuantityMicros
		p.PriceMicros = e.PriceMicros
		p.LastPriceUpdate = e.ValueDate
	case AssetMaturity:
		p.ValueMicros = 0
		p.QuantityMicros = 0
		p.PriceMicros = 0
		p.LastValueUpdate = e.ValueDate
	case AssetPrice:
		p.PriceMicros = e.PriceMicros
		p.LastPriceUpdate = e.ValueDate
	case AccountBalance:
		p.ValueMicros = e.ValueMicros
		p.LastValueUpdate = e.ValueDate
	}
}

func (v *PVal) CalculatedValueMicros() Micros {
	if v.ValueMicros != 0 {
		return v.ValueMicros
	}
	return v.QuantityMicros.MulTrunc(v.PriceMicros)
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
