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
		Currency string
		Labels   []string
		Values   []int64
	}
	if err := json.Unmarshal([]byte(textContent(script)), &data); err != nil {
		t.Fatal(err)
	}
	// Account: 100 + 25 CHF. Stock: 1 share at 20 USD, i.e. 10 CHF. Largest first.
	if data.Currency != "CHF" || !slices.Equal(data.Labels, []string{"Cash equivalents", "Equity"}) ||
		!slices.Equal(data.Values, []int64{int64(125 * units.One), int64(10 * units.One)}) {
		t.Errorf("got %+v", data)
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
