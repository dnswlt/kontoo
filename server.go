package kontoo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// JSON API for server requests and responses.
type AddLedgerEntryResponse struct {
	Status      StatusCode `json:"status"`
	Error       string     `json:"error,omitempty"`
	SequenceNum int64      `json:"sequenceNum"`
}
type DeleteLedgerEntryRequest struct {
	// We use a pointer to detect if the field was explicitly set.
	SequenceNum *int64 `json:"sequenceNum"`
}
type DeleteLedgerEntryResponse struct {
	Status      StatusCode `json:"status"`
	Error       string     `json:"error,omitempty"`
	SequenceNum int64      `json:"sequenceNum"`
}
type UpsertAssetRequest struct {
	AssetID string `json:"assetId,omitempty"`
	Asset   *Asset `json:"asset"`
}
type UpsertAssetResponse struct {
	Status  StatusCode `json:"status"`
	Error   string     `json:"error,omitempty"`
	AssetID string     `json:"assetId,omitempty"`
}

type CsvUploadResponse struct {
	Status     StatusCode `json:"status"`
	Error      string     `json:"error,omitempty"`
	NumEntries int        `json:"numEntries"`
	InnerHTML  string     `json:"innerHTML"`
}

type AddQuoteItem struct {
	AssetID     string `json:"assetID"`
	Date        Date   `json:"date"`
	PriceMicros Micros `json:"priceMicros"`
}
type AddExchangeRateItem struct {
	BaseCurrency  Currency `json:"baseCurrency"`
	QuoteCurrency Currency `json:"quoteCurrency"`
	Date          Date     `json:"date"`
	PriceMicros   Micros   `json:"priceMicros"`
}
type AddQuotesRequest struct {
	Quotes        []*AddQuoteItem        `json:"quotes"`
	ExchangeRates []*AddExchangeRateItem `json:"exchangeRates"`
}
type AddQuotesResponse struct {
	Status        StatusCode `json:"status"`
	Error         string     `json:"error,omitempty"`
	ItemsImported int        `json:"itemsImported"`
}

type PositionTimelineRequest struct {
	AssetIDs     []string `json:"assetIds"`
	EndTimestamp int64    `json:"endTimestamp"`
	Period       string   `json:"period"`
}

// PositionTimeline contains time series data about an asset position.
type PositionTimeline struct {
	AssetID    string  `json:"assetId"`
	AssetName  string  `json:"assetName"`
	Timestamps []int64 `json:"timestamps"`
	// Send values as int64 micros: the JSON marshalling of Micros
	// would send them as strings (e.g. "12.3").
	QuantityMicros []int64 `json:"quantityMicros"`
	ValueMicros    []int64 `json:"valueMicros"`
}
type PositionTimelineResponse struct {
	Status    StatusCode          `json:"status"`
	Error     string              `json:"error,omitempty"`
	Timelines []*PositionTimeline `json:"timelines,omitempty"`
}
type PositionsMaturitiesRequest struct {
	EndTimestamp int64 `json:"endTimestamp"`
}
type MaturitiesChartValues struct {
	Label       string  `json:"label"`
	ValueMicros []int64 `json:"valueMicros"`
}
type MaturitiesChartData struct {
	Currency     string                   `json:"currency"`
	BucketLabels []string                 `json:"bucketLabels"`
	Values       []*MaturitiesChartValues `json:"values"`
}
type PositionsMaturitiesResponse struct {
	Status     StatusCode           `json:"status"`
	Error      string               `json:"error,omitempty"`
	Maturities *MaturitiesChartData `json:"maturities,omitempty"`
}

type StatusCode string

const (
	StatusOK              StatusCode = "OK"
	StatusPartialSuccess  StatusCode = "PARTIAL_SUCCESS"
	StatusInvalidArgument StatusCode = "INVALID_ARGUMENT"
)

// END JSON API

type Server struct {
	addr       string
	ledgerPath string
	baseDir    string
	templates  *template.Template
	store      *Store
	debugMode  bool
	// Stock quote service
	yFinance *YFinance
}

func NewServer(addr, ledgerPath, baseDir string) (*Server, error) {
	expectedFiles := []string{
		"resources/style.css",
		"templates/ledger.html",
	}
	for _, f := range expectedFiles {
		if _, err := os.Stat(path.Join(baseDir, f)); err != nil {
			return nil, fmt.Errorf("invalid baseDir %q: file %q not found: %w", baseDir, f, err)
		}
	}
	store, err := LoadStore(ledgerPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load store: %w", err)
	}
	yf, err := NewYFinance()
	if err != nil {
		log.Printf("Error creating YFinance. Stock quotes will not be available. Error: %v", err)
		yf = nil // Should be nil anyway, but better be safe
	}
	s := &Server{
		addr:       addr,
		ledgerPath: ledgerPath,
		baseDir:    baseDir,
		store:      store,
		yFinance:   yf,
	}
	if err := s.reloadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) Store() *Store {
	return s.store
}

