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
		if !p.LastPriceUpdate.IsZero() {
			notes = append(notes, fmt.Sprintf("Price date: %s", p.LastPriceUpdate))
			lastUpdate = p.LastPriceUpdate
		}
		if !p.LastValueUpdate.IsZero() {
			notes = append(notes, fmt.Sprintf("Value date: %s", p.LastValueUpdate))
			if lastUpdate.IsZero() || p.LastValueUpdate.Before(lastUpdate.Time) {
				lastUpdate = p.LastValueUpdate
			}
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

func commonFuncs() template.FuncMap {
	return template.FuncMap{
		"nonzero": func(m Micros) bool {
			return m != 0
		},
		"money": func(m Micros) string {
			return m.Format("()'.2")
		},
		"price": func(m Micros) string {
			return m.Format("'.3")
		},
		"quantity": func(m Micros) string {
			return m.Format("'.0")
		},
		"percent": func(m Micros) string {
			return m.Format(".2%")
		},
		"yyyymmdd": func(t any) (string, error) {
			switch d := t.(type) {
			case time.Time:
				return d.Format("2006-01-02"), nil
			case Date:
				return d.Time.Format("2006-01-02"), nil
			}
			return "", fmt.Errorf("yyyymmdd called with invalid type %t", t)
		},
		"assetType": func(t AssetType) (string, error) {
			if a, ok := assetTypeInfos[t]; ok {
				return a.displayName, nil
			}
			return "", fmt.Errorf("no display name for asset type %v", t)
		},
		"assetCategory": func(t AssetType) (string, error) {
			if a, ok := assetTypeInfos[t]; ok {
				return a.category, nil
			}
			return "", fmt.Errorf("no category for asset type %v", t)
		},
		"days": func(d time.Duration) int {
			return int(math.Round(d.Seconds() / 60 / 60 / 24))
		},
	}
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

func (s *Server) renderAddEntryTemplate(w io.Writer, store *Store) error {
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
	return s.templates.ExecuteTemplate(w, "add_entry.html", struct {
		Today           string
		Assets          []*Asset
		BaseCurrency    Currency
		QuoteCurrencies []Currency
	}{
		Today:           time.Now().Format("2006-01-02"),
		Assets:          assets,
		BaseCurrency:    store.BaseCurrency(),
		QuoteCurrencies: quoteCurrencies,
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

type DropdownOptions struct {
	Selected *NamedOption
	Options  []NamedOption
}
type NamedOption struct {
	Name  string
	Value any
	Data  map[string]any
}

func yearOptions(url url.URL, date time.Time, minDate, maxDate Date) DropdownOptions {
	res := DropdownOptions{
		Selected: &NamedOption{
			Name:  fmt.Sprintf("%d", date.Year()),
			Value: date.Year(),
		},
	}
	for y := maxDate.Year(); y >= minDate.Year(); y-- {
		d := DateVal(y, date.Month(), date.Day())
		q := url.Query()
		q.Set("date", d.Format("2006-01-02"))
		url.RawQuery = q.Encode()
		res.Options = append(res.Options, NamedOption{
			Name:  fmt.Sprintf("%d", y),
			Value: y,
			Data: map[string]any{
				"URL": url.String(),
			},
		})
	}
	return res
}

func monthOptions(url url.URL, date time.Time, maxDate Date) DropdownOptions {
	months := []string{
		"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
	}
	res := DropdownOptions{
		Selected: &NamedOption{
			Name:  months[date.Month()-1],
			Value: int(date.Month()),
		},
	}
	if date.Year() > maxDate.Year() {
		return res // No options if no data
	}
	maxMonth := 12
	if maxDate.Year() == date.Year() {
		maxMonth = int(maxDate.Month())
	}
	for i := 0; i < maxMonth; i++ {
		d := DateVal(date.Year(), time.Month(i+1), 1).AddDate(0, 1, -1)
		q := url.Query()
		q.Set("date", d.Format("2006-01-02"))
		url.RawQuery = q.Encode()
		res.Options = append(res.Options, NamedOption{
			Name:  months[i],
			Value: i + 1,
			Data: map[string]any{
				"URL": url.String(),
			},
		})
	}
	return res
}

func (s *Server) renderPositionsTemplate(w io.Writer, r *http.Request, date time.Time) error {
	positions := positionTableRows(s.Store(), date)
	groups := positionTableRowGroups(positions, func(r *PositionTableRow) string {
		return assetTypeInfos[r.Type].category
	})
	minDate, maxDate := s.Store().ValueDateRange()
	return s.templates.ExecuteTemplate(w, "positions.html", map[string]any{
		"Date":         date.Format("2006-01-02"),
		"Today":        time.Now().Format("2006-01-02"),
		"Now":          time.Now().Format("2006-01-02 15:04:05"),
		"BaseCurrency": s.Store().BaseCurrency(),
		"Groups":       groups,
		"ActiveChips": map[string]bool{
			"all":   true,
			"today": toDate(date).Equal(today()),
		},
		"MonthOptions": monthOptions(*r.URL, date, maxDate),
		"YearOptions":  yearOptions(*r.URL, date, minDate, maxDate),
	})
}

func (s *Server) renderMaturingPositionsTemplate(w io.Writer, date time.Time) error {
	positions := maturingPositionTableRows(s.Store(), date)
	return s.templates.ExecuteTemplate(w, "positions_maturing.html", map[string]any{
		"Date":      date.Format("2006-01-02"),
		"Today":     time.Now().Format("2006-01-02"),
		"Now":       time.Now().Format("2006-01-02 15:04:05"),
		"TableRows": positions,
		"ActiveChips": map[string]bool{
			"maturing": true,
		},
	})
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
	var buf bytes.Buffer
	if err := s.renderAddEntryTemplate(&buf, s.Store()); err != nil {
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
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	d := r.Form.Get("date")
	var date time.Time
	if d == "" {
		date = time.Now()
	} else {
		var err error
		date, err = time.Parse("2006-01-02", d)
		if err != nil {
			http.Error(w, "invalid value for date= parameter", http.StatusBadRequest)
			return
		}
	}
	var buf bytes.Buffer
	if err := s.renderQuotesTemplate(&buf, date); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
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
	err := s.renderMaturingPositionsTemplate(&buf, date)
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
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	// Parse as a ledger entry in the same way that we'd parse it from the command line.
	var args []string
	for k, v := range r.Form {
		if strings.HasPrefix(k, "Submit") {
			continue // Ignore Submit button values.
		}
		if len(v) > 0 && len(v[0]) > 0 {
			args = append(args, "-"+k)
			args = append(args, v...)
		}
	}
	e, err := ParseLedgerEntry(args)
	if err != nil {
		http.Error(w, fmt.Sprintf("Cannot parse ledger: %v", err), http.StatusBadRequest)
		return
	}
	err = s.Store().Add(e)
	if err != nil {
		s.jsonResponse(w, AddLedgerEntryResponse{
			Status: StatusInvalidArgument,
			Error:  err.Error(),
		})
		return
	}
	err = s.Store().Save()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error saving ledger: %v", err), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, AddLedgerEntryResponse{
		Status:      StatusOK,
		SequenceNum: e.SequenceNum,
	})
}

func (s *Server) handleAssetsPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	// Parse as a ledger entry in the same way that we'd parse it from the command line.
	var args []string
	for k, v := range r.Form {
		if strings.HasPrefix(k, "Submit") {
			continue // Ignore Submit button values.
		}
		if len(v) > 0 && len(v[0]) > 0 {
			args = append(args, "-"+k)
			args = append(args, v...)
		}
	}
	a, err := ParseAsset(args)
	if err != nil {
		http.Error(w, fmt.Sprintf("Cannot parse asset: %v", err), http.StatusBadRequest)
		return
	}
	err = s.Store().AddAsset(a)
	if err != nil {
		s.jsonResponse(w, AddAssetResponse{
			Status: StatusInvalidArgument,
			Error:  err.Error(),
		})
		return
	}
	err = s.Store().Save()
	if err != nil {
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
	defer r.Body.Close()
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
	mux.HandleFunc("POST /kontoo/assets", s.handleAssetsPost)
	mux.HandleFunc("POST /kontoo/entries", s.handleEntriesPost)
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
