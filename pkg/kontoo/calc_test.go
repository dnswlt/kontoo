package kontoo

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestValidIBAN(t *testing.T) {
	tests := []struct {
		iban string
		want bool
	}{
		{"CH4804835167777581000", true},
		{"AE640260001015182581201", true},
		{"CH180024024037606600Q", true},        // Letter in the account number
		{"CH48 0483 5167 7775 8100 0", true},   // Allow whitespace
		{" CH48 0483 5167 7775 8100 0", false}, // Don't allow leading whitespace
		{"CH48 0483 5167 7775 8100 0 ", false}, // Don't allow trailing whitespace
		{"CH4704835167777581000", false},       // checksum is off by one
		{"ch4804835167777581000", false},       // Don't allow lower case ISO code
		{"ÄÖ4804835167777581000", false},       // No umlauts
		{"CH480-4835-1677-7758-1000", false},   // No hyphens
		{"CH48", false},                        // Too short
		{"CH", false},                          // Too short
		{"", false},                            // Empty
	}
	for _, tc := range tests {
		if got := validIBAN(tc.iban); got != tc.want {
			t.Errorf("validIBAN(%q) == %v, want %v", tc.iban, got, tc.want)
		}
	}
}

func TestFindValidIBAN(t *testing.T) {
	for i := 0; i < 100; i++ {
		// CH18 0024 0240 3760 6600 Q
		iban := fmt.Sprintf("CH%02d 0024 0240 3760 6600 Q", i)
		valid := validIBAN(iban)
		if valid != (i == 18) {
			t.Fatalf("Unexpected validation result: %v for i=%d", valid, i)
		}
	}
}

func BenchmarkValidIBAN(b *testing.B) {
	b.Skip("Disabled benchmark")
	iban := "CH18 0024 0240 3760 6600 Q"
	v := true
	for n := 0; n < b.N; n++ {
		v = v && validIBAN(iban)
	}
	if !v {
		b.Fail()
	}
}

func TestBisect(t *testing.T) {
	tests := []struct {
		y       float64
		low     float64
		high    float64
		f       func(x float64) float64
		x       float64
		wantErr string
	}{
		{y: 100, low: 0, high: 20, f: func(x float64) float64 { return x }, x: 100},
		{y: 100, low: 0, high: 20, f: func(x float64) float64 { return x * x * x }, x: math.Pow(100, 1/3.0)},
		{y: math.Sqrt(2), low: 1, high: 3, f: func(x float64) float64 { return math.Sqrt(x) }, x: 2},
		{y: 10, low: 0, high: 10, f: func(x float64) float64 { return math.Exp(x / 2) }, x: math.Log(100)},
		{y: 100, low: -1e10, high: -1e10 + 0.001, f: func(x float64) float64 { return x }, x: 100, wantErr: "converge"},
		{y: 100, low: -1e10, high: -1e10 - 0.001, f: func(x float64) float64 { return x }, x: 100, wantErr: "less than high"},
	}
	for _, tc := range tests {
		x, err := bisect(tc.y, tc.low, tc.high, tc.f)
		if tc.wantErr != "" {
			if err == nil {
				t.Fatalf("Wanted error, got result %.6f", x)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Wanted error containing %q, got: %v", tc.wantErr, err)
			}
			continue
		}
		if err != nil {
			t.Fatal("bisect failed:", err)
		}
		if math.Abs(x-tc.x) > 1e-6 {
			t.Errorf("Wrong result: want %.6f, got %.6f", tc.x, x)
		}
	}
}

func TestNewton(t *testing.T) {
	tests := []struct {
		y       float64
		x0      float64
		f       func(x float64) (float64, float64)
		x       float64
		wantErr string
	}{
		{y: 100, x0: 0, f: func(x float64) (float64, float64) { return x, 1 }, x: 100},
		{y: 0, x0: 1, f: func(x float64) (float64, float64) { return x*x - 10, 2 * x }, x: math.Sqrt(10)},
		{y: 10, x0: 1, f: func(x float64) (float64, float64) { return math.Exp(x / 2), math.Exp(x/2) / 2 }, x: math.Log(100)},
	}
	for _, tc := range tests {
		x, err := newton(tc.y, tc.x0, tc.f)
		if tc.wantErr != "" {
			if err == nil {
				t.Fatalf("Wanted error, got result %.6f", x)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Wanted error containing %q, got: %v", tc.wantErr, err)
			}
			continue
		}
		if err != nil {
			t.Fatal("newton failed:", err)
		}
		if math.Abs(x-tc.x) > 1e-6 {
			t.Errorf("Wrong result: want %.6f, got %.6f", tc.x, x)
		}
	}

}

func TestInternalRateOfReturn(t *testing.T) {
	asset := &Asset{
		ISIN:            "DE12",
		Type:            GovernmentBond,
		MaturityDate:    newDate(2022, 1, 1),
		Currency:        "EUR",
		InterestMicros:  40 * Millis, // 4%
		InterestPayment: AnnualPayment,
	}
	p := &AssetPosition{
		Asset: asset,
		Items: []AssetPositionItem{
			{
				ValueDate:      DateVal(2020, 1, 1),
				QuantityMicros: 10000 * UnitValue,
				PriceMicros:    950 * Millis,
			},
			{
				ValueDate:      DateVal(2021, 1, 1),
				QuantityMicros: 10000 * UnitValue,
				PriceMicros:    975 * Millis,
			},
		},
	}
	got := internalRateOfReturn(p)
	want := Micros(66344) // Verified using Excel's XIRR() function.
	if got != want {
		t.Errorf("Wrong IRR: want %v, got %v", want, got)
	}
}