func (s *Server) ReloadStore() error {
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		return err
	}
	s.store = store
	return nil
}

func (s *Server) DebugMode(enabled bool) {
	s.debugMode = enabled
}

func (s *Server) reloadTemplates() error {
	glob := path.Join(s.baseDir, "templates", "*.html")
	tmpl, err := template.New("__root__").Funcs(commonFuncs()).ParseGlob(glob)
	if err != nil {
		return fmt.Errorf("could not parse templates: %w", err)
	}
	s.templates = tmpl
	return nil
}

type LedgerEntryRow struct {
	E *LedgerEntry
	A *Asset
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
	return e.A.ID()
}
func (e *LedgerEntryRow) AssetName() string {
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

func LedgerEntryRows(s *Store, query *Query) []*LedgerEntryRow {
	var res []*LedgerEntryRow
	for _, e := range s.ledger.Entries {
		r := &LedgerEntryRow{
			E: e,
			A: s.assets[e.AssetID],
		}
		if !query.Match(r) {
			continue
		}
		res = append(res, r)
	}
	query.Sort(res)
	res = query.LimitGroups(res)
	return res
}

type PositionTableRow struct {
	AssetID   string
	AssetName string
	AssetType AssetType
	Currency  Currency
	Value     Micros
	// BaseCurrency/Currency exchange rate. Used to calculate all monetary values
	// of this position in the base currency.
	ExchangeRate Micros
	// Notes about the position to be displayed to the user
	// (e.g. about old data being shown).
	Notes []string
	// Maximum age of the data on which the Value and ValueBaseCurrency
	// are calculated. Used to display warnings in the UI if the age is
	// above a threshold.
	DataAge time.Duration

	Quantity  Micros
	Price     Micros
	PriceDate Date

	PurchasePrice           Micros
	NominalValue            Micros
	InterestRate            Micros
	IssueDate               *Date
	MaturityDate            *Date
	TotalEarningsAtMaturity Micros
	InternalRateOfReturn    Micros
	YearsToMaturity         float64
}

func (r *PositionTableRow) ProfitLoss() Micros {
	return r.Value - r.PurchasePrice
}

func (r *PositionTableRow) ProfitLossRatio() Micros {
	if r.PurchasePrice == 0 {
		return 0
	}
	return FloatAsMicros(r.Value.Float()/r.PurchasePrice.Float() - 1)
}

// Convenience function for sorting *Date. Nils come last.
func compareDatePtr(l, r *Date) int {
	if l == nil {
		if r == nil {
			return 0
		}
		return -1
	}
	if r == nil {
		return 1
	}
	return l.Compare(*r)
}

func maturingPositionTableRows(s *Store, date Date) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	slices.SortFunc(positions, func(a, b *AssetPosition) int {
		c := compareDatePtr(a.Asset.MaturityDate, b.Asset.MaturityDate)
		if c != 0 {
			return c
		}
		return strings.Compare(a.Name(), b.Name())
	})
	var res []*PositionTableRow
	for _, p := range positions {
		a := p.Asset
		if a.MaturityDate == nil {
			continue
		}
		rate, _, _ := s.ExchangeRateAt(a.Currency, date)
		row := &PositionTableRow{
			AssetID:                 a.ID(),
			AssetName:               a.Name,
			AssetType:               a.Type,
			Currency:                a.Currency,
			ExchangeRate:            rate,
			Value:                   p.MarketValue(),
			PurchasePrice:           p.PurchasePrice(),
			NominalValue:            p.QuantityMicros,
			InterestRate:            a.InterestMicros,
			IssueDate:               a.IssueDate,
			MaturityDate:            a.MaturityDate,
			TotalEarningsAtMaturity: totalEarningsAtMaturity(p),
			InternalRateOfReturn:    internalRateOfReturn(p),
			YearsToMaturity:         a.MaturityDate.Sub(date.Time).Hours() / 24 / 365,
		}
		res = append(res, row)
	}
	return res
}

func equityPositionTableRows(s *Store, date Date) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	slices.SortFunc(positions, func(a, b *AssetPosition) int {
		c := int(a.Asset.Type) - int(b.Asset.Type)
		if c != 0 {
			return c
		}
		return strings.Compare(a.Name(), b.Name())
	})
	var res []*PositionTableRow
	for _, p := range positions {
		a := p.Asset
		if a.Type.Category() != Equity {
			continue
		}
		rate, _, _ := s.ExchangeRateAt(a.Currency, date)
		row := &PositionTableRow{
			AssetID:       a.ID(),
			AssetName:     a.Name,
			AssetType:     a.Type,
			Currency:      a.Currency,
			ExchangeRate:  rate,
			Value:         p.MarketValue(),
			Quantity:      p.QuantityMicros,
			Price:         p.PriceMicros,
			PriceDate:     p.PriceDate,
			PurchasePrice: p.PurchasePrice(),
		}
		res = append(res, row)
	}
	return res
}

