package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/units"
	"golang.org/x/net/html"
)

// maturitiesTestServer has a USD and a CHF bond with a nominal value of 1000
// each, bought at 90%, an EUR fixed deposit of 500 whose balance grew to 520
// through interest, and an EUR/USD exchange rate of 1.25. With withCHFRate,
// also an EUR/CHF rate of 0.8.
func maturitiesTestServer(t *testing.T, withCHFRate bool) *Server {
	t.Helper()
	maturity := civil.DateVal(2026, 6, 30)
	depositMaturity := civil.DateVal(2025, 3, 31)
	store, err := ledger.NewStore(&ledger.Ledger{
		Header: &ledger.LedgerHeader{BaseCurrency: "EUR"},
		Assets: []*ledger.Asset{
			{Type: ledger.GovernmentBond, Name: "USD bond", ISIN: "US0000000001", Currency: "USD", MaturityDate: &maturity},
			{Type: ledger.GovernmentBond, Name: "CHF bond", ISIN: "CH0000000001", Currency: "CHF", MaturityDate: &maturity},
			{Type: ledger.FixedDepositAccount, Name: "Deposit", CustomID: "deposit", Currency: "EUR", MaturityDate: &depositMaturity},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	entries := []*ledger.LedgerEntry{
		{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.AssetPurchase, AssetID: "US0000000001", QuantityMicros: 1000 * units.One, PriceMicros: 900_000},
		{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.AssetPurchase, AssetID: "CH0000000001", QuantityMicros: 1000 * units.One, PriceMicros: 900_000},
		{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.AccountCredit, AssetID: "deposit", ValueMicros: 500 * units.One},
		{ValueDate: civil.DateVal(2024, 6, 1), Type: ledger.AccountBalance, AssetID: "deposit", ValueMicros: 520 * units.One},
		{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.ExchangeRate, Currency: "EUR", QuoteCurrency: "USD", PriceMicros: 1_250_000},
	}
	if withCHFRate {
		entries = append(entries, &ledger.LedgerEntry{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.ExchangeRate, Currency: "EUR", QuoteCurrency: "CHF", PriceMicros: 800_000})
	}
	for _, e := range entries {
		if err := store.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{store: store, baseDir: "../../resources"}
	if err := s.reloadTemplates(); err != nil {
		t.Fatal(err)
	}
	return s
}

// renderMaturing renders the maturing positions page on 2024-06-30.
func renderMaturing(t *testing.T, s *Server) *html.Node {
	t.Helper()
	var b bytes.Buffer
	r := httptest.NewRequest(http.MethodGet, "/kontoo/positions/maturing?date=2024-06-30", nil)
	if err := s.renderMaturingPositionsTemplate(&b, r, civil.DateVal(2024, 6, 30)); err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(&b)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestPositionsMaturitiesInBaseCurrency(t *testing.T) {
	script := findElement(renderMaturing(t, maturitiesTestServer(t, true)), "script", "id", "maturities-chart-data")
	if script == nil {
		t.Fatal("missing maturities chart data")
	}
	var chart MaturitiesChart
	if err := json.Unmarshal([]byte(textContent(script)), &chart); err != nil {
		t.Fatal(err)
	}
	// Nominal values, not market values: 1000 USD at 1.25 USD per EUR and
	// 1000 CHF at 0.8 CHF per EUR, maturing exactly two years after the report
	// date (bucket "2", not "3"); and the deposit's principal of 500 EUR (not its
	// balance of 520 EUR including interest), maturing after nine months.
	wantLabels := []string{"0.5", "1", "2", "3", "4", "5+"}
	wantValues := []int64{0, int64(500 * units.One), int64(2050 * units.One), 0, 0, 0}
	if chart.Currency != "EUR" || !slices.Equal(chart.BucketLabels, wantLabels) || !slices.Equal(chart.ValueMicros, wantValues) {
		t.Errorf("got %s %v %v, want EUR %v %v", chart.Currency, chart.BucketLabels, chart.ValueMicros, wantLabels, wantValues)
	}
}

func TestPositionsMaturitiesMissingExchangeRate(t *testing.T) {
	doc := renderMaturing(t, maturitiesTestServer(t, false))
	if callout := findElement(doc, "div", "class", "callout callout-warn"); callout == nil ||
		!strings.Contains(textContent(callout), "No exchange rate for CHF as of 2024-06-30") {
		t.Error("missing exchange-rate warning")
	}
	if findElement(doc, "tr", "class", "total") != nil || findElement(doc, "script", "id", "maturities-chart-data") != nil {
		t.Error("missing rates should show a warning instead of totals and the charts")
	}
}

func TestMaturingTableNominalValueAndIssueDate(t *testing.T) {
	s := maturitiesTestServer(t, true)
	var b bytes.Buffer
	r := httptest.NewRequest(http.MethodGet, "/kontoo/positions/maturing?date=2024-06-30", nil)
	if err := s.renderMaturingPositionsTemplate(&b, r, civil.DateVal(2024, 6, 30)); err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(&b)
	if err != nil {
		t.Fatal(err)
	}
	// Rows by asset ID (the name cell also contains the row's action menu).
	rows := map[string]map[string]string{}
	for _, tr := range findElements(doc, "tr") {
		if cells := rowCells(tr); cells["Code"] != "" {
			rows[cells["Code"]] = cells
		}
	}
	// Fixed deposits have no nominal value (only the charts use their principal).
	if got := rows["deposit"]["Nom value"]; got != "" {
		t.Errorf("deposit nominal value: got %q, want empty", got)
	}
	// The test bonds have no issue date.
	if got := rows["US0000000001"]["Issue date"]; got != "" {
		t.Errorf("missing issue date: got %q, want empty", got)
	}
}
