package kontoo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
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

type AddAssetResponse struct {
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

type StatusCode string

const (
	StatusOK              StatusCode = "OK"
	StatusPartialSuccess  StatusCode = "PARTIAL_SUCCESS"
	StatusInvalidArgument StatusCode = "INVALID_ARGUMENT"
)

// END JSON API

type Server struct {
	addr         string
	ledgerPath   string
	resourcesDir string
	templatesDir string
	templates    *template.Template
	store        *Store
	debugMode    bool
	// Stock quote service
	yFinance *YFinance
}

func NewServer(addr, ledgerPath, resourcesDir, templatesDir string) (*Server, error) {
	if _, err := os.Stat(path.Join(resourcesDir, "style.css")); err != nil {
		return nil, fmt.Errorf("invalid resourcesDir: %w", err)
	}
	if _, err := os.Stat(path.Join(templatesDir, "ledger.html")); err != nil {
		return nil, fmt.Errorf("invalid templatesDir: %w", err)
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
		addr:         addr,
		ledgerPath:   ledgerPath,
		resourcesDir: resourcesDir,
		templatesDir: templatesDir,
		store:        store,
		yFinance:     yf,
	}
	if err := s.reloadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) Store() *Store {
	return s.store
}

func (s *Server) DebugMode(enabled bool) {
	s.debugMode = enabled
}

func (s *Server) reloadTemplates() error {
	tmpl, err := template.New("__root__").Funcs(commonFuncs()).ParseGlob(path.Join(s.templatesDir, "*.html"))
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

func (e *LedgerEntryRow) ValueDate() Date {
	return e.E.ValueDate
}
func (e *LedgerEntryRow) Created() time.Time {
	return e.E.Created
}

func (e *LedgerEntryRow) EntryType() string {
	return e.E.Type.String()

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
func (e *LedgerEntryRow) AssetType() string {
	if e.A == nil {
		return ""
	}
	return e.A.Type.String()
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
	for _, e := range s.L.Entries {
		r := &LedgerEntryRow{
			E: e,
			A: s.assetMap[e.AssetID],
		}
		if !query.Match(r) {
			continue
		}
		res = append(res, r)
	}
	return res
}

type PositionTableRow struct {
	ID       string
	Name     string
	Type     AssetType
	Currency Currency
	Value    Micros
	// The value expressed in the base currency, converted using
	// the latest available exchange rate. 0 if no exchange rate
	// was available.
	ValueBaseCurrency Micros
	Notes             []string
	// Maximum age of the data on which the Value and ValueBaseCurrency
	// are calculated. Used to display warnings in the UI if the age is
	// above a threshold.
	DataAge time.Duration

	PurchasePrice   Micros
	NominalValue    Micros
	InterestRate    Micros
	IssueDate       *Date
	MaturityDate    *Date
	YearsToMaturity float64
}

func maturingPositionTableRows(s *Store, date time.Time) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	slices.SortFunc(positions, func(a, b *AssetPosition) int {
		c := CompareDatePtr(a.Asset.MaturityDate, b.Asset.MaturityDate)
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
		row := &PositionTableRow{
			ID:              a.ID(),
			Name:            a.Name,
			Type:            a.Type,
			Currency:        a.Currency,
			Value:           p.CalculatedValueMicros(),
			PurchasePrice:   p.PurchasePrice(),
			NominalValue:    p.QuantityMicros,
			InterestRate:    a.InterestMicros,
			IssueDate:       a.IssueDate,
			MaturityDate:    a.MaturityDate,
			YearsToMaturity: a.MaturityDate.Sub(date).Hours() / 24 / 365,
		}
		res = append(res, row)
	}
	return res
}

type PositionTableRowGroup struct {
	Label string
	Rows  []*PositionTableRow
}

func (g *PositionTableRowGroup) ValueBaseCurrency() Micros {
	var sum Micros
	for _, r := range g.Rows {
		sum += r.ValueBaseCurrency
	}
	return sum
}

type groupingFunc func(*PositionTableRow) string

func positionTableRows(s *Store, date time.Time) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	slices.SortFunc(positions, func(a, b *AssetPosition) int {
		c := strings.Compare(assetTypeInfos[a.Asset.Type].category, assetTypeInfos[b.Asset.Type].category)
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
		var lastUpdate Date
		a := p.Asset
		var notes []string
		val := p.CalculatedValueMicros()
		if !p.LastUpdate.IsZero() {
			notes = append(notes, fmt.Sprintf("Last updated: %s", p.LastUpdate))
			lastUpdate = p.LastUpdate
		}
		bval := val
		if a.Currency != s.BaseCurrency() {
			rate, rdate, err := s.ExchangeRateAt(a.Currency, toDate(date))
			if err != nil {
				// TODO: add error info to row
				log.Printf("No exchange rate at %v for %s: %s", date, a.Currency, err)
				bval = 0
			} else {
				bval = val.Frac(UnitValue, rate)
				notes = append(notes, fmt.Sprintf("Exch. rate date: %s", rdate))
				if lastUpdate.IsZero() || rdate.Before(lastUpdate.Time) {
					lastUpdate = rdate
				}
			}
		}
		res[i] = &PositionTableRow{
			ID:                a.ID(),
			Name:              a.Name,
			Type:              a.Type,
			Currency:          a.Currency,
			Value:             val,
			ValueBaseCurrency: bval,
			Notes:             notes,
			DataAge:           date.Sub(lastUpdate.Time),
		}
	}
	return res
}

func positionTableRowGroups(rows []*PositionTableRow, groupKey groupingFunc) []*PositionTableRowGroup {
	var res []*PositionTableRowGroup
	if len(rows) == 0 {
		return res
	}
	label := groupKey(rows[0])
	res = append(res, &PositionTableRowGroup{
		Label: label,
		Rows:  []*PositionTableRow{rows[0]},
	})
	for _, row := range rows[1:] {
		l := groupKey(row)
		if l == label {
			res[len(res)-1].Rows = append(res[len(res)-1].Rows, row)
		} else {
			res = append(res, &PositionTableRowGroup{
				Label: l,
				Rows:  []*PositionTableRow{row},
			})
			label = l
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

func (s *Server) renderLedgerTemplate(w io.Writer, store *Store, query *Query, snippet bool) error {
	rows := LedgerEntryRows(store, query)
	// Sort ledger rows by (ValueDate, Created) for output table.
	slices.SortFunc(rows, func(a, b *LedgerEntryRow) int {
		c := a.E.ValueDate.Time.Compare(b.E.ValueDate.Time)
		if c != 0 {
			return c
		}
		return a.E.Created.Compare(b.E.Created)
	})
	tmpl := "ledger.html"
	if snippet {
		tmpl = "snip_ledger_table.html"
	}
	return s.templates.ExecuteTemplate(w, tmpl, struct {
		TableRows []*LedgerEntryRow
		Query     string
		Now       string
	}{
		TableRows: rows,
		Query:     query.raw,
		Now:       time.Now().Format("2006-01-02 15:04:05"),
	})
}

func (s *Server) renderAddEntryTemplate(w io.Writer, date Date, store *Store) error {
	assets := make([]*Asset, len(store.L.Assets))
	copy(assets, store.L.Assets)
	slices.SortFunc(assets, func(a, b *Asset) int {
		return strings.Compare(a.Name, b.Name)
	})
	quoteCurrenciesMap := make(map[Currency]bool)
	var quoteCurrencies []Currency
	for _, a := range assets {
		if a.Currency == store.BaseCurrency() {
			continue
		}
		if _, ok := quoteCurrenciesMap[a.Currency]; !ok {
			quoteCurrencies = append(quoteCurrencies, a.Currency)
			quoteCurrenciesMap[a.Currency] = true
		}
	}
	slices.Sort(quoteCurrencies)
	return s.templates.ExecuteTemplate(w, "add_entry.html", map[string]any{
		"Today":           time.Now().Format("2006-01-02"),
		"Date":            date,
		"Assets":          assets,
		"BaseCurrency":    store.BaseCurrency(),
		"QuoteCurrencies": quoteCurrencies,
		"EntryTypes":      EntryTypeValues()[1:],
	})
}

func (s *Server) renderAddAssetTemplate(w io.Writer) error {
	assetTypeVals := AssetTypeValues()
	assetTypes := make([]string, 0, len(assetTypeVals))
	for _, a := range assetTypeVals {
		if a == UnspecifiedAssetType {
			continue
		}
		assetTypes = append(assetTypes, a.String())
	}
	return s.templates.ExecuteTemplate(w, "add_asset.html", struct {
		Today      string
		AssetTypes []string
	}{
		Today:      time.Now().Format("2006-01-02"),
		AssetTypes: assetTypes,
	})
}

func (s *Server) renderQuotesTemplate(w io.Writer, date time.Time) error {
	type QuoteEntry struct {
		AssetID      string
		AssetName    string
		Symbol       string
		Currency     Currency
		ClosingPrice Micros
		Date         time.Time
	}
	type TemplateData struct {
		Date          string
		Entries       []*QuoteEntry
		ExchangeRates []*DailyExchangeRate
	}
	if s.yFinance == nil {
		return s.templates.ExecuteTemplate(w, "quotes.html", TemplateData{
			Date: date.Format("2006-01-02"),
		})
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
	quoteCurrencies := s.Store().FindQuoteCurrencies()
	if len(quoteCurrencies) > 0 {
		var err error
		exchangeRates, err = s.yFinance.GetDailyExchangeRates(s.Store().BaseCurrency(), quoteCurrencies, date)
		if err != nil {
			log.Printf("Failed to get exchange rates: %v", err)
		}
	}
	return s.templates.ExecuteTemplate(w, "quotes.html", TemplateData{
		Date:          date.Format("2006-01-02"),
		Entries:       entries,
		ExchangeRates: exchangeRates,
	})
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
	ctx["Nav"] = map[string]string{
		"ledger":    newURL("/kontoo/ledger", ctxQ).String(),
		"positions": newURL("/kontoo/positions", ctxQ).String(),
		"addEntry":  newURL("/kontoo/entries/new", ctxQ).String(),
		"addAsset":  newURL("/kontoo/assets/new", ctxQ).String(),
		"uploadCSV": newURL("/kontoo/csv/upload", ctxQ).String(),
		"quotes":    newURL("/kontoo/quotes", ctxQ).String(),
	}
	return ctx
}

func (s *Server) renderPositionsTemplate(w io.Writer, r *http.Request, date time.Time) error {
	positions := positionTableRows(s.Store(), date)
	groups := positionTableRowGroups(positions, func(r *PositionTableRow) string {
		return assetTypeInfos[r.Type].category
	})
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
			"today": toDate(date).Equal(today()),
		},
		"MonthOptions": monthOptions(*r.URL, date, maxDate),
		"YearOptions":  yearOptions(*r.URL, date, minDate, maxDate),
	})
	return s.templates.ExecuteTemplate(w, "positions.html", ctx)
}

func (s *Server) renderMaturingPositionsTemplate(w io.Writer, r *http.Request, date time.Time) error {
	positions := maturingPositionTableRows(s.Store(), date)
	minDate, maxDate := s.Store().ValueDateRange()
	ctx := s.addCommonCtx(r, map[string]any{
		"TableRows": positions,
		"ActiveChips": map[string]bool{
			"maturing": true,
			"today":    toDate(date).Equal(today()),
		},
		"MonthOptions": monthOptions(*r.URL, date, maxDate),
		"YearOptions":  yearOptions(*r.URL, date, minDate, maxDate),
	})
	return s.templates.ExecuteTemplate(w, "positions_maturing.html", ctx)
}

func (s *Server) renderUploadCsvTemplate(w io.Writer) error {
	return s.templates.ExecuteTemplate(w, "upload_csv.html", struct{}{})
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
	if err := s.renderLedgerTemplate(&buf, s.Store(), query, snippet); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleEntriesNew(w http.ResponseWriter, r *http.Request) {
	date, err := dateParam(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid date= parameter: %s", err), http.StatusBadRequest)
		return
	}
	var buf bytes.Buffer
	if err := s.renderAddEntryTemplate(&buf, date, s.Store()); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleAssetsNew(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := s.renderAddAssetTemplate(&buf); err != nil {
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
	if err := s.renderQuotesTemplate(&buf, date.Time); err != nil {
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

func ensureDateParam(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	d := r.URL.Query().Get("date")
	if d == "" {
		now := time.Now()
		q := r.URL.Query()
		q.Set("date", now.Format("2006-01-02"))
		r.URL.RawQuery = q.Encode()
		http.Redirect(w, r, r.URL.String(), http.StatusSeeOther)
		return time.Time{}, false
	}
	date, err := time.Parse("2006-01-02", d)
	if err != nil {
		http.Error(w, "invalid date: "+err.Error(), http.StatusBadRequest)
		return time.Time{}, false
	}
	return date, true
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

func (s *Server) handleCsvUpload(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	err := s.renderUploadCsvTemplate(&buf)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) renderSnipUploadCsvData(w io.Writer, entries []*LedgerEntry, skipped []string, store *Store) error {
	type Row struct {
		*LedgerEntry
		AssetName string
	}
	var rows []*Row
	for _, entry := range entries {
		asset := store.assetMap[entry.AssetID]
		if asset == nil {
			continue
		}
		rows = append(rows, &Row{
			LedgerEntry: entry,
			AssetName:   asset.Name,
		})
	}
	slices.Sort(skipped)
	suffix := ""
	if len(skipped) > 5 {
		skipped = skipped[:5]
		suffix = ", ..."
	}
	return s.templates.ExecuteTemplate(w, "snip_upload_csv_data.html", struct {
		Entries []*Row
		Skipped string
	}{
		Entries: rows,
		Skipped: strings.Join(skipped, ", ") + suffix,
	})
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
	var entries []*LedgerEntry
	var skippedWKNs []string
	store := s.Store()
	for _, file := range r.MultipartForm.File["file"] {
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
		log.Printf("Received file %s with %d items\n", file.Filename, len(items))
		for _, item := range items {
			asset, found := store.FindAssetByWKN(item.WKN)
			if !found || item.Currency != "" && asset.Currency != item.Currency {
				log.Printf("CSV import: skipping item with WKN %q (found:%v)", item.WKN, found)
				skippedWKNs = append(skippedWKNs, item.WKN)
				continue
			}
			entries = append(entries, &LedgerEntry{
				Type:        AssetPrice,
				ValueDate:   item.ValueDate,
				AssetID:     asset.ID(),
				Currency:    asset.Currency,
				PriceMicros: item.PriceMicros,
				Comment:     "CSV import",
			})
		}
	}
	var buf bytes.Buffer
	if err := s.renderSnipUploadCsvData(&buf, entries, skippedWKNs, store); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	status := StatusOK
	if len(skippedWKNs) > 0 {
		if len(entries) > 0 {
			status = StatusPartialSuccess
		} else {
			status = StatusInvalidArgument
		}
	}
	errorText := ""
	if len(skippedWKNs) > 0 {
		errorText = fmt.Sprintf("Successfully read %d rows. Skipped WKNs: %s", len(entries),
			strings.Join(skippedWKNs, ", "))
	}
	s.jsonResponse(w, CsvUploadResponse{
		Status:     status,
		Error:      errorText,
		NumEntries: len(entries),
		InnerHTML:  buf.String(),
	})
}

func (s *Server) handleEntriesPost(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "only application/json is allowed", http.StatusBadRequest)
		return
	}
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

func (s *Server) handleAssetsPost(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		http.Error(w, "only application/json is allowed", http.StatusBadRequest)
		return
	}
	var a Asset
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Store().AddAsset(&a); err != nil {
		s.jsonResponse(w, AddAssetResponse{
			Status: StatusInvalidArgument,
			Error:  err.Error(),
		})
		return
	}
	if err := s.Store().Save(); err != nil {
		http.Error(w, fmt.Sprintf("Error saving ledger: %v", err), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, AddAssetResponse{
		Status:  StatusOK,
		AssetID: a.ID(),
	})
}

func (s *Server) createLedgerEntries(r *AddQuotesRequest) ([]*LedgerEntry, error) {
	result := make([]*LedgerEntry, 0, len(r.Quotes)+len(r.ExchangeRates))
	for _, q := range r.Quotes {
		a, ok := s.Store().assetMap[q.AssetID]
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
			s.reloadTemplates()
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
	mux.Handle("/kontoo/resources/", http.StripPrefix("/kontoo/resources", http.FileServer(http.Dir(s.resourcesDir))))

	mux.HandleFunc("GET /kontoo/ledger", s.reloadHandler(s.handleLedger))
	mux.HandleFunc("GET /kontoo/entries/new", s.reloadHandler(s.handleEntriesNew))
	mux.HandleFunc("GET /kontoo/assets/new", s.reloadHandler(s.handleAssetsNew))
	mux.HandleFunc("GET /kontoo/quotes", s.reloadHandler(s.handleQuotes))
	mux.HandleFunc("GET /kontoo/csv/upload", s.reloadHandler(s.handleCsvUpload))
	mux.HandleFunc("POST /kontoo/quotes", s.handleQuotesPost)
	mux.HandleFunc("POST /kontoo/csv", s.handleCsvPost)
	mux.HandleFunc("POST /kontoo/assets", jsonHandler(s.handleAssetsPost))
	mux.HandleFunc("POST /kontoo/entries", jsonHandler(s.handleEntriesPost))
	mux.HandleFunc("GET /kontoo/positions", s.reloadHandler(s.handlePositions))
	mux.HandleFunc("GET /kontoo/positions/maturing", s.reloadHandler(s.handleMaturingPositions))
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

	fmt.Printf("Server listening on %s\n", s.addr)
	return srv.ListenAndServe()
}
