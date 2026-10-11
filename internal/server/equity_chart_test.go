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
)

func TestChartsEquitySortedByBaseCurrencyValue(t *testing.T) {
	store, err := ledger.NewStore(&ledger.Ledger{
		Header: &ledger.LedgerHeader{BaseCurrency: "EUR"},
		Assets: []*ledger.Asset{
			{Type: ledger.Stock, Name: "EUR stock", TickerSymbol: "EURS", Currency: "EUR"},
			{Type: ledger.Stock, Name: "CHF stock", TickerSymbol: "CHFS", Currency: "CHF"},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []*ledger.LedgerEntry{
		{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.AssetPurchase, AssetID: "EURS", QuantityMicros: 1000 * units.One, PriceMicros: units.One},
		// Larger in CHF, but only 550 EUR at 2 CHF per EUR.
		{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.AssetPurchase, AssetID: "CHFS", QuantityMicros: 1100 * units.One, PriceMicros: units.One},
		{ValueDate: civil.DateVal(2024, 1, 1), Type: ledger.ExchangeRate, Currency: "EUR", QuoteCurrency: "CHF", PriceMicros: 2 * units.One},
	} {
		if err := store.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{store: store}
	chart := s.equityChart(civil.DateVal(2024, 6, 30))
	wantNames := []string{"EUR stock", "CHF stock"}
	wantValues := []int64{int64(1000 * units.One), int64(550 * units.One)}
	if !slices.Equal(chart.AssetNames, wantNames) || !slices.Equal(chart.ValueMicros, wantValues) {
		t.Errorf("got %v %v, want %v %v", chart.AssetNames, chart.ValueMicros, wantNames, wantValues)
	}
}

func TestEquityQuarterlySectionsWithoutData(t *testing.T) {
	s := ledgerViewTestServer(t)
	for _, tc := range []struct {
		date     civil.Date
		want     []string
		dontWant []string
	}{
		// The stock was bought in 2024: held, but no purchases two years later.
		// No price changes either, so all P&L totals are zero: table, but no chart.
		{civil.DateVal(2026, 6, 30), []string{"No purchases between 2025-04-01 and 2026-06-30.", "Quarterly Profit & Loss"}, []string{`id="quarterly-purchases-chart"`, `id="quarterly-pl-chart"`, "No equity positions"}},
		{civil.DateVal(2026, 7, 15), []string{"No purchases between 2025-07-01 and 2026-07-15."}, []string{`id="quarterly-purchases-chart"`}},
		// Before any entries: no equity positions at all.
		{civil.DateVal(2020, 6, 30), []string{"No purchases between 2019-04-01 and 2020-06-30.", "No equity positions since 2019-04-01."}, []string{`id="quarterly-purchases-chart"`, `id="quarterly-pl-chart"`}},
	} {
		var b bytes.Buffer
		r := httptest.NewRequest(http.MethodGet, "/kontoo/positions/equity", nil)
		if err := s.renderEquityPositionsTemplate(&b, r, tc.date); err != nil {
			t.Fatal(err)
		}
		for _, w := range tc.want {
			if !strings.Contains(b.String(), w) {
				t.Errorf("%v: missing %q", tc.date, w)
			}
		}
		for _, w := range tc.dontWant {
			if strings.Contains(b.String(), w) {
				t.Errorf("%v: unexpected %q", tc.date, w)
			}
		}
	}
}

func TestEquityQuarterlyChartData(t *testing.T) {
	s := ledgerViewTestServer(t)
	store, err := ledger.NewStore(&ledger.Ledger{
		Header: &ledger.LedgerHeader{BaseCurrency: "CHF"},
		Assets: []*ledger.Asset{{CustomID: "stock", Name: "Stock", Type: ledger.Stock, Currency: "CHF"}},
		Entries: []*ledger.LedgerEntry{
			{SequenceNum: 1, AssetID: "stock", Currency: "CHF", Type: ledger.AssetPurchase, ValueDate: civil.DateVal(2024, 1, 1), QuantityMicros: units.One, PriceMicros: units.One / 2},
			{SequenceNum: 2, AssetID: "stock", Currency: "CHF", Type: ledger.AssetPrice, ValueDate: civil.DateVal(2024, 3, 1), PriceMicros: units.One / 4},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	s.store = store
	var b bytes.Buffer
	if err := s.renderEquityPositionsTemplate(&b, httptest.NewRequest(http.MethodGet, "/kontoo/positions/equity", nil), civil.DateVal(2024, 3, 31)); err != nil {
		t.Fatal(err)
	}
	_, script, found := strings.Cut(b.String(), `<script id="quarterly-chart-data" type="application/json">`)
	if !found {
		t.Fatal("missing quarterly chart data")
	}
	script, _, _ = strings.Cut(script, "</script>")
	var data struct {
		Currency   string   `json:"currency"`
		Labels     []string `json:"labels"`
		Purchases  []int64  `json:"purchases"`
		ProfitLoss []int64  `json:"profitLoss"`
	}
	if err := json.Unmarshal([]byte(script), &data); err != nil {
		t.Fatalf("chart script must contain valid JSON: %v", err)
	}
	if data.Currency != "CHF" || len(data.Labels) != 5 || !slices.Equal(data.Purchases, []int64{0, 0, 0, 0, 500000}) || !slices.Equal(data.ProfitLoss, []int64{0, 0, 0, 0, -250000}) {
		t.Fatalf("incorrect chart data for fractional purchases and losses: %+v", data)
	}
	for _, id := range []string{"quarterly-purchases-chart", "quarterly-pl-chart"} {
		if !strings.Contains(b.String(), `id="`+id+`"`) {
			t.Errorf("missing %s for a nonzero amount", id)
		}
	}
}
