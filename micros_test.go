package kontoo

import (
	"math"
	"testing"
)

func TestMicrosMul(t *testing.T) {
	tests := []struct {
		a    Micros
		b    Micros
		want Micros
	}{
		{1 * UnitValue, 1, 1},
		{2 * UnitValue, 3 * UnitValue, 6 * UnitValue},
		{2_000, 3_000, 6},
		{math.MaxInt64, 1 * UnitValue, math.MaxInt64},
		{math.MaxInt64, 1 * Millis, math.MaxInt64 / 1000},
		{1000 * UnitValue, 3 * Millis, 3 * UnitValue},
		{-1000 * UnitValue, 3 * Millis, -3 * UnitValue},
		{-1000 * UnitValue, -3 * Millis, 3 * UnitValue},
		{1000 * UnitValue, -3 * Millis, -3 * UnitValue},
	}
	for _, tc := range tests {
		got := tc.a.Mul(tc.b)
		if got != tc.want {
			t.Errorf("%v * %v: got %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMicrosSplitFrac(t *testing.T) {
	tests := []struct {
		a        Micros
		wantInt  int64
		wantFrac int
	}{
		{0, 0, 0},
		{-0, 0, 0},
		{1, 0, 1},
		{-1, 0, -1},
		{1 * UnitValue, 1, 0},
		{-1 * UnitValue, -1, 0},
		{math.MaxInt64, 9223372036854, 775807},
		{math.MinInt64, -9223372036854, -775808},
		{1_100_000, 1, 100_000},
		{30*UnitValue + 999*Millis, 30, 999000},
		{-(30*UnitValue + 999*Millis), -30, -999000},
	}
	for _, tc := range tests {
		gotInt, gotFrac := tc.a.SplitFrac()
		if gotInt != tc.wantInt || gotFrac != tc.wantFrac {
			t.Errorf("%v.SplitFrac(): want (%v, %v), got (%v, %v)", tc.a, tc.wantInt, tc.wantFrac, gotInt, gotFrac)
		}
	}
}

func TestMicrosFormat(t *testing.T) {
	tests := []struct {
		m      Micros
		format string
		want   string
	}{
		// Whole numbers
		{1 * UnitValue, "", "1"},
		{1 * UnitValue, ".0", "1"},
		{1 * UnitValue, ".1", "1.0"},
		{1 * UnitValue, ".2", "1.00"},
		{1 * UnitValue, ".6", "1.000000"},
		{2000 * UnitValue, "'.", "2'000"},
		{2000 * UnitValue, "-'.", "2'000"},
		{-2000 * UnitValue, "-'.", "-2'000"},
		{-2000 * UnitValue, "()'.", "(2'000)"},
		// Large numbers and multiple thousand seps
		{2_100_001 * UnitValue, ",.", "2,100,001"},
		{999_999_999 * UnitValue, ",.", "999,999,999"},
		// 0, 1, -1
		{0, "()'.", "0"},
		{0, ".3", "0.000"},
		{0, "", "0"},
		{1, ".6", "0.000001"},
		{-1, ".6", "-0.000001"},
		{-1, ".", "-0.000001"},
		// "auto" formatting for decimal places
		{123450, ".", "0.12345"},
		{123400, ".", "0.1234"},
		{100000, ".", "0.1"},
		// Percent
		{1 * UnitValue, ".1%", "100.0%"},
		{123 * Millis, ".1%", "12.3%"},
		{123 * Millis, ".2%", "12.30%"},
		{123 * Millis, ".%", "12.3%"},
		{123 * Millis, "%", "12.3%"},
		{-2 * UnitValue, "()%", "(200%)"},
		{2 * UnitValue, "()%", "200%"},
		{2_000 * UnitValue, ",%", "200,000%"},
		// Other
		{-3 * Millis, ".3", "-0.003"},
		{-123999, ".6", "-0.123999"},
		{-123999, "().3", "(0.123)"},
		{12_456 * Millis, "().1", "12.4"},
		{-12_456 * Millis, "().1", "(12.4)"},
	}
	for _, tc := range tests {
		got := tc.m.Format(tc.format)
		if got != tc.want {
			t.Errorf("%v.Format(%q): want %q, got %q", tc.m, tc.format, tc.want, got)
		}
	}
}
