package server

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dnswlt/kontoo/internal/calc"
	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/quotes"
	"github.com/dnswlt/kontoo/internal/ui"
	"github.com/dnswlt/kontoo/internal/units"
	"github.com/dnswlt/kontoo/resources"
	"golang.org/x/text/encoding/charmap"
)

// JSON API for server requests and responses.
type UpsertLedgerEntryRequest struct {
	// Optional. If set, it is an update request, otherwise an add.
	UpdateExisting bool `json:"updateExisting"`
	// Optional. If nonzero, indicates how often the entry should be
	// repeated monthly. Mostly useful for repeated payments or distributing
	// tax estimates across a year.
	RepeatForMonths int                 `json:"repeatForMonths"`
	Entry           *ledger.LedgerEntry `json:"entry"`
}
type UpsertLedgerEntryResponse struct {
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
	AssetID string        `json:"assetId,omitempty"`
	Asset   *ledger.Asset `json:"asset"`
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
	AssetID     string       `json:"assetID"`
	Date        civil.Date   `json:"date"`
	PriceMicros units.Micros `json:"priceMicros"`
}
type AddExchangeRateItem struct {
	BaseCurrency  units.Currency `json:"baseCurrency"`
	QuoteCurrency units.Currency `json:"quoteCurrency"`
	Date          civil.Date     `json:"date"`
	PriceMicros   units.Micros   `json:"priceMicros"`
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

// PositionTimelineRequest asks for the value over time of each given asset,
// or of all assets combined if AssetIDs is empty.
type PositionTimelineRequest struct {
	AssetIDs     []string `json:"assetIds"`
	EndTimestamp int64    `json:"endTimestamp"`
	Period       string   `json:"period"`
}

// PositionTimeline contains the value over time of one asset or of all assets.
type PositionTimeline struct {
	AssetID    string  `json:"assetId,omitempty"` // Empty for the total of all assets.
	Label      string  `json:"label"`
	Timestamps []int64 `json:"timestamps"`
	// Values in the base currency. Sent as int64 micros: the JSON marshalling
	// of units.Micros would send them as strings (e.g. "12.3").
	ValueMicros []int64 `json:"valueMicros"`
}
type PositionTimelineResponse struct {
	Status    StatusCode          `json:"status"`
	Error     string              `json:"error,omitempty"`
	Currency  units.Currency      `json:"currency,omitempty"`
	Timelines []*PositionTimeline `json:"timelines,omitempty"`
}
type PositionsMaturitiesRequest struct {
	EndTimestamp int64 `json:"endTimestamp"`
}
type MaturitiesChartValues struct {
	Label       string  `json:"label"`
	ValueMicros []int64 `json:"valueMicros"`
}

// MaturitiesChartData contains the principal (nominal value) of maturing
// positions in the base currency, as int64 micros.
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

type EquityChartRequest struct {
	EndTimestamp int64 `json:"endTimestamp"`
}
type EquityChartResponse struct {
	Status      StatusCode `json:"status"`
	Currency    string     `json:"currency"`
	AssetNames  []string   `json:"assetNames"`
	ValueMicros []int64    `json:"valueMicros"`
}

type LedgerAssetInfoRequest struct {
	AssetID string      `json:"assetId"`
	Date    *civil.Date `json:"date"` // Optional
}
type LedgerAssetInfoResponse struct {
	Status    StatusCode `json:"status"`
	Error     string     `json:"error,omitempty"`
	InnerHTML string     `json:"innerHTML"`
}

type CalculateIRRRequest struct {
	NominalValue    units.Micros `json:"nominalValue"`
	PurchasePrice   units.Micros `json:"purchasePrice"`
	PurchaseDate    civil.Date   `json:"purchaseDate"`
	MaturityDate    civil.Date   `json:"maturityDate"`
	InterestRate    units.Micros `json:"interestRate"`
	Cost            units.Micros `json:"cost"`
	AccruedInterest bool         `json:"accruedInterest"` // Whether interest is accrued or paid annually.
}

type CalculateIRRResponse struct {
	Status       StatusCode `json:"status"`
	Error        string     `json:"error,omitempty"`
	IRRMicros    int64      `json:"irrMicros"`
	IRRFormatted string     `json:"irrFormatted"`
}

type StatusCode string

const (
	StatusOK              StatusCode = "OK"
	StatusPartialSuccess  StatusCode = "PARTIAL_SUCCESS"
	StatusInvalidArgument StatusCode = "INVALID_ARGUMENT"
)

// END JSON API

func ListEmbeddedResources() (files []string, err error) {
	err = fs.WalkDir(resources.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

type Server struct {
	addr       string // Address at which to serve, e.g. "localhost:8084"
	ledgerPath string
	// Directory from which static resources are read.
	// Use embedded resources if empty.
	baseDir   string
	templates *template.Template
	store     *ledger.Store
	debugMode bool
	// Stock quote service
	yFinance *quotes.YFinance
}

func NewServer(addr, ledgerPath, baseDir string) (*Server, error) {
	if baseDir != "" {
		expectedFiles := []string{
			"css/style.css",
			"templates/ledger.html",
		}
		for _, f := range expectedFiles {
			if _, err := os.Stat(path.Join(baseDir, f)); err != nil {
				return nil, fmt.Errorf("invalid baseDir %q: file %q not found: %w", baseDir, f, err)
			}
		}
	}
	store, err := ledger.LoadStore(ledgerPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load store: %w", err)
	}
	yf, err := quotes.NewYFinance()
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

func (s *Server) Store() *ledger.Store {
	return s.store
}

func (s *Server) ReloadStore() error {
	store, err := ledger.LoadStore(s.ledgerPath)
	if err != nil {
		return err
	}
	s.store = store
	return nil
}

func (s *Server) DebugMode(enabled bool) {
	s.debugMode = enabled
	if s.yFinance != nil {
		s.yFinance.EnableTracing(enabled)
	}
}

func (s *Server) useEmbedded() bool {
	return s.baseDir == ""
}

func (s *Server) reloadTemplates() error {
	var tmpl *template.Template
	var err error
	if s.useEmbedded() {
		// Use embedded templates
		glob := "templates/*.html"
		tmpl, err = template.New("__root__").Funcs(ui.CommonFuncs()).ParseFS(resources.Files, glob)
	} else {
		// Use templates from file system.
		glob := path.Join(s.baseDir, "templates", "*.html")
		tmpl, err = template.New("__root__").Funcs(ui.CommonFuncs()).ParseGlob(glob)
	}
	if err != nil {
		return fmt.Errorf("could not parse templates: %w", err)
	}
	s.templates = tmpl
	return nil
}

type PositionTableRow struct {
	AssetID   string
	AssetName string
	AssetType ledger.AssetType
	Currency  units.Currency
	Value     units.Micros
	// BaseCurrency/Currency exchange rate. Used to calculate all monetary values
	// of this position in the base currency.
	ExchangeRate units.Micros
	// Notes about the position to be displayed to the user
	// (e.g. about old data being shown).
	Notes []string
	// Maximum age (in days) of the data on which the Value and ValueBaseCurrency
	// are calculated. Used to display warnings in the UI if the age is
	// above a threshold.
	DataAgeDays int

	PurchasePrice units.Micros

	// Only populated for equities:
	Quantity          units.Micros
	Price             units.Micros
	PriceDate         civil.Date
	ProfitLoss1Y      units.Micros
	ProfitLoss1YBasis units.Micros // the basis value relative to which the 1Y P&L ratio is calculated.

	// Only populated for maturing assets:
	NominalValue            units.Micros
	InterestRate            units.Micros
	IssueDate               *civil.Date
	MaturityDate            *civil.Date
	TotalEarningsAtMaturity units.Micros
	InternalRateOfReturn    units.Micros
	YearsToMaturity         float64
}

func (r *PositionTableRow) ProfitLoss() units.Micros {
	return r.Value - r.PurchasePrice
}

func (r *PositionTableRow) ProfitLossRatio() units.Micros {
	if r.PurchasePrice == 0 {
		return 0
	}
	return units.FloatAsMicros(r.Value.Float()/r.PurchasePrice.Float() - 1)
}

func (r *PositionTableRow) ProfitLoss1YRatio() units.Micros {
	if r.ProfitLoss1YBasis == 0 {
		return 0
	}
	return r.ProfitLoss1Y.Div(r.ProfitLoss1YBasis)
}

func (r *PositionTableRow) AssetCategory() ledger.AssetCategory {
	return r.AssetType.Category()
}

// Convenience function for sorting *Date. Nils come last.
func compareDatePtr(l, r *civil.Date) int {
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

func maturingPositionTableRows(s *ledger.Store, date civil.Date) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	slices.SortFunc(positions, func(a, b *ledger.AssetPosition) int {
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
			TotalEarningsAtMaturity: calc.TotalEarningsAtMaturity(p),
			InternalRateOfReturn:    calc.InternalRateOfReturn(p),
			YearsToMaturity:         float64(a.MaturityDate.DaysSince(date)) / 365,
		}
		res = append(res, row)
	}
	return res
}

// maturityPrincipal returns the amount repaid at maturity: the nominal value of
// bonds, and for fixed deposits (which have no quantity) the credited amounts,
// excluding interest recorded through later balance updates.
func maturityPrincipal(r *PositionTableRow) units.Micros {
	if r.AssetType.IsAccountType() {
		return r.PurchasePrice
	}
	return r.NominalValue
}

// missingExchangeRates returns the currencies of rows that have no exchange rate
// to the base currency, sorted and without duplicates.
func missingExchangeRates(rows []*PositionTableRow) []units.Currency {
	var missing []units.Currency
	for _, r := range rows {
		if r.ExchangeRate == 0 && !slices.Contains(missing, r.Currency) {
			missing = append(missing, r.Currency)
		}
	}
	slices.Sort(missing)
	return missing
}

func equityPositionTableRows(s *ledger.Store, date civil.Date) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	slices.SortFunc(positions, func(a, b *ledger.AssetPosition) int {
		c := strings.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name()))
		if c != 0 {
			return c
		}
		return strings.Compare(a.ID(), b.ID())
	})
	var res []*PositionTableRow
	for _, p := range positions {
		a := p.Asset
		if a.Category() != ledger.Equity {
			continue
		}
		profitLoss1Y, profitLoss1YBasis, err := s.ProfitLossInPeriod(a.ID(), date.AddDays(-364), date)
		if err != nil {
			// TODO: remove once we're happy with the results.
			log.Print("Cannot calculate profit&loss over 1y period:", err)
		}
		rate, _, _ := s.ExchangeRateAt(a.Currency, date)
		row := &PositionTableRow{
			AssetID:           a.ID(),
			AssetName:         a.Name,
			AssetType:         a.Type,
			Currency:          a.Currency,
			ExchangeRate:      rate,
			Value:             p.MarketValue(),
			Quantity:          p.QuantityMicros,
			Price:             p.PriceMicros,
			PriceDate:         p.PriceDate,
			ProfitLoss1Y:      profitLoss1Y,
			ProfitLoss1YBasis: profitLoss1YBasis,
			PurchasePrice:     p.PurchasePrice(),
		}
		res = append(res, row)
	}
	return res
}

func positionTableRows(s *ledger.Store, date civil.Date) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	slices.SortFunc(positions, func(a, b *ledger.AssetPosition) int {
		c := int(a.Asset.Category()) - int(b.Asset.Category())
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
			DataAgeDays:  date.DaysSince(p.LastUpdated),
		}
	}
	return res
}

