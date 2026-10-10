package ledger

import (
	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/units"
	"slices"
	"testing"
)

func filterTestRows() []*LedgerEntryRow {
	account := &Asset{CustomID: "a:b", Name: "Alpha Bank", Type: CheckingAccount, Currency: "CHF"}
	stock := &Asset{CustomID: "stock", Name: "Beta stock", Type: Stock, Currency: "USD"}
	rows := []*LedgerEntryRow{}
	for i, item := range []struct {
		asset   *Asset
		typ     EntryType
		comment string
	}{{account, AccountCredit, "note: fee=0"}, {stock, AssetPurchase, "buy"}, {stock, DividendPayment, "dividend"}, {stock, AssetPrice, "quote"}, {nil, ExchangeRate, "FX"}} {
		e := &LedgerEntry{SequenceNum: int64(i + 1), Type: item.typ, ValueDate: civil.DateVal(2024, 2, 27+i), Comment: item.comment, Currency: "CHF", ValueMicros: units.Micros(5-i) * units.One}
		if item.asset != nil {
			e.AssetID, e.Currency = item.asset.ID(), item.asset.Currency
		}
		rows = append(rows, &LedgerEntryRow{E: e, A: item.asset})
	}
	return rows
}

func TestStructuredEntryFilters(t *testing.T) {
	tests := []struct {
		name    string
		filters EntryFilters
		want    []int64
	}{
		{"default includes market data", EntryFilters{}, []int64{1, 2, 3, 4, 5}},
		{"hide market data", EntryFilters{HideMarketData: true}, []int64{1, 2, 3}},
		{"literal query punctuation", EntryFilters{Search: "note: fee=0"}, []int64{1}},
		{"multiple assets", EntryFilters{AssetIDs: []string{"stock", "a:b"}}, []int64{1, 2, 3, 4}},
		{"asset and search", EntryFilters{AssetIDs: []string{"stock", "a:b"}, Search: "dividend"}, []int64{3}},
		{"asset IDs preserve punctuation", EntryFilters{AssetIDs: []string{"a:b"}}, []int64{1}},
		{"inclusive leap day", EntryFilters{From: civil.DateVal(2024, 2, 28), Until: civil.DateVal(2024, 2, 29)}, []int64{2, 3}},
		{"no matches", EntryFilters{Search: "missing"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query, err := NewFilterQuery(tc.filters)
			if err != nil {
				t.Fatal(err)
			}
			var got []int64
			for _, row := range filterTestRows() {
				if query.Match(row) {
					got = append(got, row.SequenceNum())
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
	if _, err := NewFilterQuery(EntryFilters{From: civil.DateVal(2024, 3, 1), Until: civil.DateVal(2024, 2, 29)}); err == nil {
		t.Fatal("reversed date range accepted")
	}
}

func TestStructuredSorting(t *testing.T) {
	query, err := NewFilterQuery(EntryFilters{})
	if err != nil {
		t.Fatal(err)
	}
	rows := filterTestRows()
	query.Sort(rows)
	if rows[0].SequenceNum() != 5 {
		t.Fatal("default order should be newest first")
	}
	if err := query.SetOrdering("value"); err != nil {
		t.Fatal(err)
	}
	query.Sort(rows)
	if rows[0].SequenceNum() != 5 || rows[4].SequenceNum() != 1 {
		t.Fatal("amounts must sort numerically")
	}
	if err := query.SetOrdering("-value"); err != nil {
		t.Fatal(err)
	}
	query.Sort(rows)
	if rows[0].SequenceNum() != 1 {
		t.Fatal("descending numeric sort failed")
	}
	if err := query.SetOrdering("unsupported"); err == nil {
		t.Fatal("unknown sort field accepted")
	}
	if query.Ordering() != "-value" {
		t.Fatal("invalid sort changed the existing order")
	}
}
