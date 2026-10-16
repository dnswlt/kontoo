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
	"github.com/google/go-cmp/cmp"
	"golang.org/x/net/html"
)

func renderPositions(t *testing.T, s *Server, date civil.Date) *html.Node {
	t.Helper()
	var b bytes.Buffer
	if err := s.renderPositionsTemplate(&b, httptest.NewRequest(http.MethodGet, "/kontoo/positions", nil), date); err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(&b)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestPositionsMissingExchangeRate(t *testing.T) {
	// The test ledger's stock is in USD, with no CHF/USD rate.
	doc := renderPositions(t, ledgerViewTestServer(t), civil.DateVal(2024, 3, 1))
	if callout := findElement(doc, "div", "class", "callout callout-warn"); callout == nil || !strings.Contains(textContent(callout), "No exchange rate for USD") {
		t.Error("missing exchange-rate warning")
	}
	if findElement(doc, "tr", "class", "total") != nil || findElement(doc, "tr", "class", "subtotal") != nil {
		t.Error("totals must not be shown without all exchange rates")
	}
	if findElement(doc, "canvas", "id", "category-canvas") != nil {
		t.Error("category chart must not be shown without all exchange rates")
	}
}

func TestPositionsCategoryChart(t *testing.T) {
	s := ledgerViewTestServer(t)
	if err := s.store.Add(&ledger.LedgerEntry{Type: ledger.ExchangeRate, Currency: "CHF", QuoteCurrency: "USD",
		ValueDate: civil.DateVal(2024, 1, 1), PriceMicros: 2 * units.One}); err != nil {
		t.Fatal(err)
	}
	doc := renderPositions(t, s, civil.DateVal(2024, 3, 1))
	script := findElement(doc, "script", "id", "category-chart-data")
	if script == nil {
		t.Fatal("missing category chart data")
	}
	var data struct {
		Currency         string
		Labels           []string
		Values           []int64
		Currencies       []string
		ValuesByCurrency [][]int64
	}
	if err := json.Unmarshal([]byte(textContent(script)), &data); err != nil {
		t.Fatal(err)
	}
	// Account: 100 + 25 CHF. Stock: 1 share at 20 USD, i.e. 10 CHF. Largest first.
	if data.Currency != "CHF" || !slices.Equal(data.Labels, []string{"Cash equivalents", "Equity"}) ||
		!slices.Equal(data.Values, []int64{int64(125 * units.One), int64(10 * units.One)}) {
		t.Errorf("got %+v", data)
	}
	// Per currency (base currency first), in CHF: the account is in CHF, the stock in USD.
	if !slices.Equal(data.Currencies, []string{"CHF", "USD"}) || len(data.ValuesByCurrency) != 2 ||
		!slices.Equal(data.ValuesByCurrency[0], []int64{int64(125 * units.One), 0}) ||
		!slices.Equal(data.ValuesByCurrency[1], []int64{0, int64(10 * units.One)}) {
		t.Errorf("got currencies %v, values by currency %v", data.Currencies, data.ValuesByCurrency)
	}
}

// Each position's action menu links to the ledger, filtered to that asset.
func TestPositionActionsShowLedger(t *testing.T) {
	s := ledgerViewTestServer(t)
	r := httptest.NewRequest(http.MethodGet, "/kontoo/positions", nil)
	date := civil.DateVal(2024, 3, 1)
	maturing := maturitiesTestServer(t, true)
	for _, tc := range []struct {
		name    string
		render  func(*bytes.Buffer) error
		assetID string
	}{
		{"all", func(b *bytes.Buffer) error { return s.renderPositionsTemplate(b, r, date) }, "account"},
		{"equity", func(b *bytes.Buffer) error { return s.renderEquityPositionsTemplate(b, r, date) }, "stock"},
		{"maturing", func(b *bytes.Buffer) error { return maturing.renderMaturingPositionsTemplate(b, r, date) }, "US0000000001"},
	} {
		var b bytes.Buffer
		if err := tc.render(&b); err != nil {
			t.Fatal(err)
		}
		doc, err := html.Parse(&b)
		if err != nil {
			t.Fatal(err)
		}
		menu := findElement(doc, "div", "id", "position-actions-"+tc.assetID)
		if menu == nil {
			t.Fatalf("%s: no action menu for %s", tc.name, tc.assetID)
		}
		link := findElement(menu, "a", "href", "/kontoo/ledger?asset="+tc.assetID)
		if link == nil || textContent(link) != "Show ledger" {
			t.Errorf("%s: missing Show ledger link for %s", tc.name, tc.assetID)
		}
	}
}

func TestValueChartParams(t *testing.T) {
	for _, tc := range []struct {
		query, cookie, wantPeriod string
		wantAssets                []string
	}{
		{"", "", "1Y", nil},
		{"", "Max", "Max", nil},         // The last chosen period, from the cookie.
		{"period=5Y", "Max", "5Y", nil}, // The URL wins over the cookie.
		{"period=bogus", "", "1Y", nil}, // Invalid periods fall back to the default.
		{"chart=a&chart=b", "", "1Y", []string{"a", "b"}},
	} {
		r := httptest.NewRequest(http.MethodGet, "/kontoo/positions?"+tc.query, nil)
		if tc.cookie != "" {
			r.AddCookie(&http.Cookie{Name: valueChartPeriodCookie, Value: tc.cookie})
		}
		period, assets := valueChartParams(r)
		if period != tc.wantPeriod || !slices.Equal(assets, tc.wantAssets) {
			t.Errorf("%q, cookie %q: got %q %v, want %q %v", tc.query, tc.cookie, period, assets, tc.wantPeriod, tc.wantAssets)
		}
	}
}

// The page embeds the value chart's data, and the endpoint serves the same data
// for the page's parameters.
func TestValueChartEmbeddedAndServed(t *testing.T) {
	s := ledgerViewTestServer(t)
	type valueChart struct {
		Period    string
		AssetIDs  []string
		Timelines []struct{ AssetID, Label string }
	}
	query := "date=2024-03-01&period=Max&chart=stock&chart=missing"

	var b bytes.Buffer
	if err := s.renderPositionsTemplate(&b, httptest.NewRequest(http.MethodGet, "/kontoo/positions?"+query, nil), civil.DateVal(2024, 3, 1)); err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(&b)
	if err != nil {
		t.Fatal(err)
	}
	script := findElement(doc, "script", "id", "value-chart-data")
	if script == nil {
		t.Fatal("missing value chart data")
	}
	var embedded valueChart
	if err := json.Unmarshal([]byte(textContent(script)), &embedded); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.handlePositionsTimeline(w, httptest.NewRequest(http.MethodGet, "/kontoo/positions/timeline?"+query, nil))
	var served valueChart
	if err := json.NewDecoder(w.Body).Decode(&served); err != nil {
		t.Fatal(err)
	}

	for name, chart := range map[string]valueChart{"embedded": embedded, "served": served} {
		// Unknown assets are ignored.
		if chart.Period != "Max" || !slices.Equal(chart.AssetIDs, []string{"stock"}) ||
			len(chart.Timelines) != 1 || chart.Timelines[0].Label != "Beta stock" {
			t.Errorf("%s: got %+v", name, chart)
		}
	}

	// Without selected assets, the chart shows the total.
	w = httptest.NewRecorder()
	s.handlePositionsTimeline(w, httptest.NewRequest(http.MethodGet, "/kontoo/positions/timeline?date=2024-03-01", nil))
	var total valueChart
	if err := json.NewDecoder(w.Body).Decode(&total); err != nil {
		t.Fatal(err)
	}
	if total.Period != "1Y" || len(total.AssetIDs) != 0 || len(total.Timelines) != 1 || total.Timelines[0].Label != "Total" {
		t.Errorf("total: got %+v", total)
	}
}

func TestYearlyChangeChart(t *testing.T) {
	balance := func(seq int64, date civil.Date, value units.Micros) *ledger.LedgerEntry {
		return &ledger.LedgerEntry{SequenceNum: seq, AssetID: "account", Currency: "CHF", Type: ledger.AccountBalance, ValueDate: date, ValueMicros: value}
	}
	store, err := ledger.NewStore(&ledger.Ledger{
		Header: &ledger.LedgerHeader{BaseCurrency: "CHF"},
		Assets: []*ledger.Asset{{CustomID: "account", Name: "Alpha Bank", Type: ledger.CheckingAccount, Currency: "CHF"}},
		Entries: []*ledger.LedgerEntry{
			balance(1, civil.DateVal(2023, 6, 1), 100*units.One),
			balance(2, civil.DateVal(2024, 3, 1), 150*units.One),
			balance(3, civil.DateVal(2024, 12, 31), 120*units.One),
			balance(4, civil.DateVal(2026, 2, 1), 200*units.One),
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: store}

	// 2023 is the first year, with no value at the end of the previous year.
	want := &YearlyChangeChart{
		Currency:     "CHF",
		Labels:       []string{"2024", "2025", "2026 YTD"},
		ChangeMicros: []int64{20_000_000, 0, 80_000_000},
		ValueMicros:  []int64{120_000_000, 120_000_000, 200_000_000},
	}
	if diff := cmp.Diff(want, s.yearlyChangeChart(civil.DateVal(2026, 10, 16))); diff != "" {
		t.Errorf("yearlyChangeChart() mismatch (-want +got):\n%s", diff)
	}
	// On the last day of a year, the year is complete.
	if got := s.yearlyChangeChart(civil.DateVal(2025, 12, 31)); !slices.Equal(got.Labels, []string{"2024", "2025"}) {
		t.Errorf("yearlyChangeChart() on 2025-12-31: got labels %q", got.Labels)
	}
	if got := s.yearlyChangeChart(civil.DateVal(2023, 12, 31)); len(got.Labels) != 0 {
		t.Errorf("yearlyChangeChart() in the first year: got labels %q, want none", got.Labels)
	}
}
