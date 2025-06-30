package ledger

import (
	"fmt"
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
		if got := ValidIBAN(tc.iban); got != tc.want {
			t.Errorf("validIBAN(%q) == %v, want %v", tc.iban, got, tc.want)
		}
	}
}

func TestFindValidIBAN(t *testing.T) {
	for i := 0; i < 100; i++ {
		// CH18 0024 0240 3760 6600 Q
		iban := fmt.Sprintf("CH%02d 0024 0240 3760 6600 Q", i)
		valid := ValidIBAN(iban)
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
		v = v && ValidIBAN(iban)
	}
	if !v {
		b.Fail()
	}
}
