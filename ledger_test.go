package kontoo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestStoreAdd(t *testing.T) {
	l := &Ledger{
		Assets: []*Asset{
			{
				Type:           GovernmentBond,
				ISIN:           "DE123123123",
				ShortName:      "BUND.123",
				InterestMicros: 35_000,
			},
		},
	}
	e := &LedgerEntry{
		Type:        AccountBalance,
		AssetRef:    "BUND.123",
		ValueDate:   DateVal(2023, 1, 30),
		ValueMicros: 1_000_000,
	}
	s, err := NewStore(l, "")
	if err != nil {
		t.Fatalf("Failed to create Store: %v", err)
	}
	err = s.Add(e)
	if err != nil {
		t.Fatalf("Failed to add entry: %v", err)
	}
	if len(l.Entries) != 1 {
		t.Fatalf("Entry was not added, len(l.Entries) = %d", len(l.Entries))
	}
	got := l.Entries[0]
	if got != e {
		t.Error("Entries are not pointer equal")
	}
	if got.AssetID != "DE123123123" {
		t.Errorf("AssetID not set: %q", got.AssetID)
	}
	if got.Created.IsZero() {
		t.Error("Created not set")
	}
}

func TestSaveLoad(t *testing.T) {
	ref := &Ledger{
		Entries: []*LedgerEntry{
			{
				Created:     time.Date(2023, 1, 1, 17, 0, 0, 0, time.UTC),
				ValueMicros: 1_000_000,
			},
		},
	}
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := ref.Save(path); err != nil {
		t.Fatalf("could not save ledger: %v", err)
	}
	l := &Ledger{}
	err := l.Load(path)
	if err != nil {
		t.Fatalf("could not load ledger: %v", err)
	}
	if diff := cmp.Diff(ref, l); diff != "" {
		t.Errorf("Loaded ledger differs (-want +got):\n%s", diff)
	}
}

func TestSaveLoadEmpty(t *testing.T) {
	ref := Ledger{}
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := ref.Save(path); err != nil {
		t.Fatalf("could not save ledger: %s", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("could not load ledger file: %s", err)
	}
	if string(data) != "{}" {
		t.Errorf("expected empty list, got %s", data)
	}
}

func TestMicrosMarshalJSON(t *testing.T) {
	tests := []struct {
		input Micros
		want  string
	}{
		{input: 1_000_000, want: `"1"`},
		{input: 20_500_000, want: `"20.50"`},
		{input: 2_000_001, want: `"2.000001"`},
		{input: -300_000, want: `"-0.30"`},
		{input: -100_300_001, want: `"-100.300001"`},
	}
	for _, tc := range tests {
		data, err := tc.input.MarshalJSON()
		if err != nil {
			t.Fatalf("failed to MarshalJSON: %v", err)
		}
		got := string(data)
		if got != tc.want {
			t.Errorf("got: %s, want: %s", got, tc.want)
		}
	}
}

func TestMarshalLedger(t *testing.T) {
	l := Ledger{
		Entries: []*LedgerEntry{
			{
				Created:            time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC),
				ValueDate:          DateVal(2024, 12, 31),
				Type:               BuyTransaction,
				NominalValueMicros: 1_500_000,
				Currency:           "EUR",
			},
		},
	}
	js, err := l.Marshal()
	if err != nil {
		t.Fatalf("could not marshal: %v", err)
	}
	got := string(js)
	want := `{
  "Entries": [
    {
      "Created": "2023-12-31T00:00:00Z",
      "SequenceNum": 0,
      "ValueDate": "2024-12-31",
      "Type": "BuyTransaction",
      "Currency": "EUR",
      "NominalValue": "1.50"
    }
  ]
}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMarshalDate(t *testing.T) {
	tests := []struct {
		input Date
		want  string
	}{
		{input: DateVal(2024, 10, 13), want: `"2024-10-13"`},
		{input: Date{}, want: `"0001-01-01"`},
	}
	for _, tc := range tests {
		data, err := tc.input.MarshalJSON()
		if err != nil {
			t.Fatalf("failed to MarshalJSON: %v", err)
		}
		got := string(data)
		if got != tc.want {
			t.Errorf("got: %s, want: %s", got, tc.want)
		}
	}
}