func positionTableRows(s *Store, date Date) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	slices.SortFunc(positions, func(a, b *AssetPosition) int {
		c := int(a.Asset.Type.Category()) - int(b.Asset.Type.Category())
		if c != 0 {
			return c
		}
		c = int(a.Asset.Type) - int(b.Asset.Type)
		if c != 0 {
			return c
		}
		return strings.Compare(a.Name(), b.Name())
	})
	res := make([]*PositionTableRow, len(positions))
	for i, p := range positions {
		a := p.Asset
		var notes []string
		if !p.LastUpdated.IsZero() {
			notes = append(notes, fmt.Sprintf("Last updated: %s", p.LastUpdated))
		}
		// Ignoring the error is fine: we interpret a 0 exchange rate as a missing value
		// and we can't do much more here if the rate is missing anyway.
		rate, _, _ := s.ExchangeRateAt(a.Currency, date)
		res[i] = &PositionTableRow{
			AssetID:      a.ID(),
			AssetName:    a.Name,
			AssetType:    a.Type,
			Currency:     a.Currency,
			ExchangeRate: rate,
			Value:        p.MarketValue(),
			Notes:        notes,
			DataAge:      date.Sub(p.LastUpdated.Time),
		}
	}
	return res
}

type PositionTableRowGroup struct {
	Category AssetCategory
	Rows     []*PositionTableRow
}

func (g *PositionTableRowGroup) ValueBaseCurrency() Micros {
	var sum Micros
	for _, r := range g.Rows {
		if r.ExchangeRate == 0 {
			return 0 // Cannot calculate sum in base currency
		}
		sum += r.Value.Div(r.ExchangeRate)
	}
	return sum
}

func positionTableRowGroups(rows []*PositionTableRow) []*PositionTableRowGroup {
	var res []*PositionTableRowGroup
	if len(rows) == 0 {
		return res
	}
	currentCategory := rows[0].AssetType.Category()
	res = append(res, &PositionTableRowGroup{
		Category: currentCategory,
		Rows:     []*PositionTableRow{rows[0]},
	})
	for _, row := range rows[1:] {
		if row.AssetType.Category() == currentCategory {
			res[len(res)-1].Rows = append(res[len(res)-1].Rows, row)
		} else {
			res = append(res, &PositionTableRowGroup{
				Category: row.AssetType.Category(),
				Rows:     []*PositionTableRow{row},
			})
			currentCategory = row.AssetType.Category()
		}
	}
	return res
}

func newURL(path string, queryParams url.Values) *url.URL {
	u, err := url.Parse(path)
	if err != nil {
		panic("failed to parse URL: " + err.Error())
	}
	u.RawQuery = queryParams.Encode()
	return u
}

func (s *Server) addCommonCtx(r *http.Request, ctx map[string]any) map[string]any {
	ctx["Today"] = time.Now().Format("2006-01-02")
	ctx["Now"] = time.Now().Format("2006-01-02 15:04:05")
	ctx["BaseCurrency"] = s.Store().BaseCurrency()
	ctx["ThisPage"] = r.URL.String()
	ctxQ := make(url.Values)
	// Inherit contextual query params from the incoming request.
	q := r.URL.Query()
	if date := q.Get("date"); date != "" {
		ctx["Date"] = date
		ctxQ.Set("date", date)
	}
	// Default filter for ledger view: no prices and exchange rates.
	ctxQ.Set("q", `$main`)
	ledgerURL := newURL("/kontoo/ledger", ctxQ)
	ctxQ.Del("q")
	ctx["Nav"] = map[string]string{
		"ledger":    ledgerURL.String(),
		"positions": newURL("/kontoo/positions", ctxQ).String(),
		"addEntry":  newURL("/kontoo/entries/new", ctxQ).String(),
		"addAsset":  newURL("/kontoo/assets/new", ctxQ).String(),
		"editAsset": newURL("/kontoo/assets/edit/{assetID}", ctxQ).String(),
		"uploadCSV": newURL("/kontoo/csv/upload", ctxQ).String(),
		"quotes":    newURL("/kontoo/quotes", ctxQ).String(),
	}
	return ctx
}

func (s *Server) renderLedgerTemplate(w io.Writer, r *http.Request, query *Query, snippet bool) error {
	rows := LedgerEntryRows(s.Store(), query)
	tmpl := "ledger.html"
	if snippet {
		tmpl = "snip_ledger_table.html"
	}
	ctx := s.addCommonCtx(r, map[string]any{
		"TableRows": rows,
		"Query":     query.raw,
	})
	return s.templates.ExecuteTemplate(w, tmpl, ctx)
}

