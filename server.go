package kontoo

import (
	"bytes"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// JSON API for server requests and responses.
type AddLedgerEntryResponse struct {
	Status      string `json:"status"`
	SequenceNum int64  `json:"sequenceNum"`
	Error       string `json:"error,omitempty"`
}

type AddAssetResponse struct {
	Status  string `json:"status"`
	AssetID string `json:"assetId,omitempty"`
	Error   string `json:"error,omitempty"`
}

type CsvUploadResponse struct {
	Status    string         `json:"status"`
	Error     string         `json:"error,omitempty"`
	Entries   []*LedgerEntry `json:"entries,omitempty"`
	InnerHTML string         `json:"innerHTML"`
}

// END JSON API

type PendingLedgerEntries struct {
	Entries []*LedgerEntry
	Created time.Time
}

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
	// In-memory cache of recent CSV uploads waiting for confirmation
	csvCacheCh        chan struct{}
	pendingEntries    map[string]PendingLedgerEntries
	mutPendingEntries sync.Mutex
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
	yf, err := NewYFinanceCached(CacheDir())
	if err != nil {
		log.Printf("Error creating YFinance. Stock quotes will not be available. Error: %v", err)
		yf = nil // Should be nil anyway, but better be safe
	}
	ch := make(chan struct{})
	s := &Server{
		addr:           addr,
		ledgerPath:     ledgerPath,
		resourcesDir:   resourcesDir,
		templatesDir:   templatesDir,
		store:          store,
		yFinance:       yf,
		csvCacheCh:     ch,
		pendingEntries: make(map[string]PendingLedgerEntries),
	}
	go func() {
		tick := time.NewTicker(1 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ch:
				return
			case now := <-tick.C:
				func() {
					s.mutPendingEntries.Lock()
					defer s.mutPendingEntries.Unlock()
					minCreated := now.Add(-10 * time.Minute)
					for k, e := range s.pendingEntries {
						if e.Created.Before(minCreated) {
							log.Printf("Deleteing old cache entry %s", k)
							delete(s.pendingEntries, k)
						}
					}
				}()
			}
		}
	}()
	if err := s.reloadTemplates(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) Shutdown() {
	// Trigger shutdown of the goroutine that clears the CSV cache.
	close(s.csvCacheCh)
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

func LedgerEntryRows(s *Store) []*LedgerEntryRow {
	var res []*LedgerEntryRow
	for _, e := range s.L.Entries {
		res = append(res, &LedgerEntryRow{
			E: e,
			A: s.assetMap[e.AssetID],
		})
	}
	return res
}

type PositionTableRow struct {
	ID              string
	Name            string
	Type            AssetType
	Currency        Currency
	Value           Micros
	PurchasePrice   Micros
	NominalValue    Micros
	InterestRate    Micros
	IssueDate       *Date
	MaturityDate    *Date
	YearsToMaturity float64
}

type PositionDisplayStyle int

const (
	DisplayStyleAll PositionDisplayStyle = iota
	DisplayStyleMaturingSecurities
)

func PositionTableRows(s *Store, date time.Time, style PositionDisplayStyle) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	sortFunc := func(a, b *AssetPosition) int {
		return strings.Compare(a.Name(), b.Name())
	}
	if style == DisplayStyleMaturingSecurities {
		sortFunc = func(a, b *AssetPosition) int {
			c := CompareDatePtr(a.Asset.MaturityDate, b.Asset.MaturityDate)
			if c != 0 {
				return c
			}
			return strings.Compare(a.Name(), b.Name())
		}
	}
	slices.SortFunc(positions, sortFunc)
	var res []*PositionTableRow
	for _, p := range positions {
		a := p.Asset
		if style == DisplayStyleMaturingSecurities && a.MaturityDate == nil {
			continue
		}
		row := &PositionTableRow{
			ID:       a.ID(),
			Name:     a.Name,
			Type:     a.Type,
			Currency: a.Currency,
			Value:    p.CalculatedValueMicros(),
		}
		if style == DisplayStyleMaturingSecurities {
			row.PurchasePrice = p.PurchasePrice()
			row.NominalValue = p.QuantityMicros
			row.InterestRate = a.InterestMicros
			row.IssueDate = a.IssueDate
			row.MaturityDate = a.MaturityDate
			row.YearsToMaturity = a.MaturityDate.Sub(date).Hours() / 24 / 365
		}
		res = append(res, row)
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
	}
}

