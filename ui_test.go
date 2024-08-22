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