func (s *Server) renderAddEntryTemplate(w io.Writer, r *http.Request, date Date) error {
	assets := make([]*Asset, len(s.Store().ledger.Assets))
	copy(assets, s.Store().ledger.Assets)
	slices.SortFunc(assets, func(a, b *Asset) int {
		return strings.Compare(a.Name, b.Name)
	})
	quoteCurrenciesMap := make(map[Currency]bool)
	var quoteCurrencies []Currency
	for _, a := range assets {
		if a.Currency == s.Store().BaseCurrency() {
			continue
		}
		if _, ok := quoteCurrenciesMap[a.Currency]; !ok {
			quoteCurrencies = append(quoteCurrencies, a.Currency)
			quoteCurrenciesMap[a.Currency] = true
		}
	}
	slices.Sort(quoteCurrencies)
	ctx := s.addCommonCtx(r, map[string]any{
		"Today":           time.Now().Format("2006-01-02"),
		"Date":            date,
		"Assets":          assets,
		"BaseCurrency":    s.Store().BaseCurrency(),
		"QuoteCurrencies": quoteCurrencies,
		"EntryTypes":      EntryTypeValues()[1:],
	})
	// Populate input field values from query params
	q := r.URL.Query()
	for _, p := range []string{"AssetID", "Type"} {
		if v := q.Get(p); v != "" {
			ctx[p] = v
		}
	}
	return s.templates.ExecuteTemplate(w, "entry.html", ctx)
}

func (s *Server) renderAssetTemplate(w io.Writer, r *http.Request, asset *Asset) error {
	assetTypeVals := AssetTypeValues()
	assetTypes := make([]string, 0, len(assetTypeVals))
	for _, a := range assetTypeVals {
		if a == UnspecifiedAssetType {
			continue
		}
		assetTypes = append(assetTypes, a.String())
	}
	if asset == nil {
		asset = &Asset{} // Ensure template can render empty values.
	}
	ctx := s.addCommonCtx(r, map[string]any{
		"AssetTypes":           assetTypes,
		"InterestPaymentTypes": allInterestPaymentSchedules,
		"Asset":                asset,
	})
	return s.templates.ExecuteTemplate(w, "asset.html", ctx)
}

func (s *Server) renderQuotesTemplate(w io.Writer, r *http.Request, date time.Time) error {
	type QuoteEntry struct {
		AssetID      string
		AssetName    string
		Symbol       string
		Currency     Currency
		ClosingPrice Micros
		Date         time.Time
	}
	if s.yFinance == nil {
		// No quotes service, can't show quotes.
		return s.templates.ExecuteTemplate(w, "quotes.html", s.addCommonCtx(r, map[string]any{}))
	}
	var entries []*QuoteEntry
	var exchangeRates []*DailyExchangeRate
	assets := s.Store().FindAssetsForQuoteService("YF")
	symbols := make([]string, len(assets))
	assetMap := make(map[string]*Asset)
	for i, a := range assets {
		symbols[i] = a.QuoteServiceSymbols["YF"]
		assetMap[a.QuoteServiceSymbols["YF"]] = a
	}
	hist, err := s.yFinance.GetDailyQuotes(symbols, date)
	if err != nil {
		log.Printf("Failed to get price history: %v", err)
	}
	for _, h := range hist {
		a := assetMap[h.Symbol]
		entries = append(entries, &QuoteEntry{
			AssetID:      a.ID(),
			AssetName:    a.Name,
			Symbol:       h.Symbol,
			Currency:     h.Currency,
			ClosingPrice: h.ClosingPrice,
			Date:         h.Timestamp,
		})
	}
	quoteCurrencies := s.Store().QuoteCurrencies()
	if len(quoteCurrencies) > 0 {
		var err error
		exchangeRates, err = s.yFinance.GetDailyExchangeRates(s.Store().BaseCurrency(), quoteCurrencies, date)
		if err != nil {
			log.Printf("Failed to get exchange rates: %v", err)
		}
	}
	ctx := s.addCommonCtx(r, map[string]any{
		"Entries":       entries,
		"ExchangeRates": exchangeRates,
	})
	return s.templates.ExecuteTemplate(w, "quotes.html", ctx)
}

func (s *Server) renderPositionsTemplate(w io.Writer, r *http.Request, date Date) error {
	rows := positionTableRows(s.Store(), date)
	groups := positionTableRowGroups(rows)
	var total Micros
	for _, g := range groups {
		total += g.ValueBaseCurrency()
	}
	minDate, maxDate := s.Store().ValueDateRange()
	ctx := s.addCommonCtx(r, map[string]any{
		"TotalValueBaseCurrency": total,
		"Groups":                 groups,
		"ActiveChips": map[string]bool{
			"all":   true,
			"today": date.Equal(today()),
		},
		"MonthOptions": monthOptions(*r.URL, date, maxDate),
		"YearOptions":  yearOptions(*r.URL, date, minDate, maxDate),
	})
	return s.templates.ExecuteTemplate(w, "positions.html", ctx)
}

