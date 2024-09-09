package kontoo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

func DateVal(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}
func NewDate(year int, month time.Month, day int) *Date {
	return &Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}
func today() Date {
	return toDate(time.Now())
}
func toDate(t time.Time) Date {
	y, m, d := t.Date()
	return DateVal(y, m, d)
}
func utcDate(date time.Time) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, err
	}
	return Date{t}, nil
}

func (d *Date) UnmarshalJSON(data []byte) error {
	var err error
	s := strings.Trim(string(data), "\"")
	*d, err = ParseDate(s)
	if err != nil {
		return err
	}
	return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte("\"" + d.Format("2006-01-02") + "\""), nil
}

func (d Date) Equal(e Date) bool {
	return d.Time.Equal(e.Time)
}

type Store struct {
	ledger        *Ledger
	path          string                      // Path to the ledger JSON.
	assets        map[string]*Asset           // Maps the ledger's assets by ID.
	entries       map[string][]*LedgerEntry   // Entries by asset ID, ordered chronologically.
	exchangeRates map[Currency][]*LedgerEntry // Exchange rates from Base Currency to other currencies, ordered chronologically
}

func (s *Store) BaseCurrency() Currency {
	return s.ledger.Header.BaseCurrency
}

func (s *Store) ValueDateRange() (min, max Date) {
	for _, e := range s.ledger.Entries {
		if e.ValueDate.After(max.Time) {
			max = e.ValueDate
		}
		if min.IsZero() || e.ValueDate.Before(min.Time) {
			min = e.ValueDate
		}
	}
	return
}

// ExchangeRateAt returns the BaseCurrency/QuoteCurrency exchange rate at the given time.
// A value of 1.50 means that for 1 BaseCurrency you get 1.50 QuoteCurrency c.
// The rate is derived from ExchangeRate entries in the ledger; the most recent rate
// before t is used and its date is returned as the second return value.
// If no exchange rate between the given currency c and the base currency is known at t,
// an error is returned.
func (s *Store) ExchangeRateAt(c Currency, t Date) (Micros, Date, error) {
	rs, ok := s.exchangeRates[c]
	if !ok || len(rs) == 0 {
		return 0, Date{}, fmt.Errorf("no exchange rates for currency %s", c)
	}
	i := sort.Search(len(rs), func(i int) bool {
		return rs[i].ValueDate.Compare(t) > 0
	})
	if i == 0 {
		return 0, Date{}, fmt.Errorf("no exchange rates for currency %s at %v", c, t)
	}
	return rs[i-1].PriceMicros, rs[i-1].ValueDate, nil
}

func NewStore(ledger *Ledger, path string) (*Store, error) {
	// Ensure header is non-nil to avoid nil checks elsewhere.
	if ledger.Header == nil {
		ledger.Header = &LedgerHeader{}
	}
	s := &Store{
		ledger:        ledger,
		path:          path,
		entries:       make(map[string][]*LedgerEntry),
		assets:        make(map[string]*Asset),
		exchangeRates: make(map[Currency][]*LedgerEntry),
	}
	// Build asset index.
	for _, asset := range ledger.Assets {
		id := asset.ID()
		if _, found := s.assets[id]; found {
			return nil, fmt.Errorf("duplicate ID in ledger assets: %q", id)
		}
		s.assets[asset.ID()] = asset
	}
	// Validate ledger entries and add to asset-keyed index.
	seqNums := make(map[int64]bool)
	for _, e := range ledger.Entries {
		if seqNums[e.SequenceNum] {
			return nil, fmt.Errorf("invalid ledger: duplicate sequence number: %d", e.SequenceNum)
		}
		seqNums[e.SequenceNum] = true
		if e.AssetID == "" && e.Type.NeedsAssetID() {
			return nil, fmt.Errorf("invalid ledger: entry #%d has no AssetID", e.SequenceNum)
		}
		if err := s.validateEntry(e); err != nil {
			return nil, fmt.Errorf("invalid ledger entry: %w", err)
		}
		if e.AssetID != "" {
			s.entries[e.AssetID] = append(s.entries[e.AssetID], e)
		}
	}
	// Sort entries map values chronologically.
	for k := range s.entries {
		slices.SortFunc(s.entries[k], cmpLedgerEntry)
	}
	// Build exchange rates (base => quote currency) map.
	baseCurrency := ledger.Header.BaseCurrency
	for _, e := range ledger.Entries {
		if e.Type == ExchangeRate && e.Currency == baseCurrency {
			s.exchangeRates[e.QuoteCurrency] = append(s.exchangeRates[e.QuoteCurrency], e)
		}
	}
	for k := range s.exchangeRates {
		slices.SortFunc(s.exchangeRates[k], func(a, b *LedgerEntry) int {
			return a.ValueDate.Compare(b.ValueDate)
		})
	}
	return s, nil
}

