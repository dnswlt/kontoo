package calc

import (
	"errors"
	"slices"
	"testing"

	"github.com/dnswlt/kontoo/internal/units"
)

func TestRebalance(t *testing.T) {
	const one = units.One
	pct := func(p int64) units.Micros { return units.Micros(p * 10_000) }
	tests := []struct {
		name    string
		items   []AllocationItem
		amount  units.Micros
		buyOnly bool
		want    []units.Micros
		wantErr error
	}{
		{
			name:  "buy and sell",
			items: []AllocationItem{{Value: 600 * one, Target: pct(50)}, {Value: 300 * one, Target: pct(30)}, {Value: 100 * one, Target: pct(20), Cash: true}},
			want:  []units.Micros{500 * one, 300 * one, 200 * one},
		},
		{
			name:   "buy and sell with new money",
			items:  []AllocationItem{{Value: 600 * one, Target: pct(50)}, {Value: 300 * one, Target: pct(30)}, {Value: 100 * one, Target: pct(20), Cash: true}},
			amount: 1000 * one,
			want:   []units.Micros{1000 * one, 600 * one, 400 * one},
		},
		{
			name:    "targets not 100%",
			items:   []AllocationItem{{Value: 600 * one, Target: pct(50)}, {Value: 400 * one, Target: pct(49)}},
			wantErr: ErrTargetSum,
		},
		{
			name:    "buy only: cash buys what is below target",
			items:   []AllocationItem{{Value: 500 * one, Target: pct(60)}, {Value: 300 * one, Target: pct(30)}, {Value: 200 * one, Target: pct(10), Cash: true}},
			buyOnly: true,
			want:    []units.Micros{600 * one, 300 * one, 100 * one},
		},
		{
			name:    "buy only: above target keeps its value",
			items:   []AllocationItem{{Value: 800 * one, Target: pct(50)}, {Value: 100 * one, Target: pct(30)}, {Value: 100 * one, Target: pct(20), Cash: true}},
			amount:  200 * one,
			buyOnly: true,
			want:    []units.Micros{800 * one, 240 * one, 160 * one},
		},
		{
			name:    "buy only: withdrawal from cash",
			items:   []AllocationItem{{Value: 900 * one, Target: pct(90)}, {Value: 100 * one, Target: pct(10), Cash: true}},
			amount:  -50 * one,
			buyOnly: true,
			want:    []units.Micros{900 * one, 50 * one},
		},
		{
			name:    "buy only: withdrawal beyond cash",
			items:   []AllocationItem{{Value: 900 * one, Target: pct(90)}, {Value: 100 * one, Target: pct(10), Cash: true}},
			amount:  -200 * one,
			buyOnly: true,
			wantErr: ErrNotEnoughCash,
		},
		{
			name:    "buy only: withdrawal without cash",
			items:   []AllocationItem{{Value: 500 * one, Target: pct(50)}, {Value: 500 * one, Target: pct(50)}},
			amount:  -100 * one,
			buyOnly: true,
			wantErr: ErrNotEnoughCash,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Rebalance(tc.items, tc.amount, tc.buyOnly)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got error %v, want %v", err, tc.wantErr)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
