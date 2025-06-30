package calc

import (
	"math"
	"strings"
	"testing"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/units"
)

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
	asset := &ledger.Asset{
		ISIN:            "DE12",
		Type:            ledger.GovernmentBond,
		MaturityDate:    civil.NewDate(2022, 1, 1),
		Currency:        "EUR",
		InterestMicros:  40 * units.Millis, // 4%
		InterestPayment: ledger.AnnualPayment,
	}
	p := &ledger.AssetPosition{
		Asset: asset,
		Items: []ledger.AssetPositionItem{
			{
				ValueDate:      civil.DateVal(2020, 1, 1),
				QuantityMicros: 10000 * units.One,
				PriceMicros:    950 * units.Millis,
			},
			{
				ValueDate:      civil.DateVal(2021, 1, 1),
				QuantityMicros: 10000 * units.One,
				PriceMicros:    975 * units.Millis,
			},
		},
	}
	got := InternalRateOfReturn(p)
	want := units.Micros(66344) // Verified using Excel's XIRR() function.
	if got != want {
		t.Errorf("Wrong IRR: want %v, got %v", want, got)
	}
}

func TestInternalRateOfReturnVaryingIntervals(t *testing.T) {
	asset := &ledger.Asset{
		ISIN:            "DE12",
		Type:            ledger.GovernmentBond,
		MaturityDate:    civil.NewDate(2022, 1, 1),
		Currency:        "EUR",
		InterestMicros:  30 * units.Millis, // 3%
		InterestPayment: ledger.AnnualPayment,
	}
	p := &ledger.AssetPosition{
		Asset: asset,
		Items: []ledger.AssetPositionItem{
			{
				ValueDate:      civil.DateVal(2020, 1, 1),
				QuantityMicros: 10000 * units.One,
				PriceMicros:    950 * units.Millis,
			},
			{
				ValueDate:      civil.DateVal(2020, 6, 1),
				QuantityMicros: 10000 * units.One,
				PriceMicros:    975 * units.Millis,
			},
			{
				ValueDate:      civil.DateVal(2021, 3, 1),
				QuantityMicros: 15000 * units.One,
				PriceMicros:    925 * units.Millis,
			},
		},
	}
	got := InternalRateOfReturn(p)
	want := units.Micros(70750) // Verified using Excel's XIRR() function.
	if got != want {
		t.Errorf("Wrong IRR: want %v, got %v", want, got)
	}
}

func TestInternalRateOfReturnSingle(t *testing.T) {
	asset := &ledger.Asset{
		ISIN:            "DE12",
		Type:            ledger.GovernmentBond,
		MaturityDate:    civil.NewDate(2023, 1, 1),
		Currency:        "EUR",
		InterestMicros:  40 * units.Millis, // 4%
		InterestPayment: ledger.AnnualPayment,
	}
	p := &ledger.AssetPosition{
		Asset: asset,
		Items: []ledger.AssetPositionItem{
			{
				ValueDate:      civil.DateVal(2020, 1, 1),
				QuantityMicros: 10000 * units.One,
				PriceMicros:    1000 * units.Millis,
			},
		},
	}
	got := InternalRateOfReturn(p)
	want := units.Micros(38497)
	if got != want {
		t.Errorf("Wrong IRR: want %v, got %v", want, got)
	}
}

func TestXIRR(t *testing.T) {
	tests := []struct {
		values []units.Micros
		dates  []civil.Date
		want   units.Micros
	}{
		{
			// Pay 100%, get back 100% ==> 0% IRR
			values: []units.Micros{-10000 * units.One, 10000 * units.One},
			dates:  []civil.Date{civil.DateVal(2024, 1, 6), civil.DateVal(2026, 1, 6)},
			want:   0,
		},
		{
			// Pay 80%, get back 100% ==> 4.5614% IRR
			values: []units.Micros{-8000 * units.One, 10000 * units.One},
			dates:  []civil.Date{civil.DateVal(2025, 1, 6), civil.DateVal(2030, 1, 6)},
			want:   45614,
		},
		{
			// Pay 90%, get 2% interest each year, get back 100% ==> 4.2606% IRR
			values: []units.Micros{
				-9000 * units.One,
				200 * units.One,
				200 * units.One,
				200 * units.One,
				200 * units.One,
				200 * units.One,
				10000 * units.One,
			},
			dates: []civil.Date{
				civil.DateVal(2025, 1, 6),
				civil.DateVal(2026, 1, 6),
				civil.DateVal(2027, 1, 6),
				civil.DateVal(2028, 1, 6),
				civil.DateVal(2029, 1, 6),
				civil.DateVal(2030, 1, 6),
				civil.DateVal(2030, 1, 6),
			},
			want: 42606,
		},
	}
	for _, tc := range tests {
		irr, err := XIRR(tc.values, tc.dates)
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
		values  []units.Micros
		dates   []civil.Date
		wantErr string
	}{
		{
			values:  []units.Micros{-1, 2, 3},
			dates:   []civil.Date{civil.DateVal(2024, 1, 6), civil.DateVal(2024, 1, 6)},
			wantErr: "different size",
		},
		{
			values:  []units.Micros{-1},
			dates:   []civil.Date{civil.DateVal(2024, 1, 6)},
			wantErr: "few values",
		},
		{
			values:  nil,
			dates:   nil,
			wantErr: "few values",
		},
		{
			values:  []units.Micros{-1, 2},
			dates:   []civil.Date{civil.DateVal(2030, 1, 6), civil.DateVal(2024, 1, 6)},
			wantErr: "sorted",
		},
	}
	for _, tc := range tests {
		irr, err := XIRR(tc.values, tc.dates)
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
		p    IRRParams
		want units.Micros
	}{
		{
			p: IRRParams{
				NominalValue:    1000 * units.One,
				Price:           units.One,
				InterestRate:    30 * units.Millis,
				PurchaseDate:    civil.DateVal(2025, 1, 6),
				MaturityDate:    civil.DateVal(2029, 1, 6),
				InterestPayment: ledger.AnnualPayment,
			},
			want: 29980,
		},
		{
			p: IRRParams{
				NominalValue:    1000 * units.One,
				Price:           units.One,
				InterestRate:    30 * units.Millis,
				PurchaseDate:    civil.DateVal(2025, 1, 6),
				MaturityDate:    civil.DateVal(2026, 7, 6),
				InterestPayment: ledger.AnnualPayment,
			},
			want: 30075,
		},
	}
	for _, tc := range tests {
		irr, err := IRRWithInterest(tc.p)
		if err != nil {
			t.Fatal("irrWithInterest failed:", err)
		}
		if irr != tc.want {
			t.Errorf("irrWithInterest wrong result: want %v, got %v", tc.want, irr)
		}
	}
}
