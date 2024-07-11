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
