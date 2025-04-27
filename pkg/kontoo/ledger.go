package kontoo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

func DateVal(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}
func newDate(year int, month time.Month, day int) *Date {
	return &Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}
func today() Date {
	return ToDate(time.Now())
}
func ToDate(t time.Time) Date {
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

// NewLedger returns an empty ledger that uses the given base currency.
func NewLedger(baseCurrency Currency) *Ledger {
	return &Ledger{
		Header: &LedgerHeader{
			BaseCurrency: baseCurrency,
		},
	}
}

type LedgerEntryRow struct {
	E *LedgerEntry
	A *Asset
	// Asset position values after incorporating ledger entry E.
	totalMarketValue Micros
	totalQuantity    Micros
	totalCost        Micros
}

func (e *LedgerEntryRow) SequenceNum() int64 {
	return e.E.SequenceNum
}
func (e *LedgerEntryRow) ValueDate() Date {
	return e.E.ValueDate
}
func (e *LedgerEntryRow) Created() time.Time {
	return e.E.Created
}

func (e *LedgerEntryRow) EntryType() EntryType {
	return e.E.Type

}
func (e *LedgerEntryRow) HasAsset() bool {
	return e.A != nil
}
func (e *LedgerEntryRow) AssetID() string {
	if e.A == nil {
		return ""
	}
	return e.A.ID()
}
func (e *LedgerEntryRow) AssetName() string {
	if e.A == nil {
		return ""
	}
	return e.A.Name
}
func (e *LedgerEntryRow) Label() string {
	if e.HasAsset() {
		return e.AssetName()
	}
	if e.E.Type == ExchangeRate {
		return string(e.E.Currency) + "/" + string(e.E.QuoteCurrency)
	}
	return ""
}
func (e *LedgerEntryRow) AssetType() AssetType {
	if e.A == nil {
		return UnspecifiedAssetType
	}
	return e.A.Type
}
func (e *LedgerEntryRow) Currency() string {
	return string(e.E.Currency)
}
func (e *LedgerEntryRow) Value() Micros {
	return e.E.ValueMicros
}

// EntryMarketValue returns the market value of the ledger entry.
// Use PositionMarketValue to obtain the position's market value.
func (e *LedgerEntryRow) EntryMarketValue() Micros {
	if e.E.ValueMicros != 0 {
		return e.E.ValueMicros
	}
	return e.E.QuantityMicros.Mul(e.E.PriceMicros)
}
func (e *LedgerEntryRow) Cost() Micros {
	return e.E.CostMicros
}
func (e *LedgerEntryRow) Quantity() Micros {
	return e.E.QuantityMicros
}
func (e *LedgerEntryRow) Price() Micros {
	return e.E.PriceMicros
}
func (e *LedgerEntryRow) Comment() string {
	return e.E.Comment
}
func (e *LedgerEntryRow) TotalMarketValue() Micros {
	return e.totalMarketValue
}
func (e *LedgerEntryRow) TotalQuantity() Micros {
	return e.totalQuantity
}
func (e *LedgerEntryRow) TotalCost() Micros {
	return e.totalCost
}

type Store struct {
	ledger        *Ledger
	path          string                      // Path to the ledger JSON.
	assets        map[string]*Asset           // Maps the ledger's assets by ID.
	entries       map[string][]*LedgerEntry   // Entries by asset ID, ordered chronologically.
	exchangeRates map[Currency][]*LedgerEntry // Exchange rates from Base Currency to other currencies, ordered chronologically
	// Cache for already seen time zone names.
	// Time zones are checked during ledger validation, so we don't want to re-load them from disk for each asset.
	timezones map[string]*time.Location
	mut       sync.Mutex
}

func (s *Store) BaseCurrency() Currency {
	return s.ledger.Header.BaseCurrency
}

func (s *Store) timezone(tz string) (*time.Location, error) {
	s.mut.Lock()
	defer s.mut.Unlock()
	if tz == "" {
		// Default time zone: NYC, the center of the financial world.
		tz = "America/New_York"
	}
	if loc, ok := s.timezones[tz]; ok {
		return loc, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid exchange timezone: %w", err)
	}
	s.timezones[tz] = loc
	return loc, nil
}

func (s *Store) allEntryRows() []*LedgerEntryRow {
	// This currently takes <1ms for a 5y ledger. For huge ledgers we might want to cache the calculation.
	allRows := make([]*LedgerEntryRow, 0, len(s.ledger.Entries))
	for assetID, es := range s.entries {
		asset := s.assets[assetID]
		pos := &AssetPosition{
			Asset: asset,
		}
		var totalCost Micros
		for _, e := range es {
			pos.Update(e)
			totalCost += e.CostMicros
			allRows = append(allRows, &LedgerEntryRow{
				A:                asset,
				E:                e,
				totalMarketValue: pos.MarketValue(),
				totalQuantity:    pos.QuantityMicros,
				totalCost:        totalCost,
			})
		}
	}
	for _, er := range s.exchangeRates {
		for _, e := range er {
			allRows = append(allRows, &LedgerEntryRow{
				E: e,
			})
		}
	}
	if len(allRows) != len(s.ledger.Entries) {
		log.Fatalf("All rows (%d) not equal to number of ledger entries (%d)", len(allRows), len(s.ledger.Entries))
	}
	return allRows
}

// LedgerEntryRows returns all ledger entries matching the given query.
func (s *Store) LedgerEntryRows(query *Query) []*LedgerEntryRow {
	// Calculate all positions.
	var res []*LedgerEntryRow
	for _, r := range s.allEntryRows() {
		if !query.Match(r) {
			continue
		}
		res = append(res, r)
	}
	query.Sort(res)
	res = query.LimitGroups(res)
	return res
}

// ValueDateRange returns the minimum and maximum value date of any ledger entry in the store.
func (s *Store) ValueDateRange() (min, max Date) {
	// We make use of the fact that all ledger entries (except exchange rates)
	// are stored chronologically sorted.
	for _, es := range s.entries {
		l := len(es)
		if l == 0 {
			continue
		}
		if es[l-1].ValueDate.After(max.Time) {
			max = es[l-1].ValueDate
		}
		if min.IsZero() || es[0].ValueDate.Before(min.Time) {
			min = es[0].ValueDate
		}
	}
	return
}

func (s *Store) FindEntryBySequenceNum(sequenceNum int64) *LedgerEntry {
	es := s.ledger.Entries
	i := sort.Search(len(es), func(i int) bool {
		return es[i].SequenceNum >= sequenceNum
	})
	if i < len(es) && es[i].SequenceNum == sequenceNum {
		return es[i]
	}
	return nil
}

// EntriesAround returns n ledger entries before date and n entries after date for assetID.
// If there are less than n entries before or after, guess what, only those are returned.
func (s *Store) EntriesAround(assetID string, date Date, n int) (before, after []*LedgerEntry) {
	es := s.entries[assetID]
	if len(es) > 0 {
		i := sort.Search(len(es), func(i int) bool {
			return es[i].ValueDate.After(date.Time)
		})
		a, b := 0, len(es)
		if i-n > 0 {
			a = i - n
		}
		if i+n < b {
			b = i + n
		}
		before = es[a:i]
		after = es[i:b]
	}
	return
}

func (s *Store) PriceAt(assetID string, t Date) (Micros, Date, bool) {
	es := s.entries[assetID]
	// Find first entry after t.
	i := sort.Search(len(es), func(i int) bool {
		return es[i].ValueDate.After(t.Time)
	})
	// Scan backwards until first entry containing a price is found.
	i--
	for ; i >= 0; i-- {
		if es[i].PriceMicros != 0 {
			return es[i].PriceMicros, es[i].ValueDate, true
		}
	}
	return 0, Date{}, false
}

// ExchangeRateAt returns the BaseCurrency/QuoteCurrency exchange rate at the given time.
// A value of 1.50 means that for 1 BaseCurrency you get 1.50 QuoteCurrency c.
// The rate is derived from ExchangeRate entries in the ledger; the most recent rate
// before t is used and its date is returned as the second return value.
// If no exchange rate between the given currency c and the base currency is known at t,
// the result is zero and the third return value is false.
func (s *Store) ExchangeRateAt(c Currency, t Date) (Micros, Date, bool) {
	if c == s.BaseCurrency() {
		return UnitValue, t, true
	}
	rs, ok := s.exchangeRates[c]
	if !ok || len(rs) == 0 {
		return 0, Date{}, false
	}
	i := sort.Search(len(rs), func(i int) bool {
		return rs[i].ValueDate.Compare(t) > 0
	})
	if i == 0 {
		return 0, Date{}, false
	}
	return rs[i-1].PriceMicros, rs[i-1].ValueDate, true
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
		timezones:     make(map[string]*time.Location),
	}
	// Build asset index.
	for _, asset := range ledger.Assets {
		if err := s.validateAsset(asset); err != nil {
			return nil, fmt.Errorf("invalid asset: %v", err)
		}
		id := asset.ID()
		if _, found := s.assets[id]; found {
			return nil, fmt.Errorf("duplicate ID in ledger assets: %q", id)
		}
		s.assets[asset.ID()] = asset
	}
	// Validate ledger entries and add to asset-keyed index.
	var prevSeqNum int64
	for _, e := range ledger.Entries {
		if e.SequenceNum <= prevSeqNum {
			return nil, fmt.Errorf("invalid ledger: invalid or non-monotonic sequence number: %d", e.SequenceNum)
		}
		prevSeqNum = e.SequenceNum
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
		return nil, fmt.Errorf("failed to open ledger file: %w", err)
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
	if a.CustomID != "" {
		return a.CustomID
	}
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
	return ""
}

func (a *Asset) Category() AssetCategory {
	return a.Type.category()
}

func (a *Asset) matchRef(ref string) bool {
	if a.IBAN == ref || a.ISIN == ref || a.WKN == ref ||
		a.AccountNumber == ref || a.TickerSymbol == ref ||
		a.ShortName == ref || a.Name == ref {
		return true
	}
	for _, s := range a.QuoteServiceSymbols {
		if s == ref {
			return true
		}
	}
	return false
}

func (s *Store) nextSequenceNum() int64 {
	if len(s.ledger.Entries) == 0 {
		return 1
	}
	return s.ledger.Entries[len(s.ledger.Entries)-1].SequenceNum + 1
}

// EntriesInRange returns all ledger entries for the given asset
// in the (inclusive) range [start, end].
func (s *Store) EntriesInRange(assetId string, start, end Date) []*LedgerEntry {
	var result []*LedgerEntry
	es := s.entries[assetId]
	if len(es) == 0 {
		return result
	}
	i := sort.Search(len(es), func(i int) bool {
		return es[i].ValueDate.Compare(start) >= 0
	})
	for ; i < len(es) && !es[i].ValueDate.After(end.Time); i++ {
		result = append(result, es[i])
	}
	return result
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
// and updates the internal ledger and all relevant indexes.
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
	if e.Type == ExchangeRate && e.Currency == s.ledger.Header.BaseCurrency {
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
	if e.Currency != "" && !ValidCurrency(e.Currency) {
		return fmt.Errorf("invalid currency: %q", e.Currency)
	}
	if e.QuoteCurrency != "" && !ValidCurrency(e.QuoteCurrency) {
		return fmt.Errorf("invalid quote currency: %q", e.QuoteCurrency)
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
	if e.Currency != a.Currency {
		return fmt.Errorf("wrong currency %q for asset %s (want: %q)", e.Currency, a.ID(), a.Currency)
	}
	// General validation
	if e.QuoteCurrency != "" {
		return fmt.Errorf("QuoteCurrency must only be specified for ExchangeRate entry, not %q", e.Type)
	}
	if e.PriceMicros < 0 {
		return fmt.Errorf("PriceMicros must not be negative")
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
		if e.Type == AssetPurchase && e.QuantityMicros < 0 {
			return fmt.Errorf("QuantityMicros must be positive for %v, was %v", e.Type, e.QuantityMicros)
		} else if e.Type == AssetSale && e.QuantityMicros > 0 {
			return fmt.Errorf("QuantityMicros must be negative for %v, was %v", e.Type, e.QuantityMicros)
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
	if e.Currency == "" && e.AssetID != "" {
		// Copy currency from asset.
		if a := s.assets[e.AssetID]; a != nil {
			e.Currency = a.Currency
		}
	}
	if err := s.validateEntry(e); err != nil {
		return fmt.Errorf("entry validation failed: %w", err)
	}
	s.insert(e)
	return nil
}

// Updates replaces the ledger entry sequenceNum with the given entry e.
// In contrast to Add, Update expects e to be entirely valid; it will only
// lookup assets by ID, the currency must be set, etc.
func (s *Store) Update(e *LedgerEntry) error {
	if e.SequenceNum == 0 {
		return fmt.Errorf("cannot update entry with 0 SequenceNum")
	}
	old := s.FindEntryBySequenceNum(e.SequenceNum)
	if old == nil {
		return fmt.Errorf("no entry with SequenceNum %d", e.SequenceNum)
	}
	if err := s.validateEntry(e); err != nil {
		return fmt.Errorf("entry validation failed: %w", err)
	}
	// Overwrite existing entry's data with new entry, but update Created
	if e.Created.IsZero() {
		e.Created = time.Now()
	}
	*old = *e
	return nil
}

func (s *Store) Delete(sequenceNum int64) error {
	i := 0
	es := s.ledger.Entries
	for ; i < len(es); i++ {
		if es[i].SequenceNum == sequenceNum {
			break
		}
	}
	if i == len(es) {
		return fmt.Errorf("sequence number %d not found in ledger", sequenceNum)
	}
	if es[i].AssetID != "" {
		// Delete from .entries index.
		aes := s.entries[es[i].AssetID]
		for j, e := range aes {
			if e == es[i] {
				copy(aes[j:], aes[j+1:])
				s.entries[es[i].AssetID] = aes[:len(aes)-1]
				break
			}
		}
	} else if es[i].Type == ExchangeRate {
		// Delete from .exchangeRates index.
		qes := s.exchangeRates[es[i].QuoteCurrency]
		for j, e := range qes {
			if e == es[i] {
				copy(qes[j:], qes[j+1:])
				s.exchangeRates[es[i].QuoteCurrency] = qes[:len(qes)-1]
				break
			}
		}
	}
	// Delete from ledger.
	copy(es[i:], es[i+1:])
	s.ledger.Entries = es[:len(es)-1]
	return nil
}

func (s *Store) validateAsset(a *Asset) error {
	id := a.ID()
	if id == "" {
		return fmt.Errorf("Asset must have an ID")
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("Asset name must not be empty")
	}
	cat := a.Category()
	if (a.InterestMicros != 0 || a.InterestPayment != "") && (cat == Equity || cat == Commodities) {
		return fmt.Errorf("interest must not be specified for asset category %s", cat)
	}
	if a.MaturityDate != nil {
		if a.MaturityDate.IsZero() {
			return fmt.Errorf("MaturityDate must be nil or non-zero")
		}
		if a.Category() != FixedIncome {
			return fmt.Errorf("MaturityDate must only be specified for fixed-income assets")
		}
	}
	if a.IssueDate != nil {
		if a.IssueDate.IsZero() {
			return fmt.Errorf("IssueDate must be nil or non-zero")
		}
		if a.Category() != FixedIncome {
			return fmt.Errorf("IssueDate must only be specified for fixed-income assets")
		}
	}
	if a.MaturityDate != nil && a.IssueDate != nil && a.MaturityDate.Before(a.IssueDate.Time) {
		return fmt.Errorf("MaturityDate must not be before IssueDate")
	}
	if !ValidCurrency(a.Currency) {
		return fmt.Errorf("unknown or invalid currency %q: must use ISO code (3 uppercase letters)", a.Currency)
	}
	if a.IBAN != "" && !validIBAN(a.IBAN) {
		return fmt.Errorf("invalid IBAN: %q", a.IBAN)
	}
	if a.ExchangeTimezone != "" {
		if _, err := s.timezone(a.ExchangeTimezone); err != nil {
			return fmt.Errorf("invalid ExchangeTimezone: %v", err)
		}
	}
	return nil
}

func (s *Store) AddAsset(a *Asset) error {
	if err := s.validateAsset(a); err != nil {
		return err
	}
	id := a.ID()
	if _, ok := s.assets[id]; ok {
		return fmt.Errorf("duplicate asset ID %q", id)
	}
	now := time.Now()
	a.Created = now
	a.Modified = now
	s.assets[id] = a
	s.ledger.Assets = append(s.ledger.Assets, a)
	return nil
}

func (s *Store) UpdateAsset(assetID string, a *Asset) error {
	old := s.assets[assetID]
	if old == nil {
		return fmt.Errorf("no asset with ID %q", assetID)
	}
	id := a.ID()
	if id != old.ID() {
		return fmt.Errorf("asset ID change is not supported")
	}
	if err := s.validateAsset(a); err != nil {
		return err
	}
	// Check that immutable fields do not change:
	hasEntries := len(s.entries[id]) > 0
	if hasEntries && old.Category() != a.Category() {
		return fmt.Errorf("cannot modify asset category (%s => %s): asset has ledger entries",
			old.Category(), a.Category())
	}
	if hasEntries && old.Currency != a.Currency {
		return fmt.Errorf("cannot modify currency: asset has ledger entries")
	}
	// Update modified time, but keep old created time.
	created := old.Created
	*old = *a
	old.Created = created
	old.Modified = time.Now()
	return nil
}

// AssetPositionItem tracks an individual purchase that is part of the
// accumulated asset position.
type AssetPositionItem struct {
	ValueDate      Date
	QuantityMicros Micros
	PriceMicros    Micros
	CostMicros     Micros
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
	PriceDate      Date
	// Items are the constituent parts of the accumulated asset position.
	// The are stored in chronological order (latest comes last) and can
	// be used to determine profit & loss (P&L) and to update the
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

// AssetPurchases returns the difference between asset positions bought and sold in the given period.
func (s *Store) AssetPurchases(assetId string, startDate, endDate Date) Micros {
	entries := s.entries[assetId]
	i := sort.Search(len(entries), func(i int) bool {
		return !entries[i].ValueDate.Before(startDate.Time)
	})
	var sum Micros
	for ; i < len(entries) && !entries[i].ValueDate.After(endDate.Time); i++ {
		e := entries[i]
		switch e.Type {
		case AssetPurchase, AssetSale:
			sum += e.QuantityMicros.Mul(e.PriceMicros) + e.CostMicros
		}
	}
	return sum
}

// ProfitLossInPeriod calculates the P&L for the given asset in the given period.
//
// As asset quantities might have been bought and sold during the period, there are
// a few cases to consider:
//
//   - The qty was owned throughout the period: P&L is just the
//     difference between the market value at the beginning and end of the period.
//   - The qty was bought during the period: P&L is the difference of its
//     purchasing price (including costs) and its market value at the end of the period.
//   - The qty was owned at the beginning, but sold during the period:
//     P&L is the difference between market value at the beginning of the period and
//     sale price (minus costs).
//   - The qty was purchased and sold during the period: P&L is the diff of
//     purchasing price (including costs) and sale price (minus costs).
func (s *Store) ProfitLossInPeriod(assetId string, startDate, endDate Date) (profitLoss, referenceValue Micros, err error) {
	if startDate.After(endDate.Time) {
		return 0, 0, fmt.Errorf("startDate %v after endDate %v", startDate, endDate)
	}
	p := s.AssetPositionAt(assetId, startDate)
	if p == nil {
		return 0, 0, fmt.Errorf("no position found for asset %q", assetId)
	}
	if cat := p.Asset.Category(); cat != Equity {
		return 0, 0, fmt.Errorf("profit/loss in period is only available for asset category Equity, not %v", cat)
	}
	// Store the price at the beginning of the period. That is the price we calculate
	// unrealized P&L against for all quantities still owned at the end of the period.
	startPrice := p.PriceMicros
	entries := s.entries[assetId]
	i := sort.Search(len(entries), func(i int) bool {
		return entries[i].ValueDate.After(startDate.Time)
	})
	var realizedPL Micros
	for ; i < len(entries) && !entries[i].ValueDate.After(endDate.Time); i++ {
		e := entries[i]
		switch e.Type {
		case AssetSale:
			// A sale realizes P&L: Realized P&L = Sale Price - Cost of sale - Price at beginning of period
			// Calculate the purchase price of the quantity sold.
			var purchasePrice Micros
			qty := -e.QuantityMicros // QuantityMicros is a negative value for a sale
			for _, item := range p.Items {
				if item.QuantityMicros < qty {
					// Fully incorporate the item's quantity and continue.
					if item.ValueDate.After(startDate.Time) {
						// Item was purchased after startDate: use price at purchase date incl. cost.
						purchasePrice += item.PurchasePrice()
					} else {
						// Use price at start date.
						// Do not include the cost, it was accounted for in a previous period.
						purchasePrice += item.QuantityMicros.Mul(startPrice)
					}
					qty -= item.QuantityMicros
				} else {
					// Pro-rate the item's quantity and end the iteration (qty has been fully processed).
					if item.ValueDate.After(startDate.Time) {
						// Item was purchased after startDate: use price at purchase date and include cost.
						purchasePrice += qty.Mul(item.PriceMicros) + item.CostMicros
					} else {
						// Use price at start date.
						purchasePrice += qty.Mul(startPrice)
					}
					break
				}
			}
			referenceValue += purchasePrice
			realizedPL += (-e.QuantityMicros).Mul(e.PriceMicros) - e.CostMicros - purchasePrice
		case AssetHolding:
			if p.QuantityMicros != e.QuantityMicros {
				return 0, 0, fmt.Errorf("cannot calculate P&L: new quantity is asserted with AssetHolding entry")
			}
			// Otherwise, ignore entry
		case AssetPurchase, AssetPrice:
			// Ignore; we can use the final AssetPositionItems to calculate unrealized P&L
		case DividendPayment, InterestPayment:
			// TOOD: include dividends in P&L
		default:
			return 0, 0, fmt.Errorf("unexpected ledger entry of type %v", e.Type)
		}
		p.Update(e)
	}
	var initialValue Micros
	for _, item := range p.Items {
		if item.ValueDate.After(startDate.Time) {
			// Item was purchased after startDate, use its own purchase price.
			// Include cost in the baselineValue: to make a profit,
			// the price must go above the price at purchase time plus incurred cost.
			initialValue += item.PurchasePrice()
		} else {
			// Item was owned at startDate already, use price at startDate.
			// Do not include the cost, it was accounted for in a previous period.
			initialValue += item.QuantityMicros.Mul(startPrice)
		}
	}
	referenceValue += initialValue
	return p.MarketValue() - initialValue + realizedPL, referenceValue, nil
}

// AssetPositionAt returns the given asset's position at date.
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

func (a *AssetPositionItem) PurchasePrice() Micros {
	return a.QuantityMicros.Mul(a.PriceMicros) + a.CostMicros
}

// Copy returns a semi-deep copy of p: It shares the pointer to the asset,
// but all position-specific values (including Items) are copied.
func (p *AssetPosition) Copy() *AssetPosition {
	q := *p
	q.Items = make([]AssetPositionItem, len(p.Items))
	copy(q.Items, p.Items)
	return &q
}

func (p *AssetPosition) ID() string {
	return p.Asset.ID()
}
func (p *AssetPosition) Name() string {
	return p.Asset.Name
}
func (p *AssetPosition) Currency() Currency {
	return p.Asset.Currency
}

func (p *AssetPosition) SetPrice(price Micros, date Date) {
	p.PriceMicros = price
	p.PriceDate = date
}

func (p *AssetPosition) Update(e *LedgerEntry) {
	p.LastUpdated = e.ValueDate
	switch e.Type {
	case AssetPurchase:
		p.QuantityMicros += e.QuantityMicros
		p.SetPrice(e.PriceMicros, e.ValueDate)
		p.Items = append(p.Items, AssetPositionItem{
			ValueDate:      e.ValueDate,
			QuantityMicros: e.QuantityMicros,
			PriceMicros:    e.PriceMicros,
			CostMicros:     e.CostMicros,
		})
	case AssetSale:
		p.QuantityMicros += e.QuantityMicros
		p.SetPrice(e.PriceMicros, e.ValueDate)
		// Remove items
		qty := e.QuantityMicros
		for len(p.Items) > 0 {
			hd := &p.Items[0]
			if hd.QuantityMicros > -qty {
				oldQ := hd.QuantityMicros
				hd.QuantityMicros += qty
				hd.CostMicros = hd.CostMicros.Frac(hd.QuantityMicros, oldQ)
				break
			}
			qty += hd.QuantityMicros
			p.Items = p.Items[1:]
		}
		if len(p.Items) == 0 {
			p.Items = nil // allow GC of Items
		}
	case AssetMaturity:
		p.ValueMicros = 0
		p.QuantityMicros = 0
		p.SetPrice(e.PriceMicros, e.ValueDate)
		p.Items = nil
	case AssetPrice:
		p.SetPrice(e.PriceMicros, e.ValueDate)
	case AccountCredit:
		p.ValueMicros += e.ValueMicros
		// In a "normal" account, we don't keep track of individual credit/debit
		// transactions in the AssetPosition, since we only care about the account
		// balance. For accounts like FixedDepositAccount or PensionAccount, we
		// do care about individual credits (and debits, though those are not typical),
		// e.g. to calculate total earnings at maturity.
		if p.Asset.Type.UseTransactionTracking() {
			p.Items = append(p.Items, AssetPositionItem{
				ValueDate:      e.ValueDate,
				QuantityMicros: e.ValueMicros,
				PriceMicros:    UnitValue,
			})
		}
	case AccountDebit:
		p.ValueMicros += e.ValueMicros
		// See the note in AccountCredit case above.
		if p.Asset.Type.UseTransactionTracking() {
			val := e.ValueMicros
			for len(p.Items) > 0 {
				hd := &p.Items[0]
				if hd.QuantityMicros > -val {
					hd.QuantityMicros += val
					break
				}
				val -= hd.QuantityMicros
				p.Items = p.Items[1:]
			}
			if len(p.Items) == 0 {
				p.Items = nil // allow GC of Items
			}
		}
	case AccountBalance:
		p.ValueMicros = e.ValueMicros
		// Items are not influenced by AccountBalance entries, not even for
		// those with transaction tracking. Credit/Debit entries must be used
		// to keep track of all inpayments/outflows.
	case AssetHolding:
		p.SetPrice(e.PriceMicros, e.ValueDate)
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
	for _, item := range p.Items {
		price += item.QuantityMicros.Mul(item.PriceMicros) + item.CostMicros
	}
	return price
}

func (p *AssetPosition) MarketValue() Micros {
	if p.QuantityMicros != 0 {
		return p.QuantityMicros.Mul(p.PriceMicros)
	}
	return p.ValueMicros
}