// LedgerRecord is a wrapper for storing a ledger
// in a file, row by row, instead of as a single record.
// Only one of its fields may be set.
// Header must be the first entry in the file,
// assets and entries can then be mixed arbitrarily.
type LedgerRecord struct {
	Header *LedgerHeader `json:",omitempty"`
	Entry  *LedgerEntry  `json:",omitempty"`
	Asset  *Asset        `json:",omitempty"`
}

func LoadStore(path string) (*Store, error) {
	var l Ledger
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer f.Close()

	ext := filepath.Ext(path)
	if ext == ".json" {
		// Stored as a single Ledger JSON record
		err = json.NewDecoder(f).Decode(&l)
		if err != nil {
			return nil, err
		}
		return NewStore(&l, path)
	}
	// Stored as a sequence of LedgerRecords.
	dec := json.NewDecoder(f)
	for i := 0; ; i++ {
		var rec LedgerRecord
		err := dec.Decode(&rec)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return nil, err
			}
			// We reached the end of the input.
			return NewStore(&l, path)
		}
		if rec.Header != nil {
			if i > 0 {
				return nil, fmt.Errorf("invalid ledger %q: header as record #%d", path, i)
			}
			l.Header = rec.Header
		} else if rec.Asset != nil {
			l.Assets = append(l.Assets, rec.Asset)
		} else if rec.Entry != nil {
			l.Entries = append(l.Entries, rec.Entry)
		} else {
			return nil, fmt.Errorf("invalid ledger %q: empty record", path)
		}
	}
}

