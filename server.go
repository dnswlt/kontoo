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
	SequenceNum int64  `json:"sequenceNum,omitempty"`
	Error       string `json:"error,omitempty"`
}

// END JSON API

type Server struct {
	addr       string
	ledgerPath string
}

func NewServer(addr, ledgerPath string) *Server {
	return &Server{
		addr:       addr,
		ledgerPath: ledgerPath,
	}
}

type LedgerEntryRow struct {
	E *LedgerEntry
	A *Asset
}

func (e *LedgerEntryRow) ValueDate() string {
	return e.E.ValueDate.Format("2006-01-02")
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
func formatMonetaryValue(m Micros) string {
	if m == 0 {
		return ""
	}
	v := float64(m) / 1e6
	return fmt.Sprintf("%.2f", v)
}
func (e *LedgerEntryRow) Value() string {
	return formatMonetaryValue(e.E.ValueMicros)
}
func (e *LedgerEntryRow) Cost() string {
	return formatMonetaryValue(e.E.CostMicros)
}
func (e *LedgerEntryRow) Quantity() string {
	return formatMonetaryValue(e.E.QuantityMicros)
}
func (e *LedgerEntryRow) Price() string {
	v := float64(e.E.PriceMicros) / 1e6
	return fmt.Sprintf("%.3f", v)
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
	Name     string
	Currency Currency
	Value    string
}

func PositionTableRows(s *Store, date time.Time) []*PositionTableRow {
	positions := s.AssetPositionsAt(date)
	slices.SortFunc(positions, func(a, b *AssetPosition) int {
		return strings.Compare(a.Name(), b.Name())
	})
	var res []*PositionTableRow
	for _, p := range positions {
		res = append(res, &PositionTableRow{
			Name:     p.Name(),
			Currency: p.Currency(),
			Value:    formatMonetaryValue(p.CalculatedValueMicros()),
		})
	}
	return res
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
	tmpl := template.New("ledger")
	tmpl.Funcs(template.FuncMap{
		"nonzero": func(v Micros) bool {
			return v != 0
		},
	})
	_, err = tmpl.Parse(string(data))
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

func RenderNewEntryTemplate(w io.Writer, templatePath string, s *Store) error {
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

func RenderPositionsTemplate(w io.Writer, templatePath string, date time.Time, s *Store) error {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}
	tmpl, err := template.New("positions").Parse(string(data))
	if err != nil {
		return err
	}
	positions := PositionTableRows(s, date)
	return tmpl.Execute(w, struct {
		Date        string
		CurrentDate string
		TableRows   []*PositionTableRow
	}{
		Date:        date.Format("2006-01-02"),
		CurrentDate: time.Now().Format("2006-01-02 15:04:05"),
		TableRows:   positions,
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

func (s *Server) handleNewEntriesNew(w http.ResponseWriter, r *http.Request) {
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	err = RenderNewEntryTemplate(&buf, "./templates/add_entry.html", store)
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
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	err = RenderPositionsTemplate(&buf, "./templates/positions.html", date, store)
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

func (s *Server) createMux() *http.ServeMux {
	mux := &http.ServeMux{}
	mux.HandleFunc("/kontoo/ledger", s.handleLedger)
	mux.HandleFunc("/kontoo/entries/new", s.handleNewEntriesNew)
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