func (s *Server) renderLedgerTemplate(w io.Writer, store *Store) error {
	rows := LedgerEntryRows(store)
	// Sort ledger rows by (ValueDate, Created) for output table.
	slices.SortFunc(rows, func(a, b *LedgerEntryRow) int {
		c := a.E.ValueDate.Time.Compare(b.E.ValueDate.Time)
		if c != 0 {
			return c
		}
		return a.E.Created.Compare(b.E.Created)
	})
	return s.templates.ExecuteTemplate(w, "ledger.html", struct {
		TableRows   []*LedgerEntryRow
		CurrentDate string
	}{
		TableRows:   rows,
		CurrentDate: time.Now().Format("2006-01-02 15:04:05"),
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
		if a.Currency == store.L.Header.BaseCurrency {
			continue
		}
		if _, ok := quoteCurrenciesMap[a.Currency]; !ok {
			quoteCurrencies = append(quoteCurrencies, a.Currency)
			quoteCurrenciesMap[a.Currency] = true
		}
	}
	slices.Sort(quoteCurrencies)
	return s.templates.ExecuteTemplate(w, "add_entry.html", struct {
		CurrentDate     string
		Assets          []*Asset
		BaseCurrency    Currency
		QuoteCurrencies []Currency
	}{
		CurrentDate:     time.Now().Format("2006-01-02"),
		Assets:          assets,
		BaseCurrency:    store.L.Header.BaseCurrency,
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
		CurrentDate string
		AssetTypes  []string
	}{
		CurrentDate: time.Now().Format("2006-01-02"),
		AssetTypes:  assetTypes,
	})
}

func (s *Server) renderQuotesTemplate(w io.Writer, date time.Time) error {
	type QuoteEntry struct {
		AssetID      string
		AssetName    string
		Symbol       string
		Currency     Currency
		ClosingPrice Micros
		Date         string
	}
	var entries []*QuoteEntry
	if s.yFinance != nil {
		assets := s.Store().FindAssetsForQuoteService("YF")
		symbols := make([]string, len(assets))
		for i, a := range assets {
			symbols[i] = a.QuoteServiceSymbols["YF"]
		}
		hist, err := s.yFinance.GetDailyQuotes(symbols, date)
		if err != nil {
			log.Printf("Failed to get price history: %v", err)
		}
		for i, h := range hist {
			a := assets[i]
			entries = append(entries, &QuoteEntry{
				AssetID:      a.ID(),
				AssetName:    a.Name,
				Symbol:       h.Symbol,
				Currency:     h.Currency,
				ClosingPrice: h.ClosingPrice,
				Date:         h.Date.Format("2006-01-02"),
			})
		}
	}
	return s.templates.ExecuteTemplate(w, "quotes.html", struct {
		Date    string
		Entries []*QuoteEntry
	}{
		Date:    date.Format("2006-01-02"),
		Entries: entries,
	})
}

func (s *Server) renderPositionsTemplate(w io.Writer, style PositionDisplayStyle, date time.Time, store *Store) error {
	positions := PositionTableRows(store, date, style)
	return s.templates.ExecuteTemplate(w, "positions.html", struct {
		Date                       string
		CurrentDate                string
		ShowMaturingSecurityFields bool
		TableRows                  []*PositionTableRow
	}{
		Date:                       date.Format("2006-01-02"),
		CurrentDate:                time.Now().Format("2006-01-02 15:04:05"),
		ShowMaturingSecurityFields: style == DisplayStyleMaturingSecurities,
		TableRows:                  positions,
	})
}

func (s *Server) renderUploadCsvTemplate(w io.Writer) error {
	return s.templates.ExecuteTemplate(w, "upload_csv.html", struct{}{})
}

func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := s.renderLedgerTemplate(&buf, s.Store()); err != nil {
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
	d := r.Form.Get("d")
	var date time.Time
	if d == "" {
		date = time.Now()
	} else {
		var err error
		date, err = time.Parse("2006-01-02", r.Form.Get("d"))
		if err != nil {
			http.Error(w, "invalid value for d= parameter", http.StatusBadRequest)
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

func (s *Server) handlePositions(w http.ResponseWriter, r *http.Request) {
	var year, month, day int
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil || year < 1970 || year > 9999 {
		http.Error(w, "", http.StatusNotFound)
		return
	}
	month, err = strconv.Atoi(r.PathValue("month"))
	if err != nil || month < 1 || month > 12 {
		http.Error(w, "", http.StatusNotFound)
		return
	}
	day, err = strconv.Atoi(r.PathValue("day"))
	if err != nil || day < 1 || day > 31 {
		http.Error(w, "", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}
	style := DisplayStyleAll
	if r.Form.Get("f") == "maturing" {
		style = DisplayStyleMaturingSecurities
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err = s.renderPositionsTemplate(&buf, style, date, s.Store())
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

func generateUID() string {
	p := make([]byte, 16)
	crand.Read(p)
	return hex.EncodeToString(p)
}

func (s *Server) renderSnippetUploadResults(w io.Writer, entries []*LedgerEntry, skipped []string, confirmationID string, store *Store) error {
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
	return s.templates.ExecuteTemplate(w, "snip_upload_results.html", struct {
		Entries        []*Row
		Skipped        string
		ConfirmationID string
	}{
		Entries:        rows,
		Skipped:        strings.Join(skipped, ", ") + suffix,
		ConfirmationID: confirmationID,
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
	confirmationID := generateUID()
	var buf bytes.Buffer
	if err := s.renderSnippetUploadResults(&buf, entries, skippedWKNs, confirmationID, store); err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	if len(entries) > 0 {
		func() {
			s.mutPendingEntries.Lock()
			defer s.mutPendingEntries.Unlock()
			s.pendingEntries[confirmationID] = PendingLedgerEntries{
				Entries: entries,
				Created: time.Now(),
			}
		}()
	}
	status := "OK"
	if len(skippedWKNs) > 0 {
		if len(entries) > 0 {
			status = "Partial Success"
		} else {
			status = "Error"
		}
	}
	error := ""
	if len(skippedWKNs) > 0 {
		error = fmt.Sprintf("Skipped WKNs: %s", strings.Join(skippedWKNs, "\n"))
	}
	s.jsonResponse(w, CsvUploadResponse{
		Status:    status,
		Error:     error,
		InnerHTML: buf.String(),
	})
}

func (s *Server) handleCsvConfirmPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	confirmationID := r.Form.Get("ConfirmationID")
	if confirmationID == "" {
		http.Error(w, "empty confirmation id", http.StatusBadRequest)
		return
	}
	log.Printf("Received confirmation for %s", confirmationID)
	p, found := func() (PendingLedgerEntries, bool) {
		s.mutPendingEntries.Lock()
		defer s.mutPendingEntries.Unlock()
		p, found := s.pendingEntries[confirmationID]
		delete(s.pendingEntries, confirmationID)
		return p, found
	}()
	if !found {
		http.Error(w, "no such id", http.StatusNotFound)
		return
	}
	added := 0
	for _, e := range p.Entries {
		err := s.Store().Add(e)
		if err != nil {
			log.Printf("Could not add ledger entry %v: %v", e, err)
			break
		}
		added++
	}
	if added != len(p.Entries) {
		http.Error(w, "could not add all entries", http.StatusConflict)
		return
	}
	if err := s.Store().Save(); err != nil {
		http.Error(w, fmt.Sprintf("Error saving ledger: %v", err), http.StatusInternalServerError)
		return
	}
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
			Status: "INVALID_ARGUMENT",
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
		Status:      "OK",
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
			Status: "INVALID_ARGUMENT",
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
		Status:  "OK",
		AssetID: a.ID(),
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
	mux.HandleFunc("POST /kontoo/csv", s.handleCsvPost)
	mux.HandleFunc("POST /kontoo/csv/confirm", s.handleCsvConfirmPost)
	mux.HandleFunc("POST /kontoo/assets", s.handleAssetsPost)
	mux.HandleFunc("POST /kontoo/entries", s.handleEntriesPost)
	mux.HandleFunc("GET /kontoo/positions", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		http.Redirect(w, r, fmt.Sprintf("/kontoo/positions/%d/%d/%d", now.Year(), now.Month(), now.Day()), http.StatusSeeOther)
	})
	mux.HandleFunc("GET /kontoo/positions/{year}/{month}/{day}", s.reloadHandler(s.handlePositions))
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
	defer s.Shutdown()
	return srv.ListenAndServe()
}
