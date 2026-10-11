package calc

import (
	"errors"

	"github.com/dnswlt/kontoo/internal/units"
)

var (
	ErrTargetSum     = errors.New("the targets do not add up to 100%")
	ErrNotEnoughCash = errors.New("not enough cash to withdraw the amount without selling")
)

// AllocationItem is a part of a portfolio, such as an asset category.
type AllocationItem struct {
	Value  units.Micros
	Target units.Micros // Share of the total after rebalancing.
	// Cash can be used to buy the other items without selling anything.
	Cash bool
}

// Rebalance returns the value of each item after investing amount (or
// withdrawing it, if negative): its target share of the new total. The targets
// must add up to 100%.
//
// With buyOnly, nothing is sold: an item other than cash whose value is above
// its target keeps its value, and the other items share the rest in proportion
// to their targets.
func Rebalance(items []AllocationItem, amount units.Micros, buyOnly bool) ([]units.Micros, error) {
	var total, targetSum units.Micros
	for _, it := range items {
		total += it.Value
		targetSum += it.Target
	}
	if targetSum != units.One {
		return nil, ErrTargetSum
	}
	total += amount
	newValues := make([]units.Micros, len(items))
	kept := make([]bool, len(items))
	for {
		rest, restTarget := total, units.Micros(0)
		for i, it := range items {
			if kept[i] {
				rest -= it.Value
			} else {
				restTarget += it.Target
			}
		}
		if restTarget == 0 && rest != 0 {
			// Only when buying only: all items with a target are kept.
			return nil, ErrNotEnoughCash
		}
		// Keeping an item leaves less for the others, so items are only added to kept.
		above := false
		for i, it := range items {
			switch {
			case kept[i]:
				newValues[i] = it.Value
			case restTarget == 0:
				newValues[i] = 0
			default:
				newValues[i] = rest.Frac(it.Target, restTarget)
				if buyOnly && !it.Cash && newValues[i] < it.Value {
					kept[i] = true
					above = true
				}
			}
		}
		if !above {
			break
		}
	}
	if buyOnly {
		for i, it := range items {
			if it.Cash && newValues[i] < 0 {
				return nil, ErrNotEnoughCash
			}
		}
	}
	return newValues, nil
}