func (s *Server) renderMaturingPositionsTemplate(w io.Writer, r *http.Request, date Date) error {
	rows := maturingPositionTableRows(s.Store(), date)
	minDate, maxDate := s.Store().ValueDateRange()
	var totalValue, totalEarnings Micros
	for _, r := range rows {
		if r.ExchangeRate == 0 {
			totalValue, totalEarnings = 0, 0
			break
		}
		totalValue += r.Value.Div(r.ExchangeRate)
		totalEarnings += r.TotalEarningsAtMaturity.Div(r.ExchangeRate)
	}
	ctx := s.addCommonCtx(r, map[string]any{
		"TableRows": rows,
		"ActiveChips": map[string]bool{
			"maturing": true,
			"today":    date.Equal(today()),
		},
		"Totals": map[string]Micros{
			"Value":              totalValue,
			"EarningsAtMaturity": totalEarnings,
		},
		"MonthOptions": monthOptions(*r.URL, date, maxDate),
		"YearOptions":  yearOptions(*r.URL, date, minDate, maxDate),
	})
	return s.templates.ExecuteTemplate(w, "positions_maturing.html", ctx)
}

func (s *Server) renderEquityPositionsTemplate(w io.Writer, r *http.Request, date Date) error {
	rows := equityPositionTableRows(s.Store(), date)
	minDate, maxDate := s.Store().ValueDateRange()
	var totalValue, totalEarnings, totalPurchasePrice Micros
	for _, r := range rows {
		if r.ExchangeRate == 0 {
			totalValue, totalEarnings, totalPurchasePrice = 0, 0, 0
			break
		}
		totalValue += r.Value.Div(r.ExchangeRate)
		totalPurchasePrice += r.PurchasePrice.Div(r.ExchangeRate)
		totalEarnings += (r.Value - r.PurchasePrice).Div(r.ExchangeRate)
	}
	ctx := s.addCommonCtx(r, map[string]any{
		"TableRows": rows,
		"ActiveChips": map[string]bool{
			"equity": true,
			"today":  date.Equal(today()),
		},
		"Totals": map[string]Micros{
			"Value":           totalValue,
			"PurchasePrice":   totalPurchasePrice,
			"ProfitLoss":      totalEarnings,
			"ProfitLossRatio": totalEarnings.Div(totalPurchasePrice),
		},
		"MonthOptions": monthOptions(*r.URL, date, maxDate),
		"YearOptions":  yearOptions(*r.URL, date, minDate, maxDate),
	})
	return s.templates.ExecuteTemplate(w, "positions_equity.html", ctx)
}

func (s *Server) renderUploadCsvTemplate(w io.Writer, r *http.Request) error {
	return s.templates.ExecuteTemplate(w, "upload_csv.html", s.addCommonCtx(r, map[string]any{}))
}

func (s *Server) renderSnipUploadCsvData(w io.Writer, items []*DepotExportItem, store *Store) error {
	type Row struct {
		AssetID               string
		AssetName             string
		ValueDate             Date
		PriceMicros           Micros
		Currency              Currency
		QuantityImportMicros  Micros
		QuantityCurrentMicros Micros
		Preselect             bool
	}
	var rows []*Row
	for _, item := range items {
		asset := store.FindAssetByWKN(item.WKN)
		if asset == nil {
			log.Fatalf("Program error: renderSnipUploadCsvData expects WKN to exist: %q", item.WKN)
		}
		p := s.Store().AssetPositionAt(asset.ID(), item.ValueDate)
		rows = append(rows, &Row{
			AssetID:               asset.ID(),
			AssetName:             asset.Name,
			ValueDate:             item.ValueDate,
			PriceMicros:           item.PriceMicros,
			Currency:              asset.Currency,
			QuantityImportMicros:  item.QuantityMicros,
			QuantityCurrentMicros: p.QuantityMicros,
			Preselect:             p.LastUpdated.Before(item.ValueDate.Time),
		})
	}
	return s.templates.ExecuteTemplate(w, "snip_upload_csv_data.html", map[string]any{
		"Entries": rows,
	})
}

