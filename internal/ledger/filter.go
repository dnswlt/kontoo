package ledger

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/dnswlt/kontoo/internal/civil"
)

// EntryFilters is the structured alternative to the advanced query language.
// Selected assets are ORed; search and date range narrow the result further.
type EntryFilters struct {
	Search         string
	AssetIDs       []string
	From, Until    civil.Date
	HideMarketData bool
}

func NewFilterQuery(filters EntryFilters) (*Query, error) {
	if !filters.From.IsZero() && !filters.Until.IsZero() && filters.From.After(filters.Until) {
		return nil, fmt.Errorf("start date must be on or before end date")
	}
	q := &Query{
		filters:   &filters,
		terms:     strings.Fields(strings.ToLower(filters.Search)),
		fromDate:  filters.From,
		untilDate: filters.Until,
	}
	q.SetOrdering("-valuedate,-num")
	return q, nil
}

func (f *EntryFilters) match(e *LedgerEntryRow) bool {
	if f.HideMarketData && (e.EntryType() == AssetPrice || e.EntryType() == ExchangeRate) {
		return false
	}
	return len(f.AssetIDs) == 0 || slices.Contains(f.AssetIDs, e.AssetID())
}

func (q *Query) SetOrdering(order string) error {
	ordering, err := parseOrdering(order)
	if err != nil {
		return err
	}
	q.ordering = ordering
	return nil
}

// Ordering returns the effective sort expression, including advanced queries.
func (q *Query) Ordering() string {
	if len(q.ordering) == 0 {
		if q.descending {
			return "-valuedate"
		}
		return "valuedate"
	}
	var fields []string
	for _, field := range q.ordering {
		name := field.name
		if field.descending {
			name = "-" + name
		}
		fields = append(fields, name)
	}
	return strings.Join(fields, ",")
}

func init() {
	rowCompareFuncs["type"] = func(a, b *LedgerEntryRow) int {
		return cmp.Compare(a.EntryType().DisplayName(), b.EntryType().DisplayName())
	}
	rowCompareFuncs["class"] = func(a, b *LedgerEntryRow) int {
		return cmp.Compare(a.AssetType().DisplayName(), b.AssetType().DisplayName())
	}
	rowCompareFuncs["currency"] = func(a, b *LedgerEntryRow) int { return cmp.Compare(a.Currency(), b.Currency()) }
	rowCompareFuncs["value"] = func(a, b *LedgerEntryRow) int { return cmp.Compare(a.EntryMarketValue(), b.EntryMarketValue()) }
	rowCompareFuncs["cost"] = func(a, b *LedgerEntryRow) int { return cmp.Compare(a.Cost(), b.Cost()) }
	rowCompareFuncs["quantity"] = func(a, b *LedgerEntryRow) int { return cmp.Compare(a.Quantity(), b.Quantity()) }
	rowCompareFuncs["price"] = func(a, b *LedgerEntryRow) int { return cmp.Compare(a.Price(), b.Price()) }
	rowCompareFuncs["totalvalue"] = func(a, b *LedgerEntryRow) int { return cmp.Compare(a.TotalMarketValue(), b.TotalMarketValue()) }
	rowCompareFuncs["totalquantity"] = func(a, b *LedgerEntryRow) int { return cmp.Compare(a.TotalQuantity(), b.TotalQuantity()) }
	rowCompareFuncs["totalcost"] = func(a, b *LedgerEntryRow) int { return cmp.Compare(a.TotalCost(), b.TotalCost()) }
	rowCompareFuncs["comment"] = func(a, b *LedgerEntryRow) int { return cmp.Compare(a.Comment(), b.Comment()) }
}
