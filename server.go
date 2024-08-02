package kontoo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
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

// END JSON API

type Server struct {
	addr          string
	ledgerPath    string
	resourcesPath string
}

func NewServer(addr, ledgerPath, resourcesPath string) *Server {
	return &Server{
		addr:          addr,
		ledgerPath:    ledgerPath,
		resourcesPath: resourcesPath,
	}
}

type LedgerEntryRow struct {
	E *LedgerEntry
	A *Asset
}

func (d *Date) String() string {
	if d == nil {
		return ""
	}
	return d.Format("2006-01-02")
}

func (e *LedgerEntryRow) ValueDate() string {
	return e.E.ValueDate.String()
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
	slices.SortFunc(positions, func(a, b *AssetPosition) int {
		return strings.Compare(a.Name(), b.Name())
	})
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
			row.PurchasePrice = 0 // TODO: calculate
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

func RenderLedgerTemplate(w io.Writer, templatePath string, s *Store) error {
	rows := LedgerEntryRows(s)
	// Sort ledger rows by (ValueDate, Created) for output table.
	slices.SortFunc(rows, func(a, b *LedgerEntryRow) int {
		c := a.E.ValueDate.Time.Compare(b.E.ValueDate.Time)
		if c != 0 {
			return c
		}
		return a.E.Created.Compare(b.E.Created)
	})
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}
	tmpl, err := template.New("ledger").Funcs(commonFuncs()).Parse(string(data))
	if err != nil {
		return err
	}
	return tmpl.Execute(w, struct {
		TableRows   []*LedgerEntryRow
		CurrentDate string
	}{
		TableRows:   rows,
		CurrentDate: time.Now().Format("2006-01-02 15:04:05"),
	})
}

func RenderAddEntryTemplate(w io.Writer, templatePath string, s *Store) error {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}
	tmpl, err := template.New("entries").Parse(string(data))
	if err != nil {
		return err
	}
	assets := make([]*Asset, len(s.L.Assets))
	copy(assets, s.L.Assets)
	slices.SortFunc(assets, func(a, b *Asset) int {
		return strings.Compare(a.Name, b.Name)
	})
	quoteCurrenciesMap := make(map[Currency]bool)
	var quoteCurrencies []Currency
	for _, a := range assets {
		if a.Currency == s.L.Header.BaseCurrency {
			continue
		}
		if _, ok := quoteCurrenciesMap[a.Currency]; !ok {
			quoteCurrencies = append(quoteCurrencies, a.Currency)
			quoteCurrenciesMap[a.Currency] = true
		}
	}
	slices.Sort(quoteCurrencies)
	return tmpl.Execute(w, struct {
		CurrentDate     string
		Assets          []*Asset
		BaseCurrency    Currency
		QuoteCurrencies []Currency
	}{
		CurrentDate:     time.Now().Format("2006-01-02"),
		Assets:          assets,
		BaseCurrency:    s.L.Header.BaseCurrency,
		QuoteCurrencies: quoteCurrencies,
	})
}

func RenderAddAssetTemplate(w io.Writer, templatePath string, s *Store) error {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}
	tmpl, err := template.New("assets").Parse(string(data))
	if err != nil {
		return err
	}
	assetTypeVals := AssetTypeValues()
	assetTypes := make([]string, 0, len(assetTypeVals))
	for _, a := range assetTypeVals {
		if a == UnspecifiedAssetType {
			continue
		}
		assetTypes = append(assetTypes, a.String())
	}
	return tmpl.Execute(w, struct {
		CurrentDate string
		AssetTypes  []string
	}{
		CurrentDate: time.Now().Format("2006-01-02"),
		AssetTypes:  assetTypes,
	})
}

func RenderPositionsTemplate(w io.Writer, templatePath string, style PositionDisplayStyle, date time.Time, s *Store) error {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}
	tmpl, err := template.New("positions").Funcs(commonFuncs()).Parse(string(data))
	if err != nil {
		return err
	}
	positions := PositionTableRows(s, date, style)
	return tmpl.Execute(w, struct {
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

func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request) {
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	err = RenderLedgerTemplate(&buf, "./templates/ledger.html", store)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleEntriesNew(w http.ResponseWriter, r *http.Request) {
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	err = RenderAddEntryTemplate(&buf, "./templates/add_entry.html", store)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
}

func (s *Server) handleAssetsNew(w http.ResponseWriter, r *http.Request) {
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	err = RenderAddAssetTemplate(&buf, "./templates/add_asset.html", store)
	if err != nil {
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
	if r.Form.Get("f") == "maturity" {
		style = DisplayStyleMaturingSecurities
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	err = RenderPositionsTemplate(&buf, "./templates/positions.html", style, date, store)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(buf.Bytes())
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
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	err = store.Add(e)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AddLedgerEntryResponse{
			Status: "INVALID_ARGUMENT",
			Error:  err.Error(),
		})
		return
	}
	err = store.Save()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error saving ledger: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AddLedgerEntryResponse{
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
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	err = store.AddAsset(a)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AddAssetResponse{
			Status: "INVALID_ARGUMENT",
			Error:  err.Error(),
		})
		return
	}
	err = store.Save()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error saving ledger: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AddAssetResponse{
		Status:  "OK",
		AssetID: a.ID(),
	})
}

func (s *Server) createMux() *http.ServeMux {
	mux := &http.ServeMux{}
	// Serve static resources like CSS from resources/ dir.
	mux.Handle("/kontoo/resources/", http.StripPrefix("/kontoo/resources", http.FileServer(http.Dir(s.resourcesPath))))

	mux.HandleFunc("/kontoo/ledger", s.handleLedger)
	mux.HandleFunc("/kontoo/entries/new", s.handleEntriesNew)
	mux.HandleFunc("GET /kontoo/assets/new", s.handleAssetsNew)
	mux.HandleFunc("POST /kontoo/assets", s.handleAssetsPost)
	mux.HandleFunc("POST /kontoo/entries", s.handleEntriesPost)
	mux.HandleFunc("GET /kontoo/positions", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		http.Redirect(w, r, fmt.Sprintf("/kontoo/positions/%d/%d/%d", now.Year(), now.Month(), now.Day()), http.StatusSeeOther)
	})
	mux.HandleFunc("GET /kontoo/positions/{year}/{month}/{day}", s.handlePositions)
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
