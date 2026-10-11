package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/units"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/net/html"
)

func renderCalcAllocation(t *testing.T, s *Server) *html.Node {
	t.Helper()
	var b bytes.Buffer
	if err := s.renderCalcTemplate(&b, httptest.NewRequest(http.MethodGet, "/kontoo/calc/allocation", nil), "allocation"); err != nil {
		t.Fatal(err)
	}
	doc, err := html.Parse(&b)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCalcAllocationCategoriesInBaseCurrency(t *testing.T) {
	// The test ledger has 125 CHF in a checking account and a stock worth 20 USD.
	s := ledgerViewTestServer(t)
	if err := s.Store().Add(&ledger.LedgerEntry{ValueDate: civil.DateVal(2024, 3, 1), Type: ledger.ExchangeRate, Currency: "CHF", QuoteCurrency: "USD", PriceMicros: 1_250_000}); err != nil {
		t.Fatal(err)
	}
	doc := renderCalcAllocation(t, s)
	rows := map[string]*html.Node{}
	for _, tr := range findElements(doc, "tr") {
		if name := htmlAttr(tr, "data-category"); name != "" {
			rows[name] = tr
		}
	}
	if len(rows) != len(ledger.AssetCategories()) {
		t.Errorf("got %d categories, want all %d", len(rows), len(ledger.AssetCategories()))
	}
	tests := []struct {
		category string
		value    units.Micros
		included bool
	}{
		{"Equity", 16 * units.One, true},
		{"Cash equivalents", 125 * units.One, true},
		{"Commodities", 0, true},
		{"Retirement savings", 0, false},
		{"Taxes", 0, false},
		{"Debt", 0, false},
	}
	for _, tc := range tests {
		tr := rows[tc.category]
		if tr == nil {
			t.Errorf("missing category %s", tc.category)
			continue
		}
		if got, want := htmlAttr(tr, "data-value"), strconv.FormatInt(int64(tc.value), 10); got != want {
			t.Errorf("%s: got value %s micros, want %s", tc.category, got, want)
		}
		if got := hasAttr(tr, "data-included"); got != tc.included {
			t.Errorf("%s: got included %t, want %t", tc.category, got, tc.included)
		}
	}
}

func TestCalcAllocationMissingExchangeRate(t *testing.T) {
	// The test ledger's stock is in USD, with no CHF/USD rate.
	doc := renderCalcAllocation(t, ledgerViewTestServer(t))
	if callout := findElement(doc, "div", "class", "callout callout-warn"); callout == nil || !strings.Contains(textContent(callout), "No exchange rate for USD") {
		t.Error("missing exchange-rate warning")
	}
	if findElement(doc, "form", "id", "allocation-form") != nil {
		t.Error("the planner must not be shown without all exchange rates")
	}
}

func TestCalculateAllocation(t *testing.T) {
	// The test ledger has 125 CHF in a checking account and a stock worth 20 USD,
	// i.e. 16 CHF at 1.25 USD per CHF.
	s := ledgerViewTestServer(t)
	if err := s.Store().Add(&ledger.LedgerEntry{ValueDate: civil.DateVal(2024, 3, 1), Type: ledger.ExchangeRate, Currency: "CHF", QuoteCurrency: "USD", PriceMicros: 1_250_000}); err != nil {
		t.Fatal(err)
	}
	post := func(body string) (int, CalculateAllocationResponse) {
		w := httptest.NewRecorder()
		s.handleCalculateAllocation(w, httptest.NewRequest(http.MethodPost, "/kontoo/calculate/allocation", strings.NewReader(body)))
		var resp CalculateAllocationResponse
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatal(err)
			}
		}
		return w.Code, resp
	}

	_, resp := post(`{"targets": [{"category": "Equity", "target": "50%"}, {"category": "Cash equivalents", "target": "50%"}], "amount": "9"}`)
	want := CalculateAllocationResponse{
		Status: StatusOK,
		Rows: []*AllocationResult{
			{Change: "59.00", NewValue: "75.00", NewShare: "50.0%"},
			{Change: "(50.00)", NewValue: "75.00", NewShare: "50.0%"},
		},
		Total: &AllocationResult{Change: "9.00", NewValue: "150.00", NewShare: "100.0%"},
	}
	if diff := cmp.Diff(want, resp); diff != "" {
		t.Errorf("buy and sell: diff (-want +got):\n%s", diff)
	}

	// Buying only, cash pays for the purchase.
	_, resp = post(`{"targets": [{"category": "Equity", "target": "50%"}, {"category": "Cash equivalents", "target": "50%"}], "amount": "0", "buyOnly": true}`)
	if len(resp.Rows) != 2 || resp.Rows[0].Change != "54.50" || resp.Rows[1].Change != "(54.50)" {
		t.Errorf("buy only: got %+v", resp)
	}

	_, resp = post(`{"targets": [{"category": "Equity", "target": "50%"}, {"category": "Cash equivalents", "target": "40%"}]}`)
	if resp.Status != StatusInvalidArgument || resp.Error != "The targets do not add up to 100%." {
		t.Errorf("targets not 100%%: got %+v", resp)
	}

	if code, _ := post(`{"targets": [{"category": "Gold", "target": "100%"}]}`); code != http.StatusBadRequest {
		t.Errorf("unknown category: got status %d, want %d", code, http.StatusBadRequest)
	}
}
