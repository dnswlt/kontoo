package kontoo

import (
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestAddBuyStockTransaction(t *testing.T) {
	l := NewLedger()
	params := &StockTransactionParams{
		txType:    BuyTransaction,
		valueDate: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		asset: Asset{
			Id:   "IE00B4L5Y983",
			Type: StockExchangeTradedFund,
			Name: "iShares Core MSCI World UCITS ETF USD (Acc)",
		},
		currency:       EUR,
		priceMicros:    20 * UnitValue,
		quantityMicros: 75 * UnitValue,
		valueMicros:    Mmul(20*UnitValue, 75*UnitValue),
		costMicros:     12_500_000,
	}
	e, err := l.AddStockTransaction(params)
	if err != nil {
		t.Fatalf("could not add to ledger: %s", err)
	}
	if e.ValueMicros != params.valueMicros {
		t.Errorf("ValueMicros: want %d, got %d", params.valueMicros, e.ValueMicros)
	}
	if e.SequenceNum != 0 {
		t.Errorf("SequenceNum: want %d, got %d", 0, e.SequenceNum)
	}
	if e.Type != BuyTransaction {
		t.Errorf("BuyTransaction: want %d, got %d", BuyTransaction, e.Type)
	}
	if len(l.entries) != 1 {
		t.Errorf("number of entries: want %d, got %d", 1, len(l.entries))
	}
	if e.CostMicros != params.costMicros {
		t.Errorf("CostMicros: want %d, got %d", params.costMicros, e.CostMicros)
	}
}

func TestAssetIds(t *testing.T) {
	l := NewLedger()
	params1 := &StockTransactionParams{
		txType:    BuyTransaction,
		valueDate: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		asset: Asset{
			Id:   "IE00B4L5Y983",
			Type: StockExchangeTradedFund,
			Name: "iShares Core MSCI World UCITS ETF USD (Acc)",
		},
		currency:       EUR,
		priceMicros:    20 * UnitValue,
		quantityMicros: 75 * UnitValue,
		costMicros:     12_500_000,
	}
	params2 := &StockTransactionParams{
		txType:    BuyTransaction,
		valueDate: time.Date(2023, 2, 1, 12, 0, 0, 0, time.UTC),
		asset: Asset{
			Id:   "IE00BTJRMP35",
			Type: StockExchangeTradedFund,
			Name: "Xtrackers MSCI Emerging Markets UCITS ETF 1C",
		},
		currency:       USD,
		priceMicros:    45 * UnitValue,
		quantityMicros: 50 * UnitValue,
		costMicros:     15_000_000,
	}
	l.AddStockTransaction(params1)
	// Add twice to test deduplication.
	l.AddStockTransaction(params2)
	l.AddStockTransaction(params2)
	ids := l.AssetIds()
	sort.Strings(ids)
	if diff := cmp.Diff(ids, []string{"IE00B4L5Y983", "IE00BTJRMP35"}); diff != "" {
		t.Errorf("Unexpected AssetIds: -want +got: %s", diff)
	}
}