func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query, err := ParseQuery(q.Get("q"))
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid query: %v", err), http.StatusBadRequest)
		return
	}
	snippet := q.Get("snippet") == "true"
	var buf bytes.Buffer
	if err := s.renderLedgerTemplate(&buf, r, query, snippet); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleLedgerReload(w http.ResponseWriter, r *http.Request) {
	if err := s.ReloadStore(); err != nil {
		http.Error(w, "Failed to reload store: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("Store reloaded"))
}

func (s *Server) handleEntriesNew(w http.ResponseWriter, r *http.Request) {
	date, err := dateParam(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid date= parameter: %s", err), http.StatusBadRequest)
		return
	}
	var buf bytes.Buffer
	if err := s.renderAddEntryTemplate(&buf, r, date); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleAssetsNew(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := s.renderAssetTemplate(&buf, r, nil); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleAssetsEdit(w http.ResponseWriter, r *http.Request) {
	assetID := r.PathValue("assetID")
	if assetID == "" {
		http.Error(w, "missing assetID", http.StatusNotFound)
		return
	}
	asset, ok := s.Store().assets[assetID]
	if !ok {
		http.Error(w, "assetID not found", http.StatusNotFound)
		return
	}
	var buf bytes.Buffer
	if err := s.renderAssetTemplate(&buf, r, asset); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleQuotes(w http.ResponseWriter, r *http.Request) {
	date, err := dateParam(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid date= parameter: %q", err), http.StatusBadRequest)
		return
	}
	var buf bytes.Buffer
	if err := s.renderQuotesTemplate(&buf, r, date.Time); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func dateParam(r *http.Request) (Date, error) {
	d := r.URL.Query().Get("date")
	var date time.Time
	if d == "" {
		date = time.Now()
	} else {
		var err error
		date, err = time.Parse("2006-01-02", d)
		if err != nil {
			return Date{}, fmt.Errorf("invalid value for date= parameter: %q", d)
		}
	}
	return Date{date}, nil
}

func ensureDateParam(w http.ResponseWriter, r *http.Request) (Date, bool) {
	d := r.URL.Query().Get("date")
	if d == "" {
		now := today()
		q := r.URL.Query()
		q.Set("date", now.Format("2006-01-02"))
		r.URL.RawQuery = q.Encode()
		http.Redirect(w, r, r.URL.String(), http.StatusSeeOther)
		return Date{}, false
	}
	date, err := ParseDate(d)
	if err != nil {
		http.Error(w, "invalid date: "+err.Error(), http.StatusBadRequest)
		return Date{}, false
	}
	return date, true
}

func (s *Server) handlePositionsTimeline(w http.ResponseWriter, r *http.Request) {
	var req PositionTimelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	end := ToDate(time.UnixMilli(req.EndTimestamp).In(time.UTC))
	start, err := parsePeriod(end, req.Period)
	if err != nil {
		http.Error(w, "invalid period: "+err.Error(), http.StatusBadRequest)
		return
	}
	var timelines []*PositionTimeline
	for _, assetId := range req.AssetIDs {
		a, ok := s.Store().assets[assetId]
		if !ok {
			continue
		}
		positions := s.Store().AssetPositionsBetween(a.ID(), start, end)
		t := &PositionTimeline{
			AssetID:   a.ID(),
			AssetName: a.Name,
		}
		for _, p := range positions {
			t.Timestamps = append(t.Timestamps, p.LastUpdated.UnixMilli())
			t.QuantityMicros = append(t.QuantityMicros, int64(p.QuantityMicros))
			t.ValueMicros = append(t.ValueMicros, int64(p.MarketValue()))
		}
		timelines = append(timelines, t)
	}
	if len(timelines) == 0 {
		s.jsonResponse(w, PositionTimelineResponse{
			Status: StatusInvalidArgument,
			Error:  fmt.Sprintf("No assets found for given %d IDs", len(req.AssetIDs)),
		})
		return
	}
	s.jsonResponse(w, PositionTimelineResponse{
		Status:    StatusOK,
		Timelines: timelines,
	})
}

func (s *Server) handlePositionsMaturities(w http.ResponseWriter, r *http.Request) {
	var req PositionsMaturitiesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	date := ToDate(time.UnixMilli(req.EndTimestamp).In(time.UTC))
	rows := maturingPositionTableRows(s.Store(), date)

	// Calculate total value for each year-bucket defined by these bounds.
	bounds := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20, 30, 50, 100}
	buckets := make([]int64, len(bounds))
	for _, row := range rows {
		if row.YearsToMaturity < 0 {
			continue
		}
		b := int(math.Floor(row.YearsToMaturity))
		j := sort.Search(len(bounds), func(i int) bool {
			return bounds[i] > b
		})
		buckets[j-1] += int64(row.Value)
	}
	// Drop empty trailing buckets.
	maxIdx := len(buckets) - 1
	for maxIdx >= 0 && buckets[maxIdx] == 0 {
		maxIdx--
	}
	buckets = buckets[:maxIdx+1]
	bucketLabels := make([]string, len(buckets))
	for i := range bucketLabels {
		if i < len(bounds)-1 {
			bucketLabels[i] = fmt.Sprintf("%d..%d", bounds[i], bounds[i+1])
		} else {
			bucketLabels[i] = fmt.Sprintf(">= %d", bounds[i])
		}
	}
	s.jsonResponse(w, PositionsMaturitiesResponse{
		Status: StatusOK,
		Maturities: &MaturitiesChartData{
			Currency:     string(s.Store().BaseCurrency()),
			BucketLabels: bucketLabels,
			Values: []*MaturitiesChartValues{
				{
					Label:       "All maturing assets",
					ValueMicros: buckets,
				},
			},
		},
	})
}

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	date, ok := ensureDateParam(w, r)
	if !ok {
		return
	}
	var buf bytes.Buffer
	err := s.renderPositionsTemplate(&buf, r, date)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleMaturingPositions(w http.ResponseWriter, r *http.Request) {
	date, ok := ensureDateParam(w, r)
	if !ok {
		return
	}
	var buf bytes.Buffer
	err := s.renderMaturingPositionsTemplate(&buf, r, date)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleEquityPositions(w http.ResponseWriter, r *http.Request) {
	date, ok := ensureDateParam(w, r)
	if !ok {
		return
	}
	var buf bytes.Buffer
	err := s.renderEquityPositionsTemplate(&buf, r, date)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleCsvUpload(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	err := s.renderUploadCsvTemplate(&buf, r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) jsonResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		log.Fatalf("Cannot encode my own JSON: %s", err)
	}
}

func (s *Server) handleCsvPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(4 * (1 << 20)); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if len(r.MultipartForm.File["file"]) != 1 {
		http.Error(w, "must upload exactly one CSV file", http.StatusBadRequest)
		return
	}
	file := r.MultipartForm.File["file"][0]
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".csv" && ext != ".txt" {
		http.Error(w, fmt.Sprintf("Invalid file extension: %s", ext), http.StatusBadRequest)
		return
	}
	f, err := file.Open()
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid form data: %v", err), http.StatusBadRequest)
		return
	}
	// Assume ISO 8859-15 encoding.
	enc := charmap.ISO8859_15.NewDecoder().Reader(f)
	items, err := ReadDepotExportCSV(enc)
	if err != nil {
		http.Error(w, fmt.Sprintf("error processing CSV: %v", err), http.StatusBadRequest)
		return
	}
	validItems := make([]*DepotExportItem, 0, len(items))
	var skipped []string
	for _, item := range items {
		if a := s.Store().FindAssetByWKN(item.WKN); a == nil {
			skipped = append(skipped, item.WKN)
			continue
		}
		validItems = append(validItems, item)
	}
	var buf bytes.Buffer
	if err := s.renderSnipUploadCsvData(&buf, validItems, s.Store()); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	status := StatusOK
	if len(validItems) == 0 {
		status = StatusInvalidArgument
	} else if len(validItems) != len(items) {
		status = StatusPartialSuccess
	}
	errorText := ""
	if len(skipped) > 0 {
		errorText = fmt.Sprintf("Successfully read %d rows. Skipped WKNs: %s", len(validItems),
			strings.Join(skipped, ", "))
	}
	s.jsonResponse(w, CsvUploadResponse{
		Status:     status,
		Error:      errorText,
		NumEntries: len(validItems),
		InnerHTML:  buf.String(),
	})
}

func (s *Server) handleEntriesAdd(w http.ResponseWriter, r *http.Request) {
	var e LedgerEntry
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Store().Add(&e); err != nil {
		s.jsonResponse(w, AddLedgerEntryResponse{
			Status: StatusInvalidArgument,
			Error:  err.Error(),
		})
		return
	}
	if err := s.Store().Save(); err != nil {
		http.Error(w, fmt.Sprintf("Error saving ledger: %v", err), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, AddLedgerEntryResponse{
		Status:      StatusOK,
		SequenceNum: e.SequenceNum,
	})
}

func (s *Server) handleEntriesDelete(w http.ResponseWriter, r *http.Request) {
	var req DeleteLedgerEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SequenceNum == nil {
		http.Error(w, "sequenceNum must be set", http.StatusBadRequest)
		return
	}
	if err := s.Store().Delete(*req.SequenceNum); err != nil {
		s.jsonResponse(w, DeleteLedgerEntryResponse{
			Status: StatusInvalidArgument,
			Error:  err.Error(),
		})
		return
	}
	if err := s.Store().Save(); err != nil {
		http.Error(w, fmt.Sprintf("Error saving ledger: %v", err), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, DeleteLedgerEntryResponse{
		Status:      StatusOK,
		SequenceNum: *req.SequenceNum,
	})
}

func (s *Server) handleAssetsPost(w http.ResponseWriter, r *http.Request) {
	var req UpsertAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Asset == nil {
		http.Error(w, "missing asset", http.StatusBadRequest)
		return
	}
	if req.AssetID == "" {
		// Insert
		if err := s.Store().AddAsset(req.Asset); err != nil {
			s.jsonResponse(w, UpsertAssetResponse{
				Status: StatusInvalidArgument,
				Error:  err.Error(),
			})
			return
		}
	} else {
		// Update
		if err := s.Store().UpdateAsset(req.AssetID, req.Asset); err != nil {
			s.jsonResponse(w, UpsertAssetResponse{
				Status: StatusInvalidArgument,
				Error:  err.Error(),
			})
			return
		}
	}
	if err := s.Store().Save(); err != nil {
		http.Error(w, fmt.Sprintf("Error saving ledger: %v", err), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, UpsertAssetResponse{
		Status:  StatusOK,
		AssetID: req.Asset.ID(),
	})
}

func (s *Server) createLedgerEntries(r *AddQuotesRequest) ([]*LedgerEntry, error) {
	result := make([]*LedgerEntry, 0, len(r.Quotes)+len(r.ExchangeRates))
	for _, q := range r.Quotes {
		a, ok := s.Store().assets[q.AssetID]
		if !ok {
			return nil, fmt.Errorf("asset %q does not exist", q.AssetID)
		}
		result = append(result, &LedgerEntry{
			Type:        AssetPrice,
			ValueDate:   q.Date,
			AssetID:     q.AssetID,
			Currency:    a.Currency,
			PriceMicros: q.PriceMicros,
		})

	}
	for _, e := range r.ExchangeRates {
		if !currencyRegexp.MatchString(string(e.BaseCurrency)) {
			return nil, fmt.Errorf("invalid currency: %q", e.BaseCurrency)
		}
		if !currencyRegexp.MatchString(string(e.QuoteCurrency)) {
			return nil, fmt.Errorf("invalid currency: %q", e.QuoteCurrency)
		}
		if e.PriceMicros <= 0 {
			return nil, fmt.Errorf("exchange rate must be postive: %v", e.PriceMicros)
		}
		result = append(result, &LedgerEntry{
			Type:          ExchangeRate,
			ValueDate:     e.Date,
			Currency:      e.BaseCurrency,
			QuoteCurrency: e.QuoteCurrency,
			PriceMicros:   e.PriceMicros,
		})
	}
	return result, nil
}

func (s *Server) handleQuotesPost(w http.ResponseWriter, r *http.Request) {
	var req AddQuotesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid input: "+err.Error(), http.StatusBadRequest)
		return
	}
	entries, err := s.createLedgerEntries(&req)
	if err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	imported := 0
	var failures []string
	for _, e := range entries {
		if err := s.Store().Add(e); err != nil {
			failures = append(failures, fmt.Sprintf("Failed to add entry: %s", err))
			continue
		}
		imported++
	}
	if imported > 0 {
		if err := s.Store().Save(); err != nil {
			http.Error(w, fmt.Sprintf("Error saving ledger: %v", err), http.StatusInternalServerError)
			return
		}
	}
	if len(failures) > 0 {
		status := StatusInvalidArgument
		if imported > 0 {
			status = StatusPartialSuccess
		}
		s.jsonResponse(w, AddQuotesResponse{
			Status:        status,
			Error:         strings.Join(failures, ",\n"),
			ItemsImported: imported,
		})
		return
	}
	s.jsonResponse(w, AddQuotesResponse{
		Status:        StatusOK,
		ItemsImported: imported,
	})
}

func (s *Server) reloadHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.debugMode {
			err := s.reloadTemplates()
			if err != nil {
				log.Fatalf("Failed to reload templates: %v", err)
			}
		}
		h.ServeHTTP(w, r)
	}
}

func jsonHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
			return
		}
		h.ServeHTTP(w, r)
	}
}

func (s *Server) createMux() *http.ServeMux {
	mux := &http.ServeMux{}
	// Serve static resources like CSS from resources/ dir.
	mux.Handle("/kontoo/resources/", http.StripPrefix("/kontoo/resources",
		http.FileServer(http.Dir(path.Join(s.baseDir, "resources")))))
	mux.Handle("/kontoo/dist/", http.StripPrefix("/kontoo/dist",
		http.FileServer(http.Dir(path.Join(s.baseDir, "dist")))))

	mux.HandleFunc("GET /kontoo/ledger", s.reloadHandler(s.handleLedger))
	mux.HandleFunc("GET /kontoo/positions", s.reloadHandler(s.handlePositions))
	mux.HandleFunc("GET /kontoo/positions/maturing", s.reloadHandler(s.handleMaturingPositions))
	mux.HandleFunc("GET /kontoo/positions/equity", s.reloadHandler(s.handleEquityPositions))
	mux.HandleFunc("GET /kontoo/entries/new", s.reloadHandler(s.handleEntriesNew))
	mux.HandleFunc("GET /kontoo/assets/new", s.reloadHandler(s.handleAssetsNew))
	mux.HandleFunc("GET /kontoo/assets/edit/{assetID}", s.reloadHandler(s.handleAssetsEdit))
	mux.HandleFunc("GET /kontoo/csv/upload", s.reloadHandler(s.handleCsvUpload))
	// TODO: Use different path, e.g. /kontoo/quotes/history? (for consistency)
	mux.HandleFunc("GET /kontoo/quotes", s.reloadHandler(s.handleQuotes))
	mux.HandleFunc("POST /kontoo/positions/timeline", jsonHandler(s.handlePositionsTimeline))
	mux.HandleFunc("POST /kontoo/positions/maturities", jsonHandler(s.handlePositionsMaturities))
	mux.HandleFunc("POST /kontoo/entries/add", jsonHandler(s.handleEntriesAdd))
	mux.HandleFunc("POST /kontoo/entries/delete", jsonHandler(s.handleEntriesDelete))
	mux.HandleFunc("POST /kontoo/assets", jsonHandler(s.handleAssetsPost))
	mux.HandleFunc("POST /kontoo/csv", s.handleCsvPost)
	mux.HandleFunc("POST /kontoo/quotes", jsonHandler(s.handleQuotesPost))
	mux.HandleFunc("POST /kontoo/ledger/reload", s.reloadHandler(s.handleLedgerReload))
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/kontoo/ledger", http.StatusTemporaryRedirect)
	})
	return mux
}

func (s *Server) Serve() error {
	mux := s.createMux()
	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	fmt.Printf("Server available at http://%s/\n", s.addr)
	return srv.ListenAndServe()
}
