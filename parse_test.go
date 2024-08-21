package kontoo

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseArgs(t *testing.T) {
	want := CommandArgs{
		Args: []string{"quz"},
		KeywordArgs: map[string][]string{
			"foo": {"bar", "baz bak"},
		},
	}
	got, err := ParseArgs([]string{
		"quz", "--foo", "bar", "baz bak",
	})
	if err != nil {
		t.Fatalf("could not parse args: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("-want +got: %s", diff)
	}
}

func TestParseDecimalAsMicros(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Micros
	}{
		{"zero_int", "0", 0},
		{"neg_zero_int", "-0", 0},
		{"zero_dec", "0.0", 0},
		{"zero_frac", ".0", 0},
		{"zero_idot", "0.", 0},
		{"one_int", "1", 1_000_000},
		{"ont_intfrac", "1.1", 1_100_000},
		{"leading_zero", "002.5", 2_500_000},
		{"negative", "-2", -2_000_000},
		{"negative_frac", "-2.5", -2_500_000},
		{"negative_frac_0", "-0.5", -500_000},
		{"negative_frac_dot", "-.5", -500_000},
		{"neg_leading_zero", "-002.5", -2_500_000},
		{"frac_trailing_zero", "2.100", 2_100_000},
		{"frac_leading_zero_3", "2.001", 2_001_000},
		{"frac_leading_zero_6", "2.000001", 2_000_001},
		{"nines", "999999.999999", 999_999_999_999},
		{"only_frac", ".123456", 123456},
		{"frac_1", "9.1", 9_100_000},
		{"frac_2", "9.12", 9_120_000},
		{"frac_3", "9.123", 9_123_000},
		{"frac_4", "9.1234", 9_123_400},
		{"frac_5", "9.12345", 9_123_450},
		{"frac_6", "9.123456", 9_123_456},
		{"min_pos", "0.000001", 1},
		{"min_neg", "-0.000001", -1},
		{"max_value", "9223372036853.999999", 9223372036853999999},
		{"min_value", "-9223372036853.999999", -9223372036853999999},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var v Micros
			err := ParseDecimalAsMicros(tc.input, &v)
			if err != nil {
				t.Fatalf("Error parsing decimal: %s", err)
			}
			if v != tc.expected {
				t.Errorf("Want: %d, got: %d", tc.expected, v)
			}
		})
	}
}

func TestParseDecimalAsMicrosErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"letters", "abc"},
		{"leading_space", " 123"},
		{"middle_space", "1 23"},
		{"trailing_space", "123 "},
		{"no_digits", "-."},
		{"only_minus", "-"},
		{"only_dot", "."},
		{"frac_too_long", "0.1234567"},
		{"overflow", "9223372036854"},
		{"underflow", "-9223372036854"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var v Micros
			err := ParseDecimalAsMicros(tc.input, &v)
			if err == nil {
				t.Fatal("Expected error, got none")
			}
		})
	}
}
func BenchmarkParseDecimalAsMicros(b *testing.B) {
	inputs := []string{
		"17.000", "-1000", "10000.50", "0.035",
	}
	for n := 0; n < b.N; n++ {
		var m Micros
		var err error
		for _, input := range inputs {
			if err = ParseDecimalAsMicros(input, &m); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func TestParseLedgerEntry(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *LedgerEntry
	}{
		{
			name:  "currency",
			input: "assetpurchase foo -Currency EUR",
			want: &LedgerEntry{
				Type:     AssetPurchase,
				AssetRef: "foo",
				Currency: EUR,
			},
		},
		{
			name:  "micros",
			input: "assetpurchase foo -Value 120.95",
			want: &LedgerEntry{
				Type:        AssetPurchase,
				AssetRef:    "foo",
				ValueMicros: 120_950_000,
			},
		},
		{
			name:  "comment",
			input: "assetpurchase foo -Comment this is  comment",
			want: &LedgerEntry{
				Type:     AssetPurchase,
				AssetRef: "foo",
				Comment:  "this is comment",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseLedgerEntry(strings.Split(tc.input, " "))
			if err != nil {
				t.Fatalf("failed to parse LedgerEntry: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ParseLedgerEntry(%q) diff: (-want, +got): %s", tc.input, diff)
			}
		})
	}
}

func TestSubseq(t *testing.T) {
	tests := []struct {
		s    string
		t    string
		want bool
	}{
		{"foo", "", true},
		{"", "", true},
		{"foo", "bar", false},
		{"foo", "foo", true},
		{"foo", "foos", false},
		{"123f4o5o", "foo", true},
		{"_jack_", "jacki", false},
	}
	for _, tc := range tests {
		t.Run(tc.s+"-"+tc.t, func(t *testing.T) {
			got := subseq(tc.s, tc.t)
			if got != tc.want {
				t.Errorf("Want: %t, got: %t", tc.want, got)
			}
		})
	}
}
