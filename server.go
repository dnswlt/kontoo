package kontoo

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

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
	return time.Time(e.E.ValueDate).Format("2006-01-02")
}
func (e *LedgerEntryRow) EntryType() string {
	return e.E.Type.String()
}
func (e *LedgerEntryRow) AssetID() string {
	return e.A.ID()
}
func (e *LedgerEntryRow) AssetName() string {
	return e.A.Name
}
func (e *LedgerEntryRow) AssetType() string {
	return e.A.Type.String()
}
func (e *LedgerEntryRow) Currency() string {
	return string(e.E.Currency)
}
func formatMonetaryValue(m Micros) string {
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

func RenderLedgerTemplate(w io.Writer, templatePath string, s *Store) error {
	rows := LedgerEntryRows(s)
	// Sort ledger rows by (ValueDate, Created) for output table.
	slices.SortFunc(rows, func(a, b *LedgerEntryRow) int {
		c := time.Time(a.E.ValueDate).Compare(time.Time(b.E.ValueDate))
		if c != 0 {
			return c
		}
		return time.Time(a.E.Created).Compare(time.Time(b.E.Created))
	})
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}
	tmpl, err := template.New("ledger").Parse(string(data))
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
	return tmpl.Execute(w, struct {
		CurrentDate string
		Assets      []*Asset
	}{
		CurrentDate: time.Now().Format("2006-01-02"),
		Assets:      assets,
	})
}

func (s *Server) handleLedger(w http.ResponseWriter, r *http.Request) {
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	err = RenderLedgerTemplate(w, "./templates/ledger.html", store)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleNewEntryForm(w http.ResponseWriter, r *http.Request) {
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error loading ledger: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	var buf bytes.Buffer
	err = RenderNewEntryTemplate(&buf, "./templates/add_entry.html", store)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render template: %s", err), http.StatusInternalServerError)
		return
	}
	w.Write(buf.Bytes())
}

func (s *Server) handleEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Add("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
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
	store.Add(e)
	store.Save()
	if _, ok := r.Form["SubmitNext"]; ok {
		// Create new one immediately.
		http.Redirect(w, r, "/kontoo/entries/new", http.StatusSeeOther)
		return
	}
	// Show ledger.
	http.Redirect(w, r, "/kontoo/ledger", http.StatusSeeOther)
}

func (s *Server) createMux() *http.ServeMux {
	mux := &http.ServeMux{}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/kontoo/ledger", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/kontoo/ledger", s.handleLedger)
	mux.HandleFunc("/kontoo/entries/new", s.handleNewEntryForm)
	mux.HandleFunc("/kontoo/entries", s.handleEntries)
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
