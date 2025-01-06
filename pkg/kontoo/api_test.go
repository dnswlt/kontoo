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
		d    Date
		want string
	}{
		{DateVal(2024, 12, 31), "2024-12-31"},
	}
	for _, tc := range tests {
		if got := tc.d.String(); got != tc.want {
			t.Errorf("Want: %q, got: %q", tc.want, got)
		}
	}
}

func TestDateCompare(t *testing.T) {
	tests := []struct {
		d1       Date
		d2       Date
		wantSign int
	}{
		{DateVal(2024, 12, 31), DateVal(2024, 12, 31), 0},
		{DateVal(2024, 12, 31), DateVal(1999, 1, 1), 1},
		{DateVal(1999, 1, 1), DateVal(2024, 12, 31), -1},
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

func TestDateAddDays(t *testing.T) {
	tests := []struct {
		d    Date
		n    int
		want Date
	}{
		{DateVal(1999, 12, 31), 1, DateVal(2000, 1, 1)},
		// Add/subtract a year
		{DateVal(1999, 1, 1), -365, DateVal(1998, 1, 1)},
		{DateVal(2004, 3, 31), 365, DateVal(2005, 3, 31)},
		// Leap year 2004
		{DateVal(2003, 3, 31), 365, DateVal(2004, 3, 30)},
	}
	for _, tc := range tests {
		if got := tc.d.AddDays(tc.n); !got.Equal(tc.want) {
			t.Errorf("Want: %v, got: %v", tc.want, got)
		}
	}
}

func TestAssetTypeInfos(t *testing.T) {
	if len(assetTypeInfos) != len(AssetTypeValues()) {
		t.Fatal("assetTypeInfos has wrong length")
	}
	for _, v := range AssetTypeValues() {
		if v == UnspecifiedAssetType {
			continue
		}
		info := assetTypeInfos[v]
		if v != info.typ {
			t.Errorf("Wrong typ in assetTypeInfos for %v: %v", v, info.typ)
		}
		if info.displayName == "" {
			t.Errorf("Missing displayName in assetTypeInfos for %v", v)
		}
		if info.category == UnspecfiedAssetCategory {
			t.Errorf("Missing category in assetTypeInfos for %v", v)
		}
		if len(info.validEntryTypes) == 0 {
			t.Errorf("Missing validEntryTypes in assetTypeInfos for %v", v)
		}
	}
}
