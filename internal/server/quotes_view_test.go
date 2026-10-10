package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/quotes"
	"github.com/dnswlt/kontoo/internal/units"
	"golang.org/x/net/html"
)

func TestExchangeRateQuoteAvailability(t *testing.T) {
	s := ledgerViewTestServer(t)
	for _, currency := range []units.Currency{"USD", "JPY"} {
		if err := s.store.Add(&ledger.LedgerEntry{
			Type: ledger.ExchangeRate, Currency: "CHF", QuoteCurrency: currency,
			ValueDate: civil.DateVal(2024, 3, 5), PriceMicros: units.One,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.store.Add(&ledger.LedgerEntry{
		Type: ledger.ExchangeRate, Currency: "CHF", QuoteCurrency: "USD",
		ValueDate: civil.DateVal(2024, 2, 23), PriceMicros: units.One,
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		currency    units.Currency
		quoteDate   civil.Date
		latest, age string
	}{
		{"USD", civil.DateVal(2024, 3, 1), "2024-02-23", "7"},
		{"USD", civil.DateVal(2024, 2, 23), "2024-02-23", "0"},
		{"EUR", civil.DateVal(2024, 3, 1), "n/a", ""},
		{"JPY", civil.DateVal(2024, 3, 1), "n/a", ""}, // Only a future rate exists.
	} {
		rates := s.exchangeRateQuoteEntries([]*quotes.DailyExchangeRate{{
			BaseCurrency: "CHF", QuoteCurrency: tc.currency, Symbol: "CHF" + string(tc.currency) + "=X",
			Timestamp: tc.quoteDate.In(time.UTC).Add(18 * time.Hour), ClosingPrice: units.One,
		}})
		// A Sunday report receives Friday's quote: age is relative to Friday.
		ctx := s.addCommonCtx(httptest.NewRequest(http.MethodGet, "/kontoo/quotes", nil), map[string]any{
			"QuoteDate": civil.DateVal(2024, 3, 3), "ExchangeRates": rates,
		})
		var b bytes.Buffer
		if err := s.templates.ExecuteTemplate(&b, "quotes.html", ctx); err != nil {
			t.Fatal(err)
		}
		doc, err := html.Parse(&b)
		if err != nil {
			t.Fatal(err)
		}
		input := findElement(doc, "input", "id", "rate-0")
		if input == nil {
			t.Fatal("missing exchange-rate row")
		}
		row := closest(input, "tr")
		if headers, cells := findElements(closest(row, "table"), "th"), findElements(row, "td"); len(headers) != len(cells) {
			t.Fatalf("exchange-rate table has %d headers but %d cells per row", len(headers), len(cells))
		}
		cells := rowCells(row)
		if cells["Latest avail."] != tc.latest || cells["Δd"] != tc.age {
			t.Errorf("%s at %v: got latest %q, age %q; want %q, %q", tc.currency, tc.quoteDate, cells["Latest avail."], cells["Δd"], tc.latest, tc.age)
		}
	}
}

func TestQuotesEmptyStateAdvice(t *testing.T) {
	s := ledgerViewTestServer(t)
	for _, hasSymbols := range []bool{false, true} {
		ctx := s.addCommonCtx(httptest.NewRequest(http.MethodGet, "/kontoo/quotes", nil), map[string]any{
			"QuoteDate": civil.DateVal(2024, 3, 1), "HasQuoteSymbols": hasSymbols,
		})
		var b bytes.Buffer
		if err := s.templates.ExecuteTemplate(&b, "quotes.html", ctx); err != nil {
			t.Fatal(err)
		}
		// The advice only helps if no asset has a symbol yet.
		if got := strings.Contains(b.String(), "Consider adding YF ticker symbols"); got == hasSymbols {
			t.Errorf("hasSymbols=%t: advice shown=%t", hasSymbols, got)
		}
	}
}