type PositionTableRowGroup struct {
	Category ledger.AssetCategory
	Rows     []*PositionTableRow
	// Only set if calculated across all row groups:
	FractionOfTotal units.Micros
}

func (g *PositionTableRowGroup) ValueBaseCurrency() units.Micros {
	var sum units.Micros
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
	currentCategory := rows[0].AssetCategory()
	res = append(res, &PositionTableRowGroup{
		Category: currentCategory,
		Rows:     []*PositionTableRow{rows[0]},
	})
	for _, row := range rows[1:] {
		if row.AssetCategory() == currentCategory {
			res[len(res)-1].Rows = append(res[len(res)-1].Rows, row)
		} else {
			res = append(res, &PositionTableRowGroup{
				Category: row.AssetCategory(),
				Rows:     []*PositionTableRow{row},
			})
			currentCategory = row.AssetCategory()
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

// isFormPage reports whether p is an entry or asset form, which return to
// the page they were opened from instead of serving as return targets.
func isFormPage(p string) bool {
	p = path.Clean(p)
	return strings.HasPrefix(p, "/kontoo/entries/") || strings.HasPrefix(p, "/kontoo/assets/")
}

func safeReturnURL(raw, fallback string) string {
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || strings.Contains(raw, "\\") ||
		!strings.HasPrefix(path.Clean(u.Path), "/kontoo/") || isFormPage(u.Path) {
		return fallback
	}
	// Return to the full page, not the table snippet a page may have been fetched as.
	if q := u.Query(); q.Has("snippet") {
		q.Del("snippet")
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (s *Server) addCommonCtx(r *http.Request, ctx map[string]any) map[string]any {
	addP := func(q url.Values, key, value string) url.Values {
		r := make(url.Values)
		for k, v := range q {
			r[k] = v
		}
		r.Add(key, value)
		return r
	}
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
	returnURL := safeReturnURL(r.URL.RequestURI(), newURL("/kontoo/ledger", ctxQ).String())
	if isFormPage(r.URL.Path) {
		returnURL = safeReturnURL(q.Get("return_to"), newURL("/kontoo/ledger", ctxQ).String())
	}
	ctx["ReturnURL"] = returnURL
	formQ := addP(ctxQ, "return_to", returnURL)
	ctx["Nav"] = map[string]string{
		"ledger":        newURL("/kontoo/ledger", ctxQ).String(),
		"positions":     newURL("/kontoo/positions", ctxQ).String(),
		"addEntry":      newURL("/kontoo/entries/new", formQ).String(),
		"updateBalance": newURL("/kontoo/entries/new", addP(formQ, "prefill", "balance")).String(),
		"addAsset":      newURL("/kontoo/assets/new", formQ).String(),
		"editAsset":     newURL("/kontoo/assets/edit/{assetID}", formQ).String(),
		"uploadCSV":     newURL("/kontoo/csv/upload", ctxQ).String(),
		"quotes":        newURL("/kontoo/quotes", ctxQ).String(),
		"calc":          newURL("/kontoo/calc", ctxQ).String(),
	}
	return ctx
}

func (s *Server) renderEntryTemplate(w io.Writer, r *http.Request, entry *ledger.LedgerEntry, date civil.Date) error {
	assets := make([]*ledger.Asset, len(s.Store().AllAssets()))
	copy(assets, s.Store().AllAssets())
	slices.SortFunc(assets, func(a, b *ledger.Asset) int {
		return strings.Compare(a.Name, b.Name)
	})
	quoteCurrenciesMap := make(map[units.Currency]bool)
	var quoteCurrencies []units.Currency
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
	if entry == nil {
		entry = &ledger.LedgerEntry{
			ValueDate: date, // Prepopulate the value date with the context date.
		}
		// Populate input field values from query params
		q := r.URL.Query()
		entry.AssetID = q.Get("AssetID")
		if prefill := q.Get("prefill"); prefill != "" {
			switch prefill {
			case "balance":
				entry.Type = ledger.AccountBalance
				if p := s.Store().AssetPositionAt(entry.AssetID, date); p != nil {
					entry.ValueMicros = p.MarketValue()
				}
			}
		} else if t, err := ledger.ParseEntryTypeString(q.Get("Type")); err == nil {
			entry.Type = t
		}
	}
	ctx := s.addCommonCtx(r, map[string]any{
		"Today":           time.Now().Format("2006-01-02"),
		"Date":            date,
		"Assets":          assets,
		"BaseCurrency":    s.Store().BaseCurrency(),
		"QuoteCurrencies": quoteCurrencies,
		"EntryTypes":      ledger.EntryTypeValues()[1:],
		"Entry":           entry,
	})
	return s.templates.ExecuteTemplate(w, "entry.html", ctx)
}

// assetFormFields returns the IDs of the type-specific asset form fields
// that apply to assets of type t. Fields that apply to all types (name,
// currency, custom ID, ...) are not included.
func assetFormFields(t ledger.AssetType) []string {
	securityIDs := []string{"ISIN", "WKN", "TickerSymbol", "YFSymbol"}
	accountIDs := []string{"IBAN", "AccountNumber"}
	terms := []string{"IssueDate", "MaturityDate", "Interest", "InterestPayment"}
	switch t {
	case ledger.Stock, ledger.StockExchangeTradedFund, ledger.StockMutualFund,
		ledger.BondExchangeTradedFund, ledger.BondMutualFund, ledger.Commodity:
		return securityIDs
	case ledger.CorporateBond, ledger.GovernmentBond:
		return slices.Concat(securityIDs, terms)
	case ledger.FixedDepositAccount:
		return slices.Concat(accountIDs, terms)
	case ledger.MoneyMarketAccount, ledger.SavingsAccount, ledger.CheckingAccount,
		ledger.BrokerageAccount, ledger.PensionAccount:
		return append(accountIDs, "Interest")
	case ledger.CreditCardDebt:
		return accountIDs
	}
	return nil // Cash, taxes, other debt: identified by custom ID only.
}

func (s *Server) renderAssetTemplate(w io.Writer, r *http.Request, asset *ledger.Asset) error {
	assetID := ""
	hasEntries := false
	if asset == nil {
		// Ensure template can render empty values. New assets default to the base currency.
		asset = &ledger.Asset{Currency: s.Store().BaseCurrency()}
	} else {
		assetID = asset.ID()
		hasEntries = s.Store().HasEntries(assetID)
	}
	type assetTypeOption struct {
		Value    ledger.AssetType
		Label    string
		Fields   []string
		Disabled bool
	}
	var assetTypes []assetTypeOption
	for _, t := range ledger.AssetTypeValues() {
		if t == ledger.UnspecifiedAssetType {
			continue
		}
		assetTypes = append(assetTypes, assetTypeOption{
			Value:  t,
			Label:  t.DisplayName(),
			Fields: assetFormFields(t),
			// The category of an asset with ledger entries is immutable.
			Disabled: hasEntries && t.Category() != asset.Category(),
		})
	}
	currencies := []units.Currency{s.Store().BaseCurrency()}
	for _, a := range s.Store().AllAssets() {
		if !slices.Contains(currencies, a.Currency) {
			currencies = append(currencies, a.Currency)
		}
	}
	slices.Sort(currencies)
	ctx := s.addCommonCtx(r, map[string]any{
		"AssetTypes": assetTypes,
		"Currencies": currencies,
		"Asset":      asset,
		"AssetID":    assetID,
		"HasEntries": hasEntries,
	})
	return s.templates.ExecuteTemplate(w, "asset.html", ctx)
}

type exchangeRateQuoteEntry struct {
	*quotes.DailyExchangeRate
	LatestDate  civil.Date
	DataAgeDays int
}

func (s *Server) exchangeRateQuoteEntries(rates []*quotes.DailyExchangeRate) []*exchangeRateQuoteEntry {
	entries := make([]*exchangeRateQuoteEntry, 0, len(rates))
	for _, rate := range rates {
		quoteDate := civil.ToDate(rate.Timestamp)
		_, latestDate, found := s.Store().ExchangeRateAt(rate.QuoteCurrency, quoteDate)
		entry := &exchangeRateQuoteEntry{DailyExchangeRate: rate, LatestDate: latestDate}
		if found {
			entry.DataAgeDays = quoteDate.DaysSince(latestDate)
		}
		entries = append(entries, entry)
	}
	return entries
}

func (s *Server) renderQuotesTemplate(w io.Writer, r *http.Request, date civil.Date) error {
	type QuoteEntry struct {
		AssetID      string
		AssetName    string
		Symbol       string
		Currency     units.Currency
		ClosingPrice units.Micros
		Date         time.Time
		LatestDate   civil.Date
		DataAgeDays  int
	}
	if s.yFinance == nil {
		// No quotes service, can't show quotes.
		return s.templates.ExecuteTemplate(w, "quotes.html", s.addCommonCtx(r, map[string]any{"QuoteDate": date}))
	}
	if date.After(civil.Today()) {
		return s.templates.ExecuteTemplate(w, "quotes.html", s.addCommonCtx(r, map[string]any{
			"QuoteDate": date,
			"Error":     "There are no quotes for future dates.",
		}))
	}
	assets := s.Store().FindAssetsForQuoteService("YF", date)
	entries := make([]*QuoteEntry, 0, len(assets))
	var errorMessage string
	for _, asset := range assets {
		symbol := asset.QuoteServiceSymbols["YF"]
		loc, err := s.Store().LookupTimezone(asset.ExchangeTimezone)
		if err != nil {
			log.Printf("Cannot get exchange timezone for asset %s: %v", asset.ID(), err)
			continue
		}
		// Request prices at 18:00 (EOD) of the requested date in the relevant time zone.
		t := time.Date(date.Year(), date.Month(), date.Day(), 18, 0, 0, 0, loc)
		h, err := s.yFinance.GetDailyQuote(symbol, t)
		if err != nil {
			log.Printf("Failed to get price history: %v", err)
			var connErr *url.Error
			if errors.As(err, &connErr) {
				errorMessage = "There was a problem getting prices from the quote service: " + err.Error()
				break // Give up on network issues
			}
			continue
		}
		_, priceDate, _ := s.Store().PriceAt(asset.ID(), civil.ToDate(h.Timestamp))
		entries = append(entries, &QuoteEntry{
			AssetID:      asset.ID(),
			AssetName:    asset.Name,
			Symbol:       h.Symbol,
			Currency:     h.Currency,
			ClosingPrice: h.ClosingPrice,
			Date:         h.Timestamp,
			LatestDate:   priceDate,
			DataAgeDays:  civil.ToDate(h.Timestamp).DaysSince(priceDate),
		})
	}
	quoteCurrencies := s.Store().QuoteCurrencies()
	exchangeRates := make([]*quotes.DailyExchangeRate, 0, len(quoteCurrencies))
	if errorMessage == "" {
		for _, qc := range quoteCurrencies {
			// Use UTC here on purpose: exchange rates in Y! are Europe/London (UTC/BST) based anyway.
			rate, err := s.yFinance.GetDailyExchangeRate(s.Store().BaseCurrency(), qc, date.In(time.UTC).Add(18*time.Hour))
			if err != nil {
				log.Printf("Failed to get exchange rate: %v", err)
				var connErr *url.Error
				if errors.As(err, &connErr) {
					errorMessage = "There was a problem getting prices from the quote service: " + err.Error()
					break // Give up on network issues
				}
				continue
			}
			exchangeRates = append(exchangeRates, rate)
		}
	}
	ctx := s.addCommonCtx(r, map[string]any{
		"QuoteDate":     date,
		"Entries":       entries,
		"ExchangeRates": s.exchangeRateQuoteEntries(exchangeRates),
		"Error":         errorMessage,
		// Without any YF symbols, the page suggests adding some.
		"HasQuoteSymbols": slices.ContainsFunc(s.Store().AllAssets(), func(a *ledger.Asset) bool {
			return a.QuoteServiceSymbols["YF"] != ""
		}),
	})
	return s.templates.ExecuteTemplate(w, "quotes.html", ctx)
}

func (s *Server) renderPositionsTemplate(w io.Writer, r *http.Request, date civil.Date) error {
	rows := positionTableRows(s.Store(), date)
	groups := positionTableRowGroups(rows)
	// Totals and the category chart need all values in the base currency.
	missingRates := missingExchangeRates(rows)
	// Category totals for the allocation chart, largest first, as int64 micros.
	type categoryChart struct {
		Currency units.Currency `json:"currency"`
		Labels   []string       `json:"labels"`
		Values   []int64        `json:"values"`
	}
	chart := categoryChart{Currency: s.Store().BaseCurrency()}
	var total units.Micros
	if len(missingRates) == 0 {
		for _, g := range groups {
			total += g.ValueBaseCurrency()
		}
		if total != 0 {
			for _, g := range groups {
				g.FractionOfTotal = g.ValueBaseCurrency().Div(total)
			}
		}
		byValue := slices.Clone(groups)
		slices.SortStableFunc(byValue, func(a, b *PositionTableRowGroup) int {
			return cmp.Compare(b.ValueBaseCurrency(), a.ValueBaseCurrency())
		})
		for _, g := range byValue {
			chart.Labels = append(chart.Labels, g.Category.DisplayName())
			chart.Values = append(chart.Values, int64(g.ValueBaseCurrency()))
		}
	}
	minDate, maxDate := s.Store().ValueDateRange()
	ctx := s.addCommonCtx(r, map[string]any{
		"TotalValueBaseCurrency": total,
		"Groups":                 groups,
		"MissingRates":           missingRates,
		"CategoryChart":          chart,
		"ActiveChips": map[string]bool{
			"all":   true,
			"today": date.Equal(civil.Today()),
		},
		"MonthOptions": ui.MonthOptions(*r.URL, date),
		"YearOptions":  ui.YearOptions(*r.URL, date, minDate, maxDate),
	})
	return s.templates.ExecuteTemplate(w, "positions.html", ctx)
}

func (s *Server) renderMaturingPositionsTemplate(w io.Writer, r *http.Request, date civil.Date) error {
	rows := maturingPositionTableRows(s.Store(), date)
	minDate, maxDate := s.Store().ValueDateRange()
	// Totals and the maturities chart need all values in the base currency.
	missingRates := missingExchangeRates(rows)
	var totals map[string]units.Micros
	if len(missingRates) == 0 {
		var totalValue, totalEarnings, totalIRR units.Micros
		for _, r := range rows {
			valueBC := r.Value.Div(r.ExchangeRate)
			totalValue += valueBC
			totalEarnings += r.TotalEarningsAtMaturity.Div(r.ExchangeRate)
			totalIRR += r.InternalRateOfReturn.Mul(valueBC)
		}
		if totalValue > 0 {
			totalIRR = totalIRR.Div(totalValue)
		}
		totals = map[string]units.Micros{
			"Value":                totalValue,
			"EarningsAtMaturity":   totalEarnings,
			"InternalRateOfReturn": totalIRR,
		}
	}
	ctx := s.addCommonCtx(r, map[string]any{
		"TableRows": rows,
		"ActiveChips": map[string]bool{
			"maturing": true,
			"today":    date.Equal(civil.Today()),
		},
		"Totals":       totals,
		"MissingRates": missingRates,
		"MonthOptions": ui.MonthOptions(*r.URL, date),
		"YearOptions":  ui.YearOptions(*r.URL, date, minDate, maxDate),
	})
	return s.templates.ExecuteTemplate(w, "positions_maturing.html", ctx)
}

func (s *Server) renderEquityPositionsTemplate(w io.Writer, r *http.Request, date civil.Date) error {
	rows := equityPositionTableRows(s.Store(), date)
	minDate, maxDate := s.Store().ValueDateRange()
	var totalValue, totalProfitLoss, totalPurchasePrice, totalPL1Y units.Micros
	var totalPL1YBasis units.Micros
	for _, r := range rows {
		if r.ExchangeRate == 0 {
			totalValue, totalProfitLoss, totalPurchasePrice, totalPL1Y = 0, 0, 0, 0
			break
		}
		totalValue += r.Value.Div(r.ExchangeRate)
		totalPurchasePrice += r.PurchasePrice.Div(r.ExchangeRate)
		totalProfitLoss += (r.Value - r.PurchasePrice).Div(r.ExchangeRate)
		totalPL1Y += r.ProfitLoss1Y.Div(r.ExchangeRate)
		totalPL1YBasis += r.ProfitLoss1YBasis.Div(r.ExchangeRate)
	}
	var totalProfitLossRatio units.Micros
	if totalPurchasePrice != 0 {
		totalProfitLossRatio = totalProfitLoss.Div(totalPurchasePrice)
	}
	var totalProfitLoss1YRatio units.Micros
	if totalPL1YBasis != 0 {
		totalProfitLoss1YRatio = totalPL1Y.Div(totalPL1YBasis)
	}
	report := s.Store().QuarterlyReport(date, 5)
	// Quarterly totals for the purchases and P&L charts, as int64 micros.
	type quarterlyChart struct {
		Currency   units.Currency `json:"currency"`
		Labels     []string       `json:"labels"`
		Purchases  []int64        `json:"purchases"`
		ProfitLoss []int64        `json:"profitLoss"`
		// False if all P&L totals are zero: then there is nothing to chart.
		HasProfitLoss bool `json:"-"`
	}
	chart := quarterlyChart{Currency: s.Store().BaseCurrency()}
	for _, d := range report.Data {
		chart.Labels = append(chart.Labels, d.Period.Label)
		chart.Purchases = append(chart.Purchases, int64(d.PurchasesTotal))
		chart.ProfitLoss = append(chart.ProfitLoss, int64(d.ProfitLossTotal))
		chart.HasProfitLoss = chart.HasProfitLoss || d.ProfitLossTotal != 0
	}
	ctx := s.addCommonCtx(r, map[string]any{
		"TableRows": rows,
		"ActiveChips": map[string]bool{
			"equity": true,
			"today":  date.Equal(civil.Today()),
		},
		"Totals": map[string]units.Micros{
			"Value":             totalValue,
			"PurchasePrice":     totalPurchasePrice,
			"ProfitLoss":        totalProfitLoss,
			"ProfitLossRatio":   totalProfitLossRatio,
			"ProfitLoss1Y":      totalPL1Y,
			"ProfitLoss1YRatio": totalProfitLoss1YRatio,
		},
		"MonthOptions":    ui.MonthOptions(*r.URL, date),
		"YearOptions":     ui.YearOptions(*r.URL, date, minDate, maxDate),
		"QuarterlyReport": report,
		"QuarterlyChart":  chart,
	})
	return s.templates.ExecuteTemplate(w, "positions_equity.html", ctx)
}

func (s *Server) renderUploadCsvTemplate(w io.Writer, r *http.Request) error {
	return s.templates.ExecuteTemplate(w, "upload_csv.html", s.addCommonCtx(r, map[string]any{}))
}

func (s *Server) renderCalcTemplate(w io.Writer, r *http.Request) error {
	return s.templates.ExecuteTemplate(w, "calc.html", s.addCommonCtx(r, map[string]any{
		"DefaultMaturity": time.Now().AddDate(1, 0, 0).Format("2006-01-02"),
	}))
}

func (s *Server) renderSnipUploadCsvData(w io.Writer, items []*quotes.DepotExportItem, store *ledger.Store) error {
	type Row struct {
		AssetID               string
		AssetName             string
		ValueDate             civil.Date
		PriceMicros           units.Micros
		Currency              units.Currency
		QuantityImportMicros  units.Micros
		QuantityCurrentMicros units.Micros
		Preselect             bool
		PriceDate             civil.Date
		DataAgeDays           int
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
			Preselect:             p.PriceDate.Before(item.ValueDate),
			PriceDate:             p.PriceDate,
			DataAgeDays:           item.ValueDate.DaysSince(p.PriceDate),
		})
	}
	return s.templates.ExecuteTemplate(w, "snip_upload_csv_data.html", map[string]any{
		"Entries": rows,
	})
}

func (s *Server) renderSnipAssetInfo(w io.Writer, asset *ledger.Asset, date civil.Date) error {
	assetID := asset.ID()
	entriesBefore, entriesAfter := s.Store().EntriesAround(assetID, date, 3)
	pos := s.Store().AssetPositionAt(assetID, date)
	return s.templates.ExecuteTemplate(w, "snip_asset_info.html", map[string]any{
		"Asset":         asset,
		"Position":      pos,
		"EntriesBefore": entriesBefore,
		"EntriesAfter":  entriesAfter,
	})
}

func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request) {
	ctx, queryErr := s.ledgerViewContext(r)
	tmpl := "ledger.html"
	if r.URL.Query().Get("snippet") == "true" {
		tmpl = "snip_ledger_table.html"
	}
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, tmpl, ctx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	if queryErr != nil {
		w.WriteHeader(http.StatusBadRequest)
	}
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
	if err := s.renderEntryTemplate(&buf, r, nil, date); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleEntriesEdit(w http.ResponseWriter, r *http.Request) {
	sequenceNum, err := strconv.ParseInt(r.PathValue("sequenceNum"), 10, 64)
	if err != nil {
		http.Error(w, "invalid sequenceNum", http.StatusBadRequest)
		return
	}
	e := s.Store().FindEntryBySequenceNum(sequenceNum)
	if e == nil {
		http.Error(w, fmt.Sprintf("no entry with sequenceNum %d", sequenceNum), http.StatusNotFound)
		return
	}
	var buf bytes.Buffer
	if err := s.renderEntryTemplate(&buf, r, e, civil.Today()); err != nil {
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
		http.Error(w, "missing assetID", http.StatusBadRequest)
		return
	}
	asset, ok := s.Store().Asset(assetID)
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
	if err := s.renderQuotesTemplate(&buf, r, date); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func dateParam(r *http.Request) (civil.Date, error) {
	d := r.URL.Query().Get("date")
	if d == "" {
		return civil.Today(), nil
	}
	date, err := civil.ParseDate(d)
	if err != nil {
		return civil.Date{}, fmt.Errorf("invalid value for date= parameter: %q", d)
	}
	return date, nil
}

func ensureDateParam(w http.ResponseWriter, r *http.Request) (civil.Date, bool) {
	d := r.URL.Query().Get("date")
	if d == "" {
		now := civil.Today()
		q := r.URL.Query()
		q.Set("date", now.String())
		r.URL.RawQuery = q.Encode()
		http.Redirect(w, r, r.URL.String(), http.StatusSeeOther)
		return civil.Date{}, false
	}
	date, err := civil.ParseDate(d)
	if err != nil {
		http.Error(w, "invalid date: "+err.Error(), http.StatusBadRequest)
		return civil.Date{}, false
	}
	return date, true
}

func (s *Server) handlePositionsTimeline(w http.ResponseWriter, r *http.Request) {
	var req PositionTimelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	end := civil.ToDate(time.UnixMilli(req.EndTimestamp))
	start, err := ui.ParsePeriod(end, req.Period)
	if err != nil {
		http.Error(w, "invalid period: "+err.Error(), http.StatusBadRequest)
		return
	}
	timeline := func(assetID, label string, assetIDs []string) *PositionTimeline {
		t := &PositionTimeline{AssetID: assetID, Label: label}
		for _, p := range s.Store().ValueTimeline(assetIDs, start, end) {
			t.Timestamps = append(t.Timestamps, p.Date.UnixMilli())
			t.ValueMicros = append(t.ValueMicros, int64(p.ValueMicros))
		}
		return t
	}
	var timelines []*PositionTimeline
	if len(req.AssetIDs) == 0 {
		var all []string
		for _, a := range s.Store().AllAssets() {
			all = append(all, a.ID())
		}
		timelines = append(timelines, timeline("", "Total", all))
	}
	for _, assetID := range req.AssetIDs {
		if a, ok := s.Store().Asset(assetID); ok {
			timelines = append(timelines, timeline(a.ID(), a.Name, []string{a.ID()}))
		}
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
		Currency:  s.Store().BaseCurrency(),
		Timelines: timelines,
	})
}

func (s *Server) handleChartsEquity(w http.ResponseWriter, r *http.Request) {
	var req EquityChartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	date := civil.ToDate(time.UnixMilli(req.EndTimestamp))
	type allocation struct {
		name  string
		value units.Micros // In the base currency.
	}
	var allocations []allocation
	for _, r := range equityPositionTableRows(s.Store(), date) {
		if r.ExchangeRate == 0 {
			// Ignore assets whose value we cannot calculate in the base currency. Should not happen.
			log.Printf("handleChartsEquity: no exchange rate for asset %q", r.AssetID)
			continue
		}
		allocations = append(allocations, allocation{r.AssetName, r.Value.Div(r.ExchangeRate)})
	}
	// Largest first.
	slices.SortFunc(allocations, func(a, b allocation) int {
		if c := cmp.Compare(b.value, a.value); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	})
	var assetNames []string
	var valueMicros []int64
	for _, a := range allocations {
		assetNames = append(assetNames, a.name)
		valueMicros = append(valueMicros, int64(a.value))
	}
	s.jsonResponse(w, EquityChartResponse{
		Status:      StatusOK,
		Currency:    string(s.Store().BaseCurrency()),
		AssetNames:  assetNames,
		ValueMicros: valueMicros,
	})
}

func (s *Server) handlePositionsMaturities(w http.ResponseWriter, r *http.Request) {
	var req PositionsMaturitiesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	date := civil.ToDate(time.UnixMilli(req.EndTimestamp))
	rows := maturingPositionTableRows(s.Store(), date)
	if missing := missingExchangeRates(rows); len(missing) > 0 {
		s.jsonResponse(w, PositionsMaturitiesResponse{
			Status: StatusInvalidArgument,
			Error:  fmt.Sprintf("no exchange rate for %v as of %v", missing, date),
		})
		return
	}

	// Principal per bucket of time to maturity: the next six months, the rest
	// of the first year, years 2 to 4, and later. Bucket i holds maturities up
	// to upperMonths[i]; the last bucket all later ones.
	upperMonths := []float64{6, 12, 24, 36, 48}
	bucketLabels := []string{"0.5", "1", "2", "3", "4", "5+"}
	buckets := make([]int64, len(bucketLabels))
	for _, row := range rows {
		if row.YearsToMaturity < 0 {
			continue
		}
		i := sort.SearchFloat64s(upperMonths, row.YearsToMaturity*12)
		buckets[i] += int64(maturityPrincipal(row).Div(row.ExchangeRate))
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

func (s *Server) handlePositionsMaturing(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handlePositionsEquity(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleCalc(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	err := s.renderCalcTemplate(&buf, r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleCalculate(w http.ResponseWriter, r *http.Request) {
	var req CalculateIRRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.PurchaseDate.IsZero() || req.MaturityDate.IsZero() {
		s.jsonResponse(w, CalculateIRRResponse{
			Status: StatusInvalidArgument,
			Error:  "Purchase date and maturity date must not be empty",
		})
		return
	}
	if req.NominalValue == 0 || req.PurchasePrice == 0 {
		s.jsonResponse(w, CalculateIRRResponse{
			Status: StatusInvalidArgument,
			Error:  "Nominal value and price must be non-zero",
		})
		return
	}
	interestPayment := ledger.AnnualPayment
	if req.AccruedInterest {
		interestPayment = ledger.AccruedPayment
	}
	irrMicros, err := calc.IRRWithInterest(calc.IRRParams{
		NominalValue:    req.NominalValue,
		Price:           req.PurchasePrice,
		InterestRate:    req.InterestRate,
		Cost:            req.Cost,
		PurchaseDate:    req.PurchaseDate,
		MaturityDate:    req.MaturityDate,
		InterestPayment: interestPayment,
	})
	if err != nil {
		s.jsonResponse(w, CalculateIRRResponse{
			Status: StatusInvalidArgument,
			Error:  err.Error(),
		})
		return
	}
	s.jsonResponse(w, CalculateIRRResponse{
		Status:       StatusOK,
		IRRMicros:    int64(irrMicros),
		IRRFormatted: irrMicros.Format("()'.2%"),
	})
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
	items, err := quotes.ReadDepotExportCSV(enc)
	if err != nil {
		http.Error(w, fmt.Sprintf("error processing CSV: %v", err), http.StatusBadRequest)
		return
	}
	validItems := make([]*quotes.DepotExportItem, 0, len(items))
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

func (s *Server) handleEntriesPost(w http.ResponseWriter, r *http.Request) {
	var req UpsertLedgerEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Entry == nil {
		http.Error(w, "missing entry in request", http.StatusBadRequest)
		return
	}
	if req.RepeatForMonths > 1 && req.UpdateExisting {
		http.Error(w, "cannot update multiple entries", http.StatusBadRequest)
		return
	}
	if req.UpdateExisting {
		// Update
		if err := s.Store().Update(req.Entry); err != nil {
			s.jsonResponse(w, UpsertLedgerEntryResponse{
				Status: StatusInvalidArgument,
				Error:  err.Error(),
			})
			return
		}
	} else {
		// Insert
		var err error
		if req.RepeatForMonths > 1 {
			err = s.Store().AddMonthly(req.Entry, req.RepeatForMonths)
		} else {
			err = s.Store().Add(req.Entry)
		}
		if err != nil {
			s.jsonResponse(w, UpsertLedgerEntryResponse{
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
	s.jsonResponse(w, UpsertLedgerEntryResponse{
		Status:      StatusOK,
		SequenceNum: req.Entry.SequenceNum,
	})
}

func (s *Server) handleEntriesAssetInfo(w http.ResponseWriter, r *http.Request) {
	var req LedgerAssetInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return

	}
	var buf bytes.Buffer
	date := civil.Today()
	if req.Date != nil {
		date = *req.Date
	}
	asset, ok := s.Store().Asset(req.AssetID)
	if !ok {
		s.jsonResponse(w, LedgerAssetInfoResponse{
			Status: StatusInvalidArgument,
			Error:  fmt.Sprintf("No asset with ID %q", req.AssetID),
		})
		return
	}
	if err := s.renderSnipAssetInfo(&buf, asset, date); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, LedgerAssetInfoResponse{
		Status:    StatusOK,
		InnerHTML: buf.String(),
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
	// The form doesn't edit the exchange timezone: keep it while the symbol is unchanged.
	if old, ok := s.Store().Asset(req.AssetID); ok && req.Asset.ExchangeTimezone == "" &&
		old.QuoteServiceSymbols["YF"] == req.Asset.QuoteServiceSymbols["YF"] {
		req.Asset.ExchangeTimezone = old.ExchangeTimezone
	}
	// Try to retrieve timezone for quote service symbol, if it is not already set.
	if len(req.Asset.QuoteServiceSymbols) > 0 && req.Asset.QuoteServiceSymbols["YF"] != "" {
		if s.yFinance != nil && req.Asset.ExchangeTimezone == "" {
			qs, err := s.yFinance.FetchQuoteSummary(req.Asset.QuoteServiceSymbols["YF"])
			if err == nil {
				log.Printf("Adding timezone %q to asset %q", qs.ExchangeTimezone(), req.Asset.Name)
				req.Asset.ExchangeTimezone = qs.ExchangeTimezone()
			}
		}
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

func (s *Server) createLedgerEntries(r *AddQuotesRequest) ([]*ledger.LedgerEntry, error) {
	result := make([]*ledger.LedgerEntry, 0, len(r.Quotes)+len(r.ExchangeRates))
	for _, q := range r.Quotes {
		a, ok := s.Store().Asset(q.AssetID)
		if !ok {
			return nil, fmt.Errorf("asset %q does not exist", q.AssetID)
		}
		result = append(result, &ledger.LedgerEntry{
			Type:        ledger.AssetPrice,
			ValueDate:   q.Date,
			AssetID:     q.AssetID,
			Currency:    a.Currency,
			PriceMicros: q.PriceMicros,
		})

	}
	for _, e := range r.ExchangeRates {
		if !e.BaseCurrency.Valid() {
			return nil, fmt.Errorf("invalid currency: %q", e.BaseCurrency)
		}
		if !e.QuoteCurrency.Valid() {
			return nil, fmt.Errorf("invalid currency: %q", e.QuoteCurrency)
		}
		if e.PriceMicros <= 0 {
			return nil, fmt.Errorf("exchange rate must be postive: %v", e.PriceMicros)
		}
		result = append(result, &ledger.LedgerEntry{
			Type:          ledger.ExchangeRate,
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

func (s *Server) handleV1ListEntries(w http.ResponseWriter, r *http.Request) {
}
func (s *Server) handleV1GetEntry(w http.ResponseWriter, r *http.Request) {
}
func (s *Server) handleV1ListAssets(w http.ResponseWriter, r *http.Request) {
}
func (s *Server) handleV1GetAsset(w http.ResponseWriter, r *http.Request) {
}

func (s *Server) apiV1Handler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /kontoo/v1/entries", s.handleV1ListEntries)
	mux.HandleFunc("GET /kontoo/v1/entries/{entryID}", s.handleV1GetEntry)
	mux.HandleFunc("GET /kontoo/v1/assets", s.handleV1ListAssets)
	mux.HandleFunc("GET /kontoo/v1/assets/{assetID}", s.handleV1GetAsset)
	return mux
}

func (s *Server) createMux() *http.ServeMux {
	mux := http.NewServeMux()
	// Serve static resources like CSS from resources/ and dist/ dirs.
	if s.useEmbedded() {
		mux.Handle("/kontoo/images/", http.StripPrefix("/kontoo",
			http.FileServer(http.FS(resources.Files))))
		mux.Handle("/kontoo/dist/", http.StripPrefix("/kontoo",
			http.FileServer(http.FS(resources.Files))))
		mux.Handle("/kontoo/css/", http.StripPrefix("/kontoo",
			http.FileServer(http.FS(resources.Files))))
	} else {
		mux.Handle("/kontoo/images/", http.StripPrefix("/kontoo/images",
			http.FileServer(http.Dir(path.Join(s.baseDir, "images")))))
		mux.Handle("/kontoo/dist/", http.StripPrefix("/kontoo/dist",
			http.FileServer(http.Dir(path.Join(s.baseDir, "dist")))))
		mux.Handle("/kontoo/css/", http.StripPrefix("/kontoo/css",
			http.FileServer(http.Dir(path.Join(s.baseDir, "css")))))
	}

	mux.Handle("/kontoo/v1/", s.apiV1Handler())

	mux.HandleFunc("GET /kontoo/ledger", s.reloadHandler(s.handleLedger))
	mux.HandleFunc("GET /kontoo/positions", s.reloadHandler(s.handlePositions))
	mux.HandleFunc("GET /kontoo/positions/maturing", s.reloadHandler(s.handlePositionsMaturing))
	mux.HandleFunc("GET /kontoo/positions/equity", s.reloadHandler(s.handlePositionsEquity))
	mux.HandleFunc("GET /kontoo/entries/new", s.reloadHandler(s.handleEntriesNew))
	mux.HandleFunc("GET /kontoo/entries/edit/{sequenceNum}", s.reloadHandler(s.handleEntriesEdit))
	mux.HandleFunc("GET /kontoo/assets/new", s.reloadHandler(s.handleAssetsNew))
	mux.HandleFunc("GET /kontoo/assets/edit/{assetID}", s.reloadHandler(s.handleAssetsEdit))
	mux.HandleFunc("GET /kontoo/csv/upload", s.reloadHandler(s.handleCsvUpload))
	mux.HandleFunc("GET /kontoo/calc", s.reloadHandler(s.handleCalc))
	// TODO: Use different path, e.g. /kontoo/quotes/history? (for consistency)
	mux.HandleFunc("GET /kontoo/quotes", s.reloadHandler(s.handleQuotes))
	mux.HandleFunc("POST /kontoo/positions/timeline", jsonHandler(s.handlePositionsTimeline))
	mux.HandleFunc("POST /kontoo/positions/maturities", jsonHandler(s.handlePositionsMaturities))
	mux.HandleFunc("POST /kontoo/charts/equity", jsonHandler(s.handleChartsEquity))
	mux.HandleFunc("POST /kontoo/entries", jsonHandler(s.handleEntriesPost))
	mux.HandleFunc("POST /kontoo/entries/delete", jsonHandler(s.handleEntriesDelete))
	mux.HandleFunc("POST /kontoo/entries/assetinfo", jsonHandler(s.handleEntriesAssetInfo))
	mux.HandleFunc("POST /kontoo/assets", jsonHandler(s.handleAssetsPost))
	mux.HandleFunc("POST /kontoo/csv", s.handleCsvPost)
	mux.HandleFunc("POST /kontoo/quotes", jsonHandler(s.handleQuotesPost))
	mux.HandleFunc("POST /kontoo/calculate", jsonHandler(s.handleCalculate))
	mux.HandleFunc("POST /kontoo/ledger/reload", s.reloadHandler(s.handleLedgerReload))
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/kontoo/positions", http.StatusTemporaryRedirect)
	})
	return mux
}

func (s *Server) Serve() error {
	mux := s.createMux()
	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	fmt.Printf("Running kontoo server at http://%s/ for %s\n", s.addr, s.ledgerPath)
	return srv.ListenAndServe()
}
