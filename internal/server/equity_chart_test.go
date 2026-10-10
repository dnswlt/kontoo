package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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
	body, _ := json.Marshal(EquityChartRequest{EndTimestamp: civil.DateVal(2024, 6, 30).UnixMilli()})
	w := httptest.NewRecorder()
	s.handleChartsEquity(w, httptest.NewRequest(http.MethodPost, "/kontoo/charts/equity", bytes.NewReader(body)))
	var resp EquityChartResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"EUR stock", "CHF stock"}
	wantValues := []int64{int64(1000 * units.One), int64(550 * units.One)}
	if !slices.Equal(resp.AssetNames, wantNames) || !slices.Equal(resp.ValueMicros, wantValues) {
		t.Errorf("got %v %v, want %v %v", resp.AssetNames, resp.ValueMicros, wantNames, wantValues)
	}
}
