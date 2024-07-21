package kontoo

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"slices"
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
	type tmplCtx struct {
		TableRows   []*LedgerEntryRow
		CurrentDate string
	}
	return tmpl.Execute(w, tmplCtx{
		TableRows:   rows,
		CurrentDate: time.Now().Format("2006-01-02 15:04:05"),
	})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	store, err := LoadStore(s.ledgerPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid ledger path: %s", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	err = RenderLedgerTemplate(w, "./templates/ledger.html", store)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid ledger path: %s", err), http.StatusInternalServerError)
		return
	}
}

func (s *Server) createMux() *http.ServeMux {
	mux := &http.ServeMux{}
	mux.HandleFunc("/", s.handleIndex)
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
