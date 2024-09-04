package kontoo

import (
	"testing"
	"time"
)

func TestJoinAny(t *testing.T) {
	tests := []struct {
		input   any
		sep     string
		want    string
		wantErr bool
	}{
		{[]string{"foo", "bar"}, " ", "foo bar", false},
		{[]string{}, ":", "", false},
		{[]int{1, 2, 3}, ", ", "1, 2, 3", false},
		{[]bool{true, false}, ":", "true:false", false},
		{[]struct{}{}, " ", "", false},
		{[]EntryType{AssetPurchase, AssetSale}, " ", "AssetPurchase AssetSale", false},
		{[]EntryType{AssetPurchase, AssetSale}, " ", "AssetPurchase AssetSale", false},
		{[]Date{DateVal(2000, 1, 1)}, " ", "2000-01-01", false},
		{1.3, "", "", true},
		{time.Now(), "", "", true},
	}
	for _, tc := range tests {
		got, err := joinAny(tc.input, tc.sep)
		if err != nil && !tc.wantErr {
			t.Fatal(err)
		}
		if err == nil && tc.wantErr {
			t.Fatal("Wanted error, got none")
		}
		if got != tc.want {
			t.Errorf("Want: %q, got: %q", tc.want, got)
		}
	}
}

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		end    Date
		period string
		want   Date
	}{
		{DateVal(2024, 1, 1), "1M", DateVal(2023, 12, 1)},
		{DateVal(2024, 2, 29), "1Y", DateVal(2023, 3, 1)},
		{DateVal(1999, 12, 1), "5D", DateVal(1999, 11, 26)},
		{DateVal(2024, 1, 1), "Max", Date{}},
		{DateVal(2023, 7, 31), "YTD", DateVal(2023, 1, 1)},
		{DateVal(2023, 1, 1), "YTD", DateVal(2023, 1, 1)},
	}
	for _, tc := range tests {
		got, err := parsePeriod(tc.end, tc.period)
		if err != nil {
			t.Fatal("Cannot parse period:", err)
		}
		if !got.Equal(tc.want) {
			t.Errorf("Want: %v, got: %v", tc.want, got)
		}
	}
}
