package kontoo

import (
	"bytes"
	"encoding/gob"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestExample(t *testing.T) {
	// Create and serialize example Log.
	example := Log{
		Created:   time.Now(),
		ValueDate: time.Date(2023, time.January, 17, 0, 40, 0, 0, time.UTC),
		Type:      AssetValue,
		Asset: Asset{
			Id:   "IE00B4L5Y983",
			Type: Stock,
			Name: "iShares Core MSCI World UCITS ETF USD (Acc)",
		},
		Currency:       "EUR",
		ValueMicros:    1000 * UnitValue,
		QuantityMicros: 40 * UnitValue,
		PriceMicros:    25 * UnitValue,
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(&example); err != nil {
		t.Errorf("could not encode Log: %s", err)
	}
	// Decode and check it's the same value.
	dec := gob.NewDecoder(&buf)
	var log Log
	if err := dec.Decode(&log); err != nil {
		t.Errorf("could not decode Log: %s", err)
	}
	if diff := cmp.Diff(example, log); diff != "" {
		t.Errorf("Log mismatch (-want +got):\n%s", diff)
	}
}
