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

func TestPositionsAtSingleAsset(t *testing.T) {
	l := &Ledger{
		Assets: []*Asset{
			{
				Type: SavingsAccount,
				IBAN: "DE12",
			},
		},
	}
	s, err := NewStore(l, "/test")
	if err != nil {
		t.Fatal("Could not create store", err)
	}
	entries := []*LedgerEntry{
		{
			Type:        AccountBalance,
			AssetID:     "DE12",
			ValueDate:   DateVal(2023, 1, 31),
			ValueMicros: 1000 * UnitValue,
		},
		{
			Type:        InterestPayment,
			AssetID:     "DE12",
			ValueDate:   DateVal(2023, 2, 15),
			ValueMicros: 10 * UnitValue,
		},
		{
			Type:        AccountBalance,
			AssetID:     "DE12",
			ValueDate:   DateVal(2023, 2, 28),
			ValueMicros: 2000 * UnitValue,
		},
	}
	for _, e := range entries {
		err = s.Add(e)
		if err != nil {
			t.Fatal("Cannot add to ledger:", err)
		}
	}
	params := []struct {
		date  Date
		value Micros
	}{
		{date: DateVal(2023, 1, 31), value: 1000 * UnitValue},
		{date: DateVal(2023, 2, 16), value: 1000 * UnitValue},
		{date: DateVal(2023, 2, 28), value: 2000 * UnitValue},
	}
	for _, p := range params {
		ps := s.AssetPositionsAt(p.date.Time)
		if len(ps) != 1 {
			t.Fatalf("Wrong number of positions: want 1, got %d", len(ps))
		}
		gotValue := ps[0].ValueMicros
		if gotValue != p.value {
			t.Errorf("Wrong value: Want %v, got %v", p.value, gotValue)
		}
	}
}

func TestPositionsAtMultipleAssets(t *testing.T) {
	l := &Ledger{
		Assets: []*Asset{
			{
				Type: SavingsAccount,
				IBAN: "DE12",
			},
			{
				Type: GovernmentBond,
				IBAN: "DE99",
			},
		},
	}
	s, err := NewStore(l, "/test")
	if err != nil {
		t.Fatal("Could not create store", err)
	}
	entries := []*LedgerEntry{
		{
			Type:        AccountBalance,
			AssetID:     "DE12",
			ValueDate:   DateVal(2023, 1, 31),
			ValueMicros: 1000 * UnitValue,
		},
		{
			Type:           BuyTransaction,
			AssetID:        "DE99",
			ValueDate:      DateVal(2023, 2, 14),
			QuantityMicros: 2000 * UnitValue,
			PriceMicros:    950 * Millis,
		},
		{
			Type:           SellTransaction,
			AssetID:        "DE99",
			ValueDate:      DateVal(2023, 2, 20),
			QuantityMicros: 500 * UnitValue,
			PriceMicros:    1100 * Millis,
		},
		{
			Type:        AccountBalance,
			AssetID:     "DE12",
			ValueDate:   DateVal(2023, 2, 28),
			ValueMicros: 2000 * UnitValue,
		},
	}
	for _, e := range entries {
		err = s.Add(e)
		if err != nil {
			t.Fatal("Cannot add to ledger:", err)
		}
	}
	tests := []struct {
		name    string
		date    Date
		wantPos map[string]Micros
	}{
		{
			name: "first_entry",
			date: DateVal(2023, 1, 31),
			wantPos: map[string]Micros{
				"DE12": 1000 * UnitValue,
			},
		},
		{
			name: "after_buy",
			date: DateVal(2023, 2, 14),
			wantPos: map[string]Micros{
				"DE12": 1000 * UnitValue,
				"DE99": 1900 * UnitValue,
			},
		},
		{
			name: "after_buy_next_day",
			date: DateVal(2023, 2, 15),
			wantPos: map[string]Micros{
				"DE12": 1000 * UnitValue,
				"DE99": 1900 * UnitValue,
			},
		},
		{
			name: "after_sell",
			date: DateVal(2023, 2, 20),
			wantPos: map[string]Micros{
				"DE12": 1000 * UnitValue,
				"DE99": 1650 * UnitValue,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := s.AssetPositionsAt(tc.date.Time)
			if len(ps) != len(tc.wantPos) {
				t.Fatalf("Wrong number of positions: want %d, got %d", len(tc.wantPos), len(ps))
			}
			for _, gotPos := range ps {
				gotValue := gotPos.CalculatedValueMicros()
				assetId := gotPos.Asset.ID()
				if gotValue != tc.wantPos[assetId] {
					t.Errorf("Wrong value for asset %s: Want %v, got %v", assetId, tc.wantPos[assetId], gotValue)
				}
			}
		})
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
				Created:        time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC),
				ValueDate:      DateVal(2024, 12, 31),
				Type:           BuyTransaction,
				QuantityMicros: 1 * UnitValue,
				PriceMicros:    1_500_000,
				Currency:       "EUR",
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
      "Quantity": "1",
      "Price": "1.50"
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
