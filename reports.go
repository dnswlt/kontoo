package kontoo

import (
	"fmt"
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
	MarketValue     []Micros
	ProfitLoss      []Micros
	ProfitLossRatio []Micros
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

func (s *Store) QuarterlyReport(endDate Date, numPeriods int) *Report {
	report := &Report{}
	assetIdx := make(map[string]int)
	periods := quarterlyPeriods(endDate, numPeriods)
	report.Data = make([]*ReportingPeriodData, len(periods))
	for i, period := range periods {
		report.Data[i] = &ReportingPeriodData{}
		report.Data[i].Period = period
		// TODO: Include all equity assets with non-zero value at any period.End, not just the last one.
		// If we get them in advance, we could also iterate over the assets in the right order,
		// avoiding the need to call .sortAssets below.
		positions := s.AssetPositionsAt(period.End)
		for _, pos := range positions {
			if pos.Asset.Category() != Equity {
				continue
			}
			id := pos.Asset.ID()
			j, ok := assetIdx[id]
			if !ok {
				j = len(assetIdx)
				assetIdx[id] = j
			}
			report.Data[i].ensureLength(j + 1)
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
		}
	}
	// Add assets to report, in the order of their data.
	report.Assets = make([]*Asset, len(assetIdx))
	for id, i := range assetIdx {
		report.Assets[i] = s.assets[id]
	}
	// Ensure all quarters have data for all assets (rectangular data).
	for _, d := range report.Data {
		d.ensureLength(len(assetIdx))
	}
	report.sortAssets(func(a, b *Asset) bool {
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return report
}
