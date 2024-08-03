package kontoo

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestAssetTypeJsonEnum(t *testing.T) {
	// AssetType is an int enum, but should be (de-)serialized as a string.
	s := `{
		"ISIN":   "IE00B4L5Y983",
		"Type": "Stock",
		"Name": "iShares Core MSCI World UCITS ETF USD (Acc)"
	 }`
	var got Asset
	if err := json.Unmarshal([]byte(s), &got); err != nil {
		t.Fatalf("unmarshal failed: %s", err)
	}
	want := Asset{
		ISIN: "IE00B4L5Y983",
		Type: Stock,
		Name: "iShares Core MSCI World UCITS ETF USD (Acc)",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("asset diff (-want +got):\n%s", diff)
	}
}

func TestDateString(t *testing.T) {
	tests := []struct {
		d    *Date
		want string
	}{
		{nil, ""},
		{NewDate(2024, 12, 31), "2024-12-31"},
	}
	for _, tc := range tests {
		if got := tc.d.String(); got != tc.want {
			t.Errorf("Want: %q, got: %q", tc.want, got)
		}
	}
}

func TestDateCompare(t *testing.T) {
	tests := []struct {
		d1       *Date
		d2       *Date
		wantSign int
	}{
		{NewDate(2024, 12, 31), NewDate(2024, 12, 31), 0},
		{NewDate(2024, 12, 31), NewDate(1999, 1, 1), 1},
		{NewDate(1999, 1, 1), NewDate(2024, 12, 31), -1},
		{NewDate(2024, 12, 31), nil, 1},
		{nil, NewDate(2024, 12, 31), -1},
		{nil, nil, 0},
	}
	sgn := func(i int) int {
		if i < 0 {
			return -1
		}
		if i > 0 {
			return 1
		}
		return 0
	}
	for _, tc := range tests {

		if got := tc.d1.Compare(tc.d2); sgn(got) != tc.wantSign {
			t.Errorf("Want: %q, got: %q", tc.wantSign, got)
		}
	}
}