func TestInternalRateOfReturnVaryingIntervals(t *testing.T) {
	asset := &Asset{
		ISIN:            "DE12",
		Type:            GovernmentBond,
		MaturityDate:    newDate(2022, 1, 1),
		Currency:        "EUR",
		InterestMicros:  30 * Millis, // 3%
		InterestPayment: AnnualPayment,
	}
	p := &AssetPosition{
		Asset: asset,
		Items: []AssetPositionItem{
			{
				ValueDate:      DateVal(2020, 1, 1),
				QuantityMicros: 10000 * UnitValue,
				PriceMicros:    950 * Millis,
			},
			{
				ValueDate:      DateVal(2020, 6, 1),
				QuantityMicros: 10000 * UnitValue,
				PriceMicros:    975 * Millis,
			},
			{
				ValueDate:      DateVal(2021, 3, 1),
				QuantityMicros: 15000 * UnitValue,
				PriceMicros:    925 * Millis,
			},
		},
	}
	got := internalRateOfReturn(p)
	want := Micros(70750) // Verified using Excel's XIRR() function.
	if got != want {
		t.Errorf("Wrong IRR: want %v, got %v", want, got)
	}
}

func TestInternalRateOfReturnSingle(t *testing.T) {
	asset := &Asset{
		ISIN:            "DE12",
		Type:            GovernmentBond,
		MaturityDate:    newDate(2023, 1, 1),
		Currency:        "EUR",
		InterestMicros:  40 * Millis, // 4%
		InterestPayment: AnnualPayment,
	}
	p := &AssetPosition{
		Asset: asset,
		Items: []AssetPositionItem{
			{
				ValueDate:      DateVal(2020, 1, 1),
				QuantityMicros: 10000 * UnitValue,
				PriceMicros:    1000 * Millis,
			},
		},
	}
	got := internalRateOfReturn(p)
	want := Micros(38497)
	if got != want {
		t.Errorf("Wrong IRR: want %v, got %v", want, got)
	}
}

func TestXIRR(t *testing.T) {
	tests := []struct {
		values []Micros
		dates  []Date
		want   Micros
	}{
		{
			// Pay 100%, get back 100% ==> 0% IRR
			values: []Micros{-10000 * UnitValue, 10000 * UnitValue},
			dates:  []Date{DateVal(2024, 1, 6), DateVal(2026, 1, 6)},
			want:   0,
		},
		{
			// Pay 80%, get back 100% ==> 4.5614% IRR
			values: []Micros{-8000 * UnitValue, 10000 * UnitValue},
			dates:  []Date{DateVal(2025, 1, 6), DateVal(2030, 1, 6)},
			want:   45614,
		},
		{
			// Pay 90%, get 2% interest each year, get back 100% ==> 4.2606% IRR
			values: []Micros{
				-9000 * UnitValue,
				200 * UnitValue,
				200 * UnitValue,
				200 * UnitValue,
				200 * UnitValue,
				200 * UnitValue,
				10000 * UnitValue,
			},
			dates: []Date{
				DateVal(2025, 1, 6),
				DateVal(2026, 1, 6),
				DateVal(2027, 1, 6),
				DateVal(2028, 1, 6),
				DateVal(2029, 1, 6),
				DateVal(2030, 1, 6),
				DateVal(2030, 1, 6),
			},
			want: 42606,
		},
	}
	for _, tc := range tests {
		irr, err := xIRR(tc.values, tc.dates)
		if err != nil {
			t.Fatal("xIRR failed:", err)
		}
		if irr != tc.want {
			t.Errorf("xIRR wrong result: want %v, got %v", tc.want, irr)
		}
	}
}

func TestXIRRError(t *testing.T) {
	tests := []struct {
		values  []Micros
		dates   []Date
		wantErr string
	}{
		{
			values:  []Micros{-1, 2, 3},
			dates:   []Date{DateVal(2024, 1, 6), DateVal(2024, 1, 6)},
			wantErr: "different size",
		},
		{
			values:  []Micros{-1},
			dates:   []Date{DateVal(2024, 1, 6)},
			wantErr: "few values",
		},
		{
			values:  nil,
			dates:   nil,
			wantErr: "few values",
		},
		{
			values:  []Micros{-1, 2},
			dates:   []Date{DateVal(2030, 1, 6), DateVal(2024, 1, 6)},
			wantErr: "sorted",
		},
	}
	for _, tc := range tests {
		irr, err := xIRR(tc.values, tc.dates)
		if err == nil {
			t.Fatalf("Wanted error, got result %v", irr)
		}
		if !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("Wanted error containing %q, got: %v", tc.wantErr, err)
		}
	}
}

func TestIRRWithInterest(t *testing.T) {
	tests := []struct {
		p    irrParams
		want Micros
	}{
		{
			p: irrParams{
				nominalValue:    1000 * UnitValue,
				price:           UnitValue,
				interestRate:    30 * Millis,
				purchaseDate:    DateVal(2025, 1, 6),
				maturityDate:    DateVal(2029, 1, 6),
				interestPayment: AnnualPayment,
			},
			want: 29980,
		},
		{
			p: irrParams{
				nominalValue:    1000 * UnitValue,
				price:           UnitValue,
				interestRate:    30 * Millis,
				purchaseDate:    DateVal(2025, 1, 6),
				maturityDate:    DateVal(2026, 7, 6),
				interestPayment: AnnualPayment,
			},
			want: 30075,
		},
	}
	for _, tc := range tests {
		irr, err := irrWithInterest(tc.p)
		if err != nil {
			t.Fatal("irrWithInterest failed:", err)
		}
		if irr != tc.want {
			t.Errorf("irrWithInterest wrong result: want %v, got %v", tc.want, irr)
		}
	}
}
