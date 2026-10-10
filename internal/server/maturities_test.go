package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/units"
)

// maturitiesTestServer has a USD and a CHF bond worth 1000 each, and an
// EUR/USD exchange rate of 1.25. With withCHFRate, also an EUR/CHF rate of 0.8.
func maturitiesTestServer(t *testing.T, withCHFRate bool) *Server {
	t.Helper()
	maturity := civil.DateVal(2026, 6, 30)
	store, err := ledger.NewStore(&ledger.Ledger{
		Header: &ledger.LedgerHeader{BaseCurrency: "EUR"},
		Assets: []*ledger.Asset{
			{Type: ledger.GovernmentBond, Name: "USD bond", ISIN: "US0000000001", Currency: "USD", MaturityDate: &maturity},
			{Type: ledger.GovernmentBond, Name: "CHF bond", ISIN: "CH0000000001", Currency: "CHF", MaturityDate: &maturity},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	entries := []*ledger.LedgerEntry{
		{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.AssetPurchase, AssetID: "US0000000001", QuantityMicros: 1000 * units.One, PriceMicros: units.One},
		{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.AssetPurchase, AssetID: "CH0000000001", QuantityMicros: 1000 * units.One, PriceMicros: units.One},
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

type maturitiesResponse struct {
	Status     StatusCode
	Error      string
	Maturities *struct {
		Currency string
		Values   []struct{ ValueMicros []int64 }
	}
}

func postMaturities(t *testing.T, s *Server) maturitiesResponse {
	t.Helper()
	body, _ := json.Marshal(PositionsMaturitiesRequest{EndTimestamp: civil.DateVal(2024, 6, 30).UnixMilli()})
	w := httptest.NewRecorder()
	s.handlePositionsMaturities(w, httptest.NewRequest(http.MethodPost, "/kontoo/positions/maturities", bytes.NewReader(body)))
	var resp maturitiesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestPositionsMaturitiesInBaseCurrency(t *testing.T) {
	resp := postMaturities(t, maturitiesTestServer(t, true))
	if resp.Status != StatusOK {
		t.Fatalf("status %v: %s", resp.Status, resp.Error)
	}
	var total int64
	for _, v := range resp.Maturities.Values[0].ValueMicros {
		total += v
	}
	// 1000 USD at 1.25 USD per EUR plus 1000 CHF at 0.8 CHF per EUR.
	if resp.Maturities.Currency != "EUR" || total != int64(2050*units.One) {
		t.Errorf("got %d micros %s, want 2050 EUR", total, resp.Maturities.Currency)
	}
}

func TestPositionsMaturitiesMissingExchangeRate(t *testing.T) {
	s := maturitiesTestServer(t, false)
	if resp := postMaturities(t, s); resp.Status == StatusOK || !strings.Contains(resp.Error, "CHF") {
		t.Errorf("want an error naming CHF, got status %v: %q", resp.Status, resp.Error)
	}
	var b bytes.Buffer
	r := httptest.NewRequest(http.MethodGet, "/kontoo/positions/maturing?date=2024-06-30", nil)
	if err := s.renderMaturingPositionsTemplate(&b, r, civil.DateVal(2024, 6, 30)); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	if !strings.Contains(html, "No exchange rate for CHF as of 2024-06-30") ||
		strings.Contains(html, `class="total"`) || strings.Contains(html, `id="maturities-chart"`) {
		t.Error("missing rates should show a warning instead of totals and the chart")
	}
}
