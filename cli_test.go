package kontoo

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestFields(t *testing.T) {
	ps := strings.Fields(" a    b c\n")
	if len(ps) != 3 {
		t.Errorf("want 3 elements, got %d", len(ps))
	}
}

func TestParseDecimalAsMicros(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"zero_int", "0", 0},
		{"zero_dec", "0.0", 0},
		{"zero_frac", ".0", 0},
		{"zero_idot", "0.", 0},
		{"one_int", "1", 1_000_000},
		{"ont_intfrac", "1.1", 1_100_000},
		{"frac_trailing_zero", "2.100", 2_100_000},
		{"nines", "999999.999999", 999_999_999_999},
		{"only_frac", ".123456", 123456},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := ParseDecimalAsMicros(tc.input)
			if err != nil {
				t.Fatalf("Error parsing decimal: %s", err)
			}
			if v != tc.expected {
				t.Errorf("Want: %d, got: %d", tc.expected, v)
			}
		})
	}
}

func TestFuzzyParseParams(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected fpParams
	}{
		{"price_short", "p:10.50", fpParams{priceMicros: 10_500_000}},
		{"price_full", "price:10.50", fpParams{priceMicros: 10_500_000}},
		{"price_and_quantity", "p:10.50 q:100", fpParams{priceMicros: 10_500_000, quantityMicros: 100 * UnitValue}},
		{"cost_and_date", "c:1 2023-07-10", fpParams{costMicros: 1 * UnitValue, valueDate: time.Date(2023, 7, 10, 0, 0, 0, 0, time.UTC)}},
		{"id", "id:DE12341234", fpParams{assetId: "DE12341234"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fp, err := fuzzyParseParams(strings.Fields(tc.input))
			if err != nil {
				t.Fatalf("Error parsing params: %s", err)
			}
			if diff := cmp.Diff(tc.expected, fp, cmp.AllowUnexported(fpParams{})); diff != "" {
				t.Errorf("fpParams mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