func (s *Store) Save() error {
	f, err := os.Create(s.path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if filepath.Ext(s.path) == ".json" {
		// Store as single record
		return enc.Encode(s.ledger)
	}
	// Store as sequence of LedgerRecord
	l := s.ledger
	if l.Header != nil {
		if err := enc.Encode(LedgerRecord{
			Header: l.Header,
		}); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}
	}
	for _, a := range l.Assets {
		if err := enc.Encode(LedgerRecord{
			Asset: a,
		}); err != nil {
			return fmt.Errorf("failed to write asset: %w", err)
		}
	}
	for _, e := range l.Entries {
		if err := enc.Encode(LedgerRecord{
			Entry: e,
		}); err != nil {
			return fmt.Errorf("failed to write ledger entry: %w", err)
		}
	}
	return nil
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

func (a *Asset) matchRef(ref string) bool {
	return a.IBAN == ref || a.ISIN == ref || a.WKN == ref ||
		a.AccountNumber == ref || a.TickerSymbol == ref ||
		a.ShortName == ref || a.Name == ref
}

func (s *Store) nextSequenceNum() int64 {
	if len(s.ledger.Entries) == 0 {
		return 0
	}
	return s.ledger.Entries[len(s.ledger.Entries)-1].SequenceNum + 1
}

func (s *Store) FindAssetByWKN(wkn string) *Asset {
	if wkn == "" {
		return nil
	}
	for _, asset := range s.ledger.Assets {
		if asset.WKN == wkn {
			return asset
		}
	}
	return nil
}

func (s *Store) FindAssetByRef(ref string) *Asset {
	var res *Asset
	for _, asset := range s.ledger.Assets {
		if asset.matchRef(ref) {
			if res != nil {
				// Non-unique reference
				return nil
			}
			res = asset
		}
	}
	return res
}

func (s *Store) FindAssetsForQuoteService(quoteService string) []*Asset {
	// TODO: only return assets still in possession at a given date.
	var assets []*Asset
	for _, a := range s.ledger.Assets {
		if _, ok := a.QuoteServiceSymbols[quoteService]; ok {
			assets = append(assets, a)
		}
	}
	return assets
}

func (s *Store) QuoteCurrencies() []Currency {
	var currencies []Currency
	seen := make(map[Currency]bool)
	for _, a := range s.ledger.Assets {
		if a.Currency == s.ledger.Header.BaseCurrency || seen[a.Currency] {
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

// insert inserts the already validated entry e into the store.
// It sets the entry's created date and sequence number
// and updates internal ledger and all relevant indexes.
func (s *Store) insert(e *LedgerEntry) {
	if e.Created.IsZero() {
		e.Created = time.Now()
	}
	e.SequenceNum = s.nextSequenceNum()
	s.ledger.Entries = append(s.ledger.Entries, e)
	ins := func(es []*LedgerEntry, e *LedgerEntry) []*LedgerEntry {
		l := len(es)
		es = append(es, e)
		i := sort.Search(l, func(i int) bool {
			return es[i].ValueDate.Compare(e.ValueDate) > 0
		})
		if i < l {
			copy(es[i+1:], es[i:l])
			es[i] = e
		}
		return es
	}
	if e.Type == ExchangeRate {
		// Insert rate, maintain chronological order.
		s.exchangeRates[e.QuoteCurrency] = ins(s.exchangeRates[e.QuoteCurrency], e)
	} else {
		// Insert asset-based entry, maintain chronological order.
		s.entries[e.AssetID] = ins(s.entries[e.AssetID], e)
	}
}

func (s *Store) validateEntry(e *LedgerEntry) error {
	if e.ValueDate.IsZero() {
		return fmt.Errorf("ValueDate must be set")
	}
	if e.Type == ExchangeRate {
		if e.QuoteCurrency == "" {
			return fmt.Errorf("QuoteCurrency must not be empty")
		}
		if e.Currency == "" {
			e.Currency = s.ledger.Header.BaseCurrency
		}
		if e.PriceMicros == 0 {
			return fmt.Errorf("PriceMicros must be non-zero for ExchangeRate entry")
		}
		if !allZero(e.ValueMicros, e.CostMicros, e.QuantityMicros) {
			return fmt.Errorf("only PriceMicros may be specified for ExchangeRate entry")
		}
		return nil
	}
	if e.Type == UnspecifiedEntryType || !e.Type.Registered() {
		return fmt.Errorf("invalid EntryType: %v", e.Type)
	}
	// Must be an entry that refers to an asset.
	if !e.Type.NeedsAssetID() {
		log.Fatalf("Program error: unhandled non-asset-based entry type: %v", e.Type)
	}
	a, found := s.assets[e.AssetID]
	if !found {
		return fmt.Errorf("no asset found with AssetID=%q", e.AssetID)
	}
	if !slices.Contains(a.Type.ValidEntryTypes(), e.Type) {
		return fmt.Errorf("%v is not a valid entry type for an asset of type %v", e.Type, a.Type)
	}
	if e.Currency != "" && e.Currency != a.Currency {
		return fmt.Errorf("wrong currency (%s) for asset %s (want: %s)", e.Currency, a.ID(), a.Currency)
	} else if e.Currency == "" {
		e.Currency = a.Currency
	}
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
	// Type-specific validation
	switch e.Type {
	case AssetPurchase, AssetSale:
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
			return fmt.Errorf("PriceMicros and QuantityMicros must be 0 for %v, was (%v, %v)",
				e.Type, e.PriceMicros, e.QuantityMicros)
		}
	case AccountCredit, AccountDebit:
		if e.PriceMicros != 0 || e.QuantityMicros != 0 {
			return fmt.Errorf("PriceMicros and QuantityMicros must be 0 for %v, was (%v, %v)",
				e.Type, e.PriceMicros, e.QuantityMicros)
		}
		if e.Type == AccountCredit && e.ValueMicros <= 0 {
			return fmt.Errorf("ValueMicros must be positive for %v, was %v", e.Type, e.ValueMicros)
		} else if e.Type == AccountDebit && e.ValueMicros >= 0 {
			return fmt.Errorf("ValueMicros must be negative for %v, was %v", e.Type, e.ValueMicros)
		}
	case AssetHolding:
		if e.QuantityMicros == 0 || e.PriceMicros == 0 {
			return fmt.Errorf("QuantityMicros and PriceMicros must be specified for %s", e.Type)
		}
	case AssetMaturity:
		// ValueMicros is allowed to specify the final account balance, e.g. for fixed deposit accounts.
		if !allZero(e.QuantityMicros, e.PriceMicros, e.CostMicros) {
			return fmt.Errorf("QuantityMicros, PriceMicros, CostMicros should be zero for %s", e.Type)
		}
	case InterestPayment, DividendPayment:
		if e.ValueMicros == 0 {
			return fmt.Errorf("ValueMicros must be specified for %s", e.Type)
		}
		if e.PriceMicros != 0 || e.QuantityMicros != 0 {
			return fmt.Errorf("PriceMicros and QuantityMicros must both be 0, was (%v, %v)", e.PriceMicros, e.QuantityMicros)
		}
	}

	return nil
}

// Add validates the given entry e and, on successful validation, inserts
// the entry into the store.
func (s *Store) Add(e *LedgerEntry) error {
	if e.Type.NeedsAssetID() && e.AssetID == "" {
		// Change ref-link to ID if necessary:
		a := s.FindAssetByRef(e.AssetRef)
		if a == nil {
			return fmt.Errorf("no asset found with ref=%q", e.AssetRef)
		}
		e.AssetRef, e.AssetID = "", a.ID()
	}
	if err := s.validateEntry(e); err != nil {
		return fmt.Errorf("entry validation failed: %w", err)
	}
	s.insert(e)
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
	if len(a.Currency) == 0 {
		return fmt.Errorf("Currency must not be empty")
	}
	if ok, _ := regexp.MatchString("^[A-Z]{3}$", string(a.Currency)); !ok {
		return fmt.Errorf("Currency must use ISO code (3 uppercase letters)")
	}
	if _, ok := s.assets[id]; ok {
		return fmt.Errorf("duplicate asset ID %q", id)
	}
	s.assets[id] = a
	s.ledger.Assets = append(s.ledger.Assets, a)
	return nil
}

// AssetPositionItem tracks an individual purchase that is part of the
// accumulated asset position.
type AssetPositionItem struct {
	ValueDate      Date
	QuantityMicros Micros
	PriceMicros    Micros
	CostMicros     Micros
	// Used for accounts that track individual inpayment/outflow events.
	// Negative values represent outflows.
	TransactionValueMicros Micros
}

// AssetPosition represents the "current" asset position.
// It is typically calculated from ledger entries for the given asset.
type AssetPosition struct {
	Asset *Asset
	// Last value date of any ledger entry seen for this position.
	LastUpdated    Date
	ValueMicros    Micros
	QuantityMicros Micros
	PriceMicros    Micros
	// Items are the constituent parts of the accumulated asset position.
	// The are stored in chronological order (latest comes last) and can
	// be used to determine profits and losses (P&L) and to update the
	// accumulated values when an asset is partially sold.
	Items []AssetPositionItem
}

func cmpLedgerEntry(a, b *LedgerEntry) int {
	c := a.ValueDate.Compare(b.ValueDate)
	if c != 0 {
		return c
	}
	return int(a.SequenceNum - b.SequenceNum)
}

// AssetPositionsBetween returns all asset positions for assetID
// on days with ledger entries between start and end.
func (s *Store) AssetPositionsBetween(assetID string, start, end Date) []*AssetPosition {
	asset, ok := s.assets[assetID]
	if !ok {
		return nil
	}
	entries := s.entries[assetID]
	var res []*AssetPosition
	pos := &AssetPosition{
		Asset: asset,
	}
	for _, e := range entries {
		if e.ValueDate.After(end.Time) {
			break
		}
		pos.Update(e)
		if !e.ValueDate.Before(start.Time) {
			res = append(res, pos.Copy())
		}
	}
	return res
}

func (s *Store) AssetPositionAt(assetId string, date Date) *AssetPosition {
	asset, ok := s.assets[assetId]
	if !ok {
		return nil
	}
	pos := &AssetPosition{
		Asset: asset,
	}
	for _, e := range s.entries[assetId] {
		if e.ValueDate.After(date.Time) {
			break
		}
		pos.Update(e)
	}
	return pos
}

// AssetPositionsAt returns the asset positions for each non-zero asset position at t.
func (s *Store) AssetPositionsAt(date Date) []*AssetPosition {
	// Calculate position values at date.
	var res []*AssetPosition
	for assetId := range s.assets {
		pos := s.AssetPositionAt(assetId, date)
		if pos.MarketValue() != 0 {
			res = append(res, pos)
		}
	}
	return res
}

// Copy returns a semi-deep copy of p: It shares the pointer to the asset,
// but all other, position-specific values are copied.
func (p *AssetPosition) Copy() *AssetPosition {
	q := *p
	items := make([]AssetPositionItem, len(p.Items))
	copy(items, p.Items)
	q.Items = items
	return &q
}

func (p *AssetPosition) Name() string {
	return p.Asset.Name
}
func (p *AssetPosition) Currency() Currency {
	return p.Asset.Currency
}

func (p *AssetPosition) Update(e *LedgerEntry) {
	p.LastUpdated = e.ValueDate
	switch e.Type {
	case AssetPurchase:
		p.QuantityMicros += e.QuantityMicros
		p.PriceMicros = e.PriceMicros
		p.Items = append(p.Items, AssetPositionItem{
			ValueDate:      e.ValueDate,
			QuantityMicros: e.QuantityMicros,
			PriceMicros:    e.PriceMicros,
			CostMicros:     e.CostMicros,
		})
	case AssetSale:
		p.QuantityMicros -= e.QuantityMicros
		p.PriceMicros = e.PriceMicros
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
		p.Items = nil
	case AssetPrice:
		p.PriceMicros = e.PriceMicros
	case AccountCredit, AccountDebit:
		p.ValueMicros += e.ValueMicros
		// In a "normal" account, we don't keep track of individual credit/debit
		// transactions in the AssetPosition, since we only care about the account
		// balance. For accounts like FixedDepositAccount or PensionAccount, we
		// do care about individual credits (and debits, though those are not typical)
		if p.Asset.Type.UseTransactionTracking() {
			p.Items = append(p.Items, AssetPositionItem{
				ValueDate:              e.ValueDate,
				TransactionValueMicros: e.ValueMicros,
			})
		}
	case AccountBalance:
		p.ValueMicros = e.ValueMicros
		// Items are not influenced by AccountBalance entries, not even for
		// those with transaction tracking. Credit/Debit entries must be used
		// to keep track of all inpayments/outflows.
	case AssetHolding:
		if e.PriceMicros != 0 {
			p.PriceMicros = e.PriceMicros
		}
		if e.QuantityMicros != p.QuantityMicros {
			// Only update position if the quantity has changed,
			// otherwise consider it an informational ledger entry.
			p.QuantityMicros = e.QuantityMicros
			p.ValueMicros = e.ValueMicros
			p.Items = nil
			if e.QuantityMicros > 0 {
				p.Items = append(p.Items, AssetPositionItem{
					ValueDate:      e.ValueDate,
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
	if p.Asset.Type.UseTransactionTracking() {
		for _, item := range p.Items {
			price += item.TransactionValueMicros
		}
	} else {
		// Price*qty based pricing.
		for _, item := range p.Items {
			price += item.CostMicros
			price += item.QuantityMicros.Mul(item.PriceMicros)
		}
	}
	return price
}

func (p *AssetPosition) MarketValue() Micros {
	if p.QuantityMicros != 0 {
		return p.QuantityMicros.Mul(p.PriceMicros)
	}
	return p.ValueMicros
}

// totalEarningsAtMaturity calculates the predicted earnings of a fixed-income
// asset position by its maturity date. These earnings include both interest
// and capital gains (or losses) from price appreciation or depreciation.
// It assumes the asset matures at 100% of its nominal value.
func totalEarningsAtMaturity(p *AssetPosition) Micros {
	md := p.Asset.MaturityDate
	if md == nil {
		return 0
	}
	interestRate := p.Asset.InterestMicros
	var interest, gains Micros
	for _, item := range p.Items {
		years := md.Sub(item.ValueDate.Time).Hours() / 24 / 365
		var value Micros
		if p.Asset.Type.UseTransactionTracking() {
			value = item.TransactionValueMicros
			// No capital gains for fixed deposit.
		} else {
			value = item.QuantityMicros
			// Capital gains are nominal value (i.e. price at 100%) minus invested sum.
			gains += item.QuantityMicros - item.QuantityMicros.Mul(item.PriceMicros) - item.CostMicros
		}
		switch p.Asset.InterestPayment {
		case AccruedPayment:
			interest += value.Mul(FloatAsMicros(math.Pow(1+interestRate.Float(), years))) - value
		case AnnualPayment:
			interest += value.Mul(interestRate).Mul(FloatAsMicros(years))
		default:
			// If no payment schedule is specified, we can't calculate the interest
		}
	}
	return interest + gains
}
