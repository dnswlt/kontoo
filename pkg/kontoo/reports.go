package kontoo

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"
)

type ReportingPeriod struct {
	Start Date
	End   Date
	Label string
}

type ReportingPeriodData struct {
	Period *ReportingPeriod
	// "Columns" of the report, i.e. different values for all assets of the report.
	// Make sure to update sortAssets() and ensureLength() when adding columns!
	Purchases       []Micros
	PurchasesTotal  Micros
	MarketValue     []Micros
	ProfitLoss      []Micros
	ProfitLossRatio []Micros
	ProfitLossTotal Micros // Sum of ProfitLoss in base currency
}

type Report struct {
	Assets []*Asset // Assets for which data exists in the report.
	Data   []*ReportingPeriodData
}

// AssetsWithPurchases returns indices into Assets of assets that had any non-zero purchase.
func (r *Report) AssetsWithPurchases() []int {
	var res []int
	for i := range r.Assets {
		for _, d := range r.Data {
			if d.Purchases[i] != 0 {
				res = append(res, i)
				break
			}
		}
	}
	return res
}

func (d *ReportingPeriodData) ensureLength(n int) {
	if len(d.Purchases) >= n {
		// Assume invariant that all fields are of the same length already.
		return
	}
	dl := n - len(d.Purchases)
	d.Purchases = append(d.Purchases, make([]Micros, dl)...)
	d.MarketValue = append(d.MarketValue, make([]Micros, dl)...)
	d.ProfitLoss = append(d.ProfitLoss, make([]Micros, dl)...)
	d.ProfitLossRatio = append(d.ProfitLossRatio, make([]Micros, dl)...)
}

// quarterlyPeriods returns the numPeriods quarterly reporting periods up to endDate.
func quarterlyPeriods(endDate Date, numPeriods int) []*ReportingPeriod {
	if numPeriods <= 0 {
		return nil
	}
	ps := make([]*ReportingPeriod, numPeriods)
	// Last quarter might not end on a typical quarter end.
	i := numPeriods - 1
	m := ((endDate.Month()-1)/3)*3 + 1 // Truncate to nearest quarter start
	ps[i] = &ReportingPeriod{
		Start: DateVal(endDate.Year(), m, 1),
		End:   endDate,
	}
	// Other periods are regular:
	i--
	for ; i >= 0; i-- {
		ps[i] = &ReportingPeriod{
			Start: Date{ps[i+1].Start.AddDate(0, -3, 0)},
			End:   ps[i+1].Start.AddDays(-1),
		}
	}
	// Add labels
	for _, p := range ps {
		q := (p.Start.Month() + 2) / 3
		p.Label = fmt.Sprintf("%02dQ%d", p.Start.Year()%100, q)
	}
	return ps
}

// Sort assets and their data in the report by the given asset comparison function.
func (r *Report) sortAssets(less func(a, b *Asset) bool) {
	reorderMicros := func(indices []int, ms []Micros) {
		tmp := make([]Micros, len(indices))
		for i, idx := range indices {
			tmp[i] = ms[idx]
		}
		copy(ms, tmp)
	}
	// Sort indices by the ordering induced on the assets by `less`.
	indices := make([]int, len(r.Assets))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		return less(r.Assets[indices[i]], r.Assets[indices[j]])
	})
	// Now reorder assets and all values by the ordering given by indices.
	tmpAssets := make([]*Asset, len(r.Assets))
	for i, idx := range indices {
		tmpAssets[i] = r.Assets[idx]
	}
	copy(r.Assets, tmpAssets)
	for _, d := range r.Data {
		reorderMicros(indices, d.Purchases)
		reorderMicros(indices, d.MarketValue)
		reorderMicros(indices, d.ProfitLoss)
		reorderMicros(indices, d.ProfitLossRatio)
	}
}

// AssetsInPeriod returns all assets that had a non-zero value at any point in time
// between startDate and endDate.
func (s *Store) AssetsInPeriod(startDate, endDate Date) []*Asset {
	var res []*Asset
	for assetId, asset := range s.assets {
		// Existed at start date:
		if pos := s.AssetPositionAt(assetId, startDate); pos.MarketValue() != 0 {
			res = append(res, asset)
			continue
		}
		// Had activity between start and end date:
		if entries := s.EntriesInRange(assetId, startDate, endDate); len(entries) > 0 {
			res = append(res, asset)
		}
	}
	return res
}

func (s *Store) QuarterlyReport(endDate Date, numPeriods int) *Report {
	report := &Report{}
	// assetIdx := make(map[string]int)
	periods := quarterlyPeriods(endDate, numPeriods)
	report.Data = make([]*ReportingPeriodData, len(periods))
	// Collect all equity assets that had a non-zero value at any period.End.
	// In each reporting period, we will iterate over these assets to calculate
	// their P&L and purchases/sales.
	allAssets := s.AssetsInPeriod(periods[0].Start, periods[len(periods)-1].End)
	equityAssets := make([]*Asset, 0, len(allAssets))
	for _, a := range allAssets {
		if a.Category() == Equity {
			equityAssets = append(equityAssets, a)
		}
	}
	slices.SortFunc(equityAssets, func(a, b *Asset) int {
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	for i, period := range periods {
		report.Data[i] = &ReportingPeriodData{}
		report.Data[i].Period = period
		report.Data[i].ensureLength(len(equityAssets))
		var purchasesTotal, plTotal Micros
		for j, asset := range equityAssets {
			pos := s.AssetPositionAt(asset.ID(), period.End)
			id := pos.Asset.ID()
			pur := s.AssetPurchases(id, period.Start, period.End)
			profitLoss, profitLossBasis, err := s.ProfitLossInPeriod(id, period.Start, period.End)
			if err == nil {
				report.Data[i].ProfitLoss[j] = profitLoss
				if profitLossBasis != 0 {
					report.Data[i].ProfitLossRatio[j] = profitLoss.Div(profitLossBasis)
				}
			}
			report.Data[i].Purchases[j] = pur
			report.Data[i].MarketValue[j] = pos.MarketValue()
			// Update total purchases and total P&L in base currency.
			rate, _, rateFound := s.ExchangeRateAt(pos.Currency(), period.End)
			if rateFound {
				purchasesTotal += pur.Div(rate)
				plTotal += profitLoss.Div(rate)
			}
		}
		report.Data[i].PurchasesTotal = purchasesTotal
		report.Data[i].ProfitLossTotal = plTotal
	}
	// Add assets to report, in the order of their data.
	report.Assets = equityAssets
	return report
}
