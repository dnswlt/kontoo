package kontoo

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// Some valid IBANs for testing.
var (
	ibanDE100 = "DE52100"
	ibanDE999 = "DE29999"
)

func TestStoreAdd(t *testing.T) {
	tests := []struct {
		E *LedgerEntry
	}{
		{
			E: &LedgerEntry{
				Type:        AccountBalance,
				AssetRef:    ibanDE100,
				ValueDate:   DateVal(2023, 1, 1),
				ValueMicros: 1_000_000,
			},
		},
		{
			E: &LedgerEntry{
				Type:           AssetHolding,
				AssetRef:       "NESN",
				ValueDate:      DateVal(2023, 1, 2),
				QuantityMicros: 20 * UnitValue,
				PriceMicros:    5 * UnitValue,
			},
		},
		{
			E: &LedgerEntry{
				Type:           AssetSale,
				AssetRef:       "NESN",
				ValueDate:      DateVal(2023, 1, 3),
				QuantityMicros: -10 * UnitValue,
				PriceMicros:    10 * UnitValue,
			},
		},
		{
			E: &LedgerEntry{
				Type:        InterestPayment,
				AssetRef:    ibanDE100,
				ValueDate:   DateVal(2023, 1, 3),
				ValueMicros: 150 * Millis,
			},
		},
	}
	l := &Ledger{
		Assets: []*Asset{
			{
				Type:           FixedDepositAccount,
				IBAN:           ibanDE100,
				Name:           "Festgeld",
				InterestMicros: 35_000,
				Currency:       "EUR",
			},
			{
				Type:         Stock,
				Name:         "Nestle",
				TickerSymbol: "NESN",
				Currency:     "CHF",
			},
		},
	}
	s, err := NewStore(l, "")
	if err != nil {
		t.Fatalf("Failed to create Store: %v", err)
	}
	for i, tc := range tests {
		err = s.Add(tc.E)
		if err != nil {
			t.Fatalf("Failed to add entry: %v", err)
		}
		if len(l.Entries) != i+1 {
			t.Fatalf("Entry was not added, len(l.Entries) = %d", len(l.Entries))
		}
		got := l.Entries[i]
		if got != tc.E {
			t.Error("Entries are not pointer equal")
		}
		a := s.assets[tc.E.AssetID] // AssetID is set during insertion.
		if a == nil {
			t.Fatalf("Asset not found for ref %q", tc.E.AssetRef)
		}
		if got.AssetID != a.ID() {
			t.Errorf("Wrong AssetID: want %q, got %q", a.ID(), got.AssetID)
		}
		if got.Created.IsZero() {
			t.Error("Created not set")
		}
	}
}

func TestStoreUpdate(t *testing.T) {
	s, err := newTestStore([]*LedgerEntry{
		{
			ValueDate:      DateVal(2024, 1, 1),
			AssetID:        "BMW",
			Type:           AssetPurchase,
			QuantityMicros: 1 * UnitValue,
			PriceMicros:    100 * UnitValue,
		},
	}, Stock)
	if err != nil {
		t.Fatal(err)
	}
	u := &LedgerEntry{
		SequenceNum:    s.ledger.Entries[0].SequenceNum,
		ValueDate:      DateVal(2023, 1, 1),
		AssetID:        "BMW",
		Type:           AssetSale,
		QuantityMicros: -2 * UnitValue,
		PriceMicros:    200 * UnitValue,
		Currency:       "EUR",
	}
	if err := s.Update(u); err != nil {
		t.Fatal("Update error:", err)
	}
	e := s.entries["BMW"][0]
	if diff := cmp.Diff(u, e); diff != "" {
		t.Errorf("Updated entry diff (-want, +got): %s", diff)
	}
	if e.Created.IsZero() {
		t.Error("Updated entry has zero Created field")
	}
}

func TestStoreDelete(t *testing.T) {
	entries := []*LedgerEntry{
		{
			Type:        AccountCredit,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 1, 1),
			ValueMicros: 100 * UnitValue,
		},
		{
			Type:        AccountDebit,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 1, 2),
			ValueMicros: -50 * UnitValue,
		},
		{
			Type:        AccountCredit,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 1, 3),
			ValueMicros: 50 * UnitValue,
		},
	}
	s, err := newTestStore(entries, CheckingAccount)
	if err != nil {
		t.Fatalf("Failed to create Store: %v", err)
	}
	tests := []struct {
		seq       int64
		wantLen   int
		wantValue Micros
		wantErr   bool
	}{
		{seq: 100, wantErr: true},
		{seq: 2, wantLen: 2, wantValue: 150 * UnitValue},
		{seq: 1, wantLen: 1, wantValue: 50 * UnitValue},
		{seq: 3, wantLen: 0, wantValue: 0 * UnitValue},
		{seq: 3, wantErr: true},
	}
	for i, tc := range tests {
		err = s.Delete(tc.seq)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Wanted error, got none for %v", tc)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Failed to delete entry %d in round %d: %v", tc.seq, i, err)
		}
		if len(s.ledger.Entries) != tc.wantLen {
			t.Errorf("Wrong ledger length: want %d, got %d", tc.wantLen, len(s.ledger.Entries))
		}
		if len(s.entries[ibanDE100]) != tc.wantLen {
			t.Errorf("Wrong entries index length: want %d, got %d", tc.wantLen, len(s.entries[ibanDE100]))
		}
		p := s.AssetPositionAt(ibanDE100, DateVal(2023, 1, 3))
		if p == nil {
			t.Fatal("No position returned in round", i)
		}
		if p.MarketValue() != tc.wantValue {
			t.Errorf("Wrong value in round %d: want %v, got %v", i, tc.wantValue, p.MarketValue())
		}
	}
}

func TestStoreDeleteExchangeRate(t *testing.T) {
	entries := []*LedgerEntry{
		{
			Type:          ExchangeRate,
			ValueDate:     DateVal(2023, 1, 1),
			Currency:      "EUR",
			QuoteCurrency: "CHF",
			PriceMicros:   950 * Millis,
		},
		{
			Type:          ExchangeRate,
			ValueDate:     DateVal(2023, 1, 2),
			Currency:      "EUR",
			QuoteCurrency: "CHF",
			PriceMicros:   960 * Millis,
		},
	}
	s, err := newTestStore(entries, CheckingAccount)
	if err != nil {
		t.Fatalf("Failed to create Store: %v", err)
	}
	tests := []struct {
		seq     int64
		wantLen int
		wantErr bool
	}{
		{seq: 100, wantErr: true},
		{seq: 1, wantLen: 1},
		{seq: 2, wantLen: 0},
		{seq: 3, wantErr: true},
	}
	for i, tc := range tests {
		err = s.Delete(tc.seq)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Wanted error, got none for %v", tc)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Failed to delete entry %d in round %d: %v", tc.seq, i, err)
		}
		if len(s.exchangeRates["CHF"]) != tc.wantLen {
			t.Errorf("Wrong exchangeRates length: want %d, got %d", tc.wantLen, len(s.exchangeRates["CHF"]))
		}
	}
}

func TestStoreAddAsset(t *testing.T) {
	tests := []struct {
		A *Asset
	}{
		{
			A: &Asset{
				Type:         GovernmentBond,
				ISIN:         "DE123",
				Name:         "Bund123",
				Currency:     "EUR",
				IssueDate:    newDate(2023, 1, 1),
				MaturityDate: newDate(2030, 1, 1),
			},
		},
		{
			A: &Asset{
				Type:     SavingsAccount,
				IBAN:     ibanDE999,
				Name:     "Sparkonto",
				Currency: "CHF",
			},
		},
	}
	s, err := NewStore(&Ledger{}, "/test")
	if err != nil {
		t.Fatalf("Failed to create Store: %v", err)
	}
	for i, tc := range tests {
		err = s.AddAsset(tc.A)
		if err != nil {
			t.Fatalf("Failed to add entry: %v", err)
		}
		if len(s.ledger.Assets) != i+1 {
			t.Fatalf("Entry was not added, len(l.Assets) = %d", len(s.ledger.Assets))
		}
		got := s.ledger.Assets[i]
		if got != tc.A {
			t.Error("Assets are not pointer equal")
		}
		_, found := s.assets[tc.A.ID()]
		if !found {
			t.Fatalf("Asset not found for ID %q", tc.A.ID())
		}
	}
}

func TestStoreAddAssetFail(t *testing.T) {
	tests := []struct {
		name string
		A    *Asset
	}{
		{
			name: "maturity_before_issue",
			A: &Asset{
				Type:         GovernmentBond,
				ISIN:         "DE123",
				Name:         "Bund123",
				Currency:     "EUR",
				IssueDate:    newDate(2023, 1, 1),
				MaturityDate: newDate(2021, 1, 1),
			},
		},
		{
			name: "missing_name",
			A: &Asset{
				Type:     SavingsAccount,
				IBAN:     "DE999",
				Currency: "CHF",
			},
		},
		{
			name: "missing_id",
			A: &Asset{
				Type:     SavingsAccount,
				Currency: "CHF",
				Name:     "Test",
			},
		},
		{
			name: "invalid_currency",
			A: &Asset{
				Type:     SavingsAccount,
				IBAN:     "DE999",
				Name:     "Sparkonto",
				Currency: "chf",
			},
		},
	}
	s, err := NewStore(&Ledger{}, "")
	if err != nil {
		t.Fatalf("Failed to create Store: %v", err)
	}
	for _, tc := range tests {
		err = s.AddAsset(tc.A)
		if err == nil {
			t.Errorf("%s: expected error, got none", tc.name)
		}
		if len(s.ledger.Assets) != 0 {
			t.Errorf("%s: errorneous entry was added, len(l.Assets) = %d", tc.name, len(s.ledger.Assets))
		}
	}
}

func TestExchangeRatesAdd(t *testing.T) {
	dates := func(entries []*LedgerEntry) []Date {
		r := make([]Date, len(entries))
		for i, e := range entries {
			r[i] = e.ValueDate
		}
		return r
	}
	l := &Ledger{
		Header: &LedgerHeader{
			BaseCurrency: "EUR",
		},
	}
	s, _ := NewStore(l, "test")
	s.Add(&LedgerEntry{
		Type:          ExchangeRate,
		Currency:      "EUR",
		QuoteCurrency: "USD",
		PriceMicros:   1_241_000,
		ValueDate:     DateVal(2024, 1, 1),
	})
	if !cmp.Equal(dates(s.exchangeRates["USD"]), []Date{DateVal(2024, 1, 1)}) {
		t.Errorf("wrong date order: %v", dates(s.exchangeRates["USD"]))
	}
	s.Add(&LedgerEntry{
		Type:          ExchangeRate,
		Currency:      "EUR",
		QuoteCurrency: "USD",
		PriceMicros:   1_243_000,
		ValueDate:     DateVal(2024, 3, 1),
	})
	if !cmp.Equal(dates(s.exchangeRates["USD"]), []Date{DateVal(2024, 1, 1), DateVal(2024, 3, 1)}) {
		t.Errorf("wrong date order: %v", dates(s.exchangeRates["USD"]))
	}
	s.Add(&LedgerEntry{
		Type:          ExchangeRate,
		Currency:      "EUR",
		QuoteCurrency: "USD",
		PriceMicros:   1_231_200,
		ValueDate:     DateVal(2023, 12, 1),
	})
	if !cmp.Equal(dates(s.exchangeRates["USD"]), []Date{DateVal(2023, 12, 1), DateVal(2024, 1, 1), DateVal(2024, 3, 1)}) {
		t.Errorf("wrong date order: %v", dates(s.exchangeRates["USD"]))
	}
	s.Add(&LedgerEntry{
		Type:          ExchangeRate,
		Currency:      "EUR",
		QuoteCurrency: "USD",
		PriceMicros:   1_242_000,
		ValueDate:     DateVal(2024, 2, 1),
	})
	if !cmp.Equal(dates(s.exchangeRates["USD"]), []Date{DateVal(2023, 12, 1), DateVal(2024, 1, 1), DateVal(2024, 2, 1), DateVal(2024, 3, 1)}) {
		t.Errorf("wrong date order: %v", dates(s.exchangeRates["USD"]))
	}
}

func TestExchangeRatesLookup(t *testing.T) {
	tests := []struct {
		date     Date
		wantDate Date
		wantRate Micros
		wantErr  bool
	}{
		{date: DateVal(2024, 1, 1), wantDate: DateVal(2024, 1, 1), wantRate: 20240101, wantErr: false},
		{date: DateVal(2023, 12, 31), wantErr: true},
		{date: DateVal(2024, 1, 15), wantDate: DateVal(2024, 1, 1), wantRate: 20240101, wantErr: false},
		{date: DateVal(2024, 2, 1), wantDate: DateVal(2024, 2, 1), wantRate: 20240201, wantErr: false},
		{date: DateVal(2024, 2, 15), wantDate: DateVal(2024, 2, 1), wantRate: 20240201, wantErr: false},
		{date: DateVal(2024, 3, 1), wantDate: DateVal(2024, 3, 1), wantRate: 20240301, wantErr: false},
		{date: DateVal(2024, 4, 1), wantDate: DateVal(2024, 3, 1), wantRate: 20240301, wantErr: false},
	}
	l := &Ledger{
		Header: &LedgerHeader{
			BaseCurrency: "EUR",
		},
	}
	s, _ := NewStore(l, "test")
	s.Add(&LedgerEntry{
		Type:          ExchangeRate,
		Currency:      "EUR",
		QuoteCurrency: "USD",
		PriceMicros:   20240101,
		ValueDate:     DateVal(2024, 1, 1),
	})
	s.Add(&LedgerEntry{
		Type:          ExchangeRate,
		Currency:      "EUR",
		QuoteCurrency: "USD",
		PriceMicros:   20240201,
		ValueDate:     DateVal(2024, 2, 1),
	})
	s.Add(&LedgerEntry{
		Type:          ExchangeRate,
		Currency:      "EUR",
		QuoteCurrency: "USD",
		PriceMicros:   20240301,
		ValueDate:     DateVal(2024, 3, 1),
	})
	for _, tc := range tests {
		rate, date, err := s.ExchangeRateAt("USD", tc.date)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Want error, got none for %v", tc.date)
			}
			continue
		}
		if err != nil {
			t.Errorf("Cannot get exchange rate: %s", err)
			continue
		}
		if rate != tc.wantRate {
			t.Errorf("Wrong rate: %v", rate)
		}
		if !date.Equal(tc.wantDate) {
			t.Errorf("Wrong date: %v", date)
		}
	}
}

func TestPositionsAtSavingsAccount(t *testing.T) {
	l := &Ledger{
		Assets: []*Asset{
			{
				Name:     "Test",
				Type:     SavingsAccount,
				IBAN:     ibanDE100,
				Currency: "EUR",
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
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 1, 31),
			ValueMicros: 1000 * UnitValue,
		},
		{
			Type:        InterestPayment,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 2, 15),
			ValueMicros: 10 * UnitValue,
		},
		{
			Type:        AccountBalance,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 2, 28),
			ValueMicros: 2000 * UnitValue,
		},
		{
			Type:        AccountCredit,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 3, 15),
			ValueMicros: 3000 * UnitValue,
		},
		{
			Type:        AccountDebit,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 3, 31),
			ValueMicros: -4000 * UnitValue,
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
		{date: DateVal(2023, 3, 15), value: 5000 * UnitValue},
		{date: DateVal(2023, 3, 31), value: 1000 * UnitValue},
	}
	for _, p := range params {
		ps := s.AssetPositionsAt(p.date)
		if len(ps) != 1 {
			t.Fatalf("Wrong number of positions: want 1, got %d", len(ps))
		}
		gotValue := ps[0].ValueMicros
		if gotValue != p.value {
			t.Errorf("Wrong value: Want %v, got %v", p.value, gotValue)
		}
	}
}

func TestPositionAtFixedDeposit(t *testing.T) {
	l := &Ledger{
		Assets: []*Asset{
			{
				Name:         "Festgeld",
				Type:         FixedDepositAccount,
				IBAN:         ibanDE100,
				IssueDate:    newDate(2023, 1, 1),
				MaturityDate: newDate(2024, 1, 1),
				Currency:     "EUR",
			},
		},
	}
	s, err := NewStore(l, "/test")
	if err != nil {
		t.Fatal("Could not create store", err)
	}
	entries := []*LedgerEntry{
		{
			Type:        AccountCredit,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 1, 1),
			ValueMicros: 1000 * UnitValue,
		},
		{
			Type:        InterestPayment,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 6, 1),
			ValueMicros: 10 * UnitValue,
		},
		{
			Type:        AccountBalance,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 6, 1),
			ValueMicros: 1010 * UnitValue,
		},
		{
			Type:        AccountDebit,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 9, 1),
			ValueMicros: -100 * UnitValue,
		},
		{
			Type:        AssetMaturity,
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2024, 1, 1),
			ValueMicros: 920 * UnitValue,
		},
	}
	for _, e := range entries {
		err = s.Add(e)
		if err != nil {
			t.Fatal("Cannot add to ledger:", err)
		}
	}
	tests := []struct {
		date  Date
		value Micros
		items []AssetPositionItem
	}{
		{date: DateVal(2023, 1, 1), value: 1000 * UnitValue,
			items: []AssetPositionItem{
				{ValueDate: DateVal(2023, 1, 1), QuantityMicros: 1000 * UnitValue, PriceMicros: UnitValue},
			}},
		{date: DateVal(2023, 6, 1), value: 1010 * UnitValue,
			items: []AssetPositionItem{
				// AccountBalance should not have changed items:
				{ValueDate: DateVal(2023, 1, 1), QuantityMicros: 1000 * UnitValue, PriceMicros: UnitValue},
			}},
		{date: DateVal(2023, 9, 1), value: 910 * UnitValue,
			items: []AssetPositionItem{
				// AccountDebit should have reduced nominal value (stored in QuantityMicros):
				{ValueDate: DateVal(2023, 1, 1), QuantityMicros: 900 * UnitValue, PriceMicros: UnitValue},
			}},
		{date: DateVal(2024, 1, 1), value: 0},
	}
	for _, tc := range tests {
		p := s.AssetPositionAt(ibanDE100, tc.date)
		if p == nil {
			t.Fatalf("No position obtained at %v", tc.date)
		}
		if p.MarketValue() != tc.value {
			t.Errorf("Wrong value: Want %v, got %v", tc.value, p.MarketValue())
		}
		if diff := cmp.Diff(tc.items, p.Items); diff != "" {
			t.Errorf("Wrong items (-want, +got): %s", diff)
		}
	}
}

func TestProfitLossInPeriod(t *testing.T) {
	l := &Ledger{
		Assets: []*Asset{
			{
				Name:         "Microsoft Corporation",
				Type:         Stock,
				TickerSymbol: "MSFT",
				Currency:     "USD",
			},
		},
	}
	s, err := NewStore(l, "/test")
	if err != nil {
		t.Fatal("Could not create store", err)
	}
	entries := []*LedgerEntry{
		{
			Type:           AssetPurchase,
			AssetID:        "MSFT",
			ValueDate:      DateVal(2022, 6, 1),
			QuantityMicros: 1000 * UnitValue,
			PriceMicros:    270 * UnitValue,
			CostMicros:     50 * UnitValue,
		},
		{
			Type:           AssetPurchase,
			AssetID:        "MSFT",
			ValueDate:      DateVal(2023, 6, 1),
			QuantityMicros: 500 * UnitValue,
			PriceMicros:    330 * UnitValue,
			CostMicros:     50 * UnitValue,
		},
		{
			Type:        AssetPrice,
			AssetID:     "MSFT",
			ValueDate:   DateVal(2023, 12, 31),
			PriceMicros: 370 * UnitValue,
		},
		{
			Type:           AssetSale,
			AssetID:        "MSFT",
			ValueDate:      DateVal(2024, 3, 1),
			QuantityMicros: -500 * UnitValue,
			PriceMicros:    425 * UnitValue,
			CostMicros:     50 * UnitValue,
		},
		{
			Type:        AssetPrice,
			AssetID:     "MSFT",
			ValueDate:   DateVal(2024, 6, 30),
			PriceMicros: 450 * UnitValue,
		},
	}
	for _, e := range entries {
		err = s.Add(e)
		if err != nil {
			t.Fatal("Cannot add to ledger:", err)
		}
	}
	tests := []struct {
		endDate Date
		days    int
		wantPL  Micros
		wantRef Micros
	}{
		{endDate: DateVal(1999, 1, 1), days: 365, wantPL: 0, wantRef: 0},
		// P&L: (370-270) * 1000 + (370-330) * 500 == 120000 (minus 50 cost)
		// Ref: 1000*270 + 500*330 + 50 == 435050
		{endDate: DateVal(2023, 12, 31), days: 365, wantPL: 119950 * UnitValue, wantRef: 435050 * UnitValue},
		// P&L: (425-370) * 500 + 1000 * (450-370) == 107500 (minus 50 cost)
		// Ref: 1500*370 == 555000
		{endDate: DateVal(2024, 6, 30), days: 180, wantPL: 107450 * UnitValue, wantRef: 555000 * UnitValue},
	}
	for _, tc := range tests {
		pl, ref, err := s.ProfitLossInPeriod("MSFT", tc.endDate, tc.days)
		if err != nil {
			t.Fatal("ProfitLossInPeriod error:", err)
		}
		if pl != tc.wantPL {
			t.Errorf(`ProfitLossInPeriod("MSFT", %q, %v): want profit/loss %v, got %v`, tc.endDate, tc.days, tc.wantPL, pl)
		}
		if ref != tc.wantRef {
			t.Errorf(`ProfitLossInPeriod("MSFT", %q, %v): want ref val %v, got %v`, tc.endDate, tc.days, tc.wantRef, ref)
		}
	}
}

func TestProfitLossInPeriodBuySellSameYear(t *testing.T) {
	l := &Ledger{
		Assets: []*Asset{
			{
				Name:         "Microsoft Corporation",
				Type:         Stock,
				TickerSymbol: "MSFT",
				Currency:     "USD",
			},
		},
	}
	s, err := NewStore(l, "/test")
	if err != nil {
		t.Fatal("Could not create store", err)
	}
	entries := []*LedgerEntry{
		{
			// Buy 1000 in the middle of 2025...
			Type:           AssetPurchase,
			AssetID:        "MSFT",
			ValueDate:      DateVal(2025, 3, 31),
			QuantityMicros: 1000 * UnitValue,
			PriceMicros:    450 * UnitValue,
			CostMicros:     50 * UnitValue,
		},
		{
			// ... and sell them in the same year.
			Type:           AssetSale,
			AssetID:        "MSFT",
			ValueDate:      DateVal(2025, 6, 31),
			QuantityMicros: -1000 * UnitValue,
			PriceMicros:    500 * UnitValue,
			CostMicros:     50 * UnitValue,
		},
	}
	for _, e := range entries {
		err = s.Add(e)
		if err != nil {
			t.Fatal("Cannot add to ledger:", err)
		}
	}
	tests := []struct {
		endDate Date
		days    int
		wantPL  Micros
		wantRef Micros
	}{
		// Started the year with 0, bought 1000 and sold them:
		// P&L: 1000*(500-450) - 50 == 49950
		// Ref: 1000*450 + 50 == 450050
		{endDate: DateVal(2025, 12, 31), days: 365, wantPL: 49900 * UnitValue, wantRef: 450050 * UnitValue},
	}
	for _, tc := range tests {
		pl, ref, err := s.ProfitLossInPeriod("MSFT", tc.endDate, tc.days)
		if err != nil {
			t.Fatal("ProfitLossInPeriod error:", err)
		}
		if pl != tc.wantPL {
			t.Errorf(`ProfitLossInPeriod("MSFT", %q, %v): want profit/loss %v, got %v`, tc.endDate, tc.days, tc.wantPL, pl)
		}
		if ref != tc.wantRef {
			t.Errorf(`ProfitLossInPeriod("MSFT", %q, %v): want ref val %v, got %v`, tc.endDate, tc.days, tc.wantRef, ref)
		}
	}
}

func TestPositionsAtMultipleAssets(t *testing.T) {
	l := &Ledger{
		Assets: []*Asset{
			{
				Name:     "Sparkonto",
				Type:     SavingsAccount,
				IBAN:     ibanDE100,
				Currency: "EUR",
			},
			{
				Name:     "BUND",
				Type:     GovernmentBond,
				ISIN:     "DE99",
				Currency: "EUR",
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
			AssetID:     ibanDE100,
			ValueDate:   DateVal(2023, 1, 31),
			ValueMicros: 1000 * UnitValue,
		},
		{
			Type:           AssetPurchase,
			AssetID:        "DE99",
			ValueDate:      DateVal(2023, 2, 14),
			QuantityMicros: 2000 * UnitValue,
			PriceMicros:    950 * Millis,
		},
		{
			Type:           AssetSale,
			AssetID:        "DE99",
			ValueDate:      DateVal(2023, 2, 20),
			QuantityMicros: -500 * UnitValue,
			PriceMicros:    1100 * Millis,
		},
		{
			Type:        AccountBalance,
			AssetID:     ibanDE100,
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
				ibanDE100: 1000 * UnitValue,
			},
		},
		{
			name: "after_buy",
			date: DateVal(2023, 2, 14),
			wantPos: map[string]Micros{
				ibanDE100: 1000 * UnitValue,
				"DE99":    1900 * UnitValue,
			},
		},
		{
			name: "after_buy_next_day",
			date: DateVal(2023, 2, 15),
			wantPos: map[string]Micros{
				ibanDE100: 1000 * UnitValue,
				"DE99":    1900 * UnitValue,
			},
		},
		{
			name: "after_sell",
			date: DateVal(2023, 2, 20),
			wantPos: map[string]Micros{
				ibanDE100: 1000 * UnitValue,
				"DE99":    1650 * UnitValue,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := s.AssetPositionsAt(tc.date)
			if len(ps) != len(tc.wantPos) {
				t.Fatalf("Wrong number of positions: want %d, got %d", len(tc.wantPos), len(ps))
			}
			for _, gotPos := range ps {
				gotValue := gotPos.MarketValue()
				assetId := gotPos.Asset.ID()
				if gotValue != tc.wantPos[assetId] {
					t.Errorf("Wrong value for asset %s: Want %v, got %v", assetId, tc.wantPos[assetId], gotValue)
				}
			}
		})
	}
}

func TestAssetPositionUpdateStock(t *testing.T) {
	asset := &Asset{
		Type:         Stock,
		Name:         "Microsoft Corporation",
		TickerSymbol: "MSFT",
		Currency:     "USD",
	}
	const u = UnitValue
	tests := []struct {
		E *LedgerEntry
		P *AssetPosition
	}{
		{
			E: &LedgerEntry{
				Type:           AssetPurchase,
				AssetID:        "MSFT",
				ValueDate:      DateVal(2024, 1, 1),
				QuantityMicros: 10 * u,
				PriceMicros:    2 * u,
				CostMicros:     5 * u,
			},
			P: &AssetPosition{
				Asset:          asset,
				LastUpdated:    DateVal(2024, 1, 1),
				QuantityMicros: 10 * u,
				PriceMicros:    2 * u,
				PriceDate:      DateVal(2024, 1, 1),
				Items: []AssetPositionItem{
					{ValueDate: DateVal(2024, 1, 1), QuantityMicros: 10 * u, PriceMicros: 2 * u, CostMicros: 5 * u},
				},
			},
		},
		{
			E: &LedgerEntry{
				Type:           AssetPurchase,
				AssetID:        "MSFT",
				ValueDate:      DateVal(2024, 1, 2),
				QuantityMicros: 20 * u,
				PriceMicros:    3 * u,
				CostMicros:     12 * u,
			},
			P: &AssetPosition{
				Asset:          asset,
				LastUpdated:    DateVal(2024, 1, 2),
				QuantityMicros: 30 * u,
				PriceMicros:    3 * u,
				PriceDate:      DateVal(2024, 1, 2),
				Items: []AssetPositionItem{
					{ValueDate: DateVal(2024, 1, 1), QuantityMicros: 10 * u, PriceMicros: 2 * u, CostMicros: 5 * u},
					{ValueDate: DateVal(2024, 1, 2), QuantityMicros: 20 * u, PriceMicros: 3 * u, CostMicros: 12 * u},
				},
			},
		},
		{
			// Sell 20, i.e. 10 of the first purchase and 10 of the second one.
			E: &LedgerEntry{
				Type:           AssetSale,
				AssetID:        "MSFT",
				ValueDate:      DateVal(2024, 1, 3),
				QuantityMicros: -20 * u,
				PriceMicros:    3 * u,
				CostMicros:     10 * u,
			},
			P: &AssetPosition{
				Asset:          asset,
				LastUpdated:    DateVal(2024, 1, 3),
				QuantityMicros: 10 * u,
				PriceMicros:    3 * u,
				PriceDate:      DateVal(2024, 1, 3),
				Items: []AssetPositionItem{
					{ValueDate: DateVal(2024, 1, 2), QuantityMicros: 10 * u, PriceMicros: 3 * u, CostMicros: 6 * u},
				},
			},
		},
		{
			// Sell the rest.
			E: &LedgerEntry{
				Type:           AssetSale,
				AssetID:        "MSFT",
				ValueDate:      DateVal(2024, 1, 4),
				QuantityMicros: -10 * u,
				PriceMicros:    3 * u,
				CostMicros:     10 * u,
			},
			P: &AssetPosition{
				Asset:          asset,
				LastUpdated:    DateVal(2024, 1, 4),
				QuantityMicros: 0 * u,
				PriceMicros:    3 * u,
				PriceDate:      DateVal(2024, 1, 4),
				Items:          nil,
			},
		},
	}
	p := &AssetPosition{Asset: asset}
	for i, tc := range tests {
		p.Update(tc.E)
		if diff := cmp.Diff(tc.P, p); diff != "" {
			t.Errorf("Update mismatch at element %d (-want, +got): %s", i, diff)
		}
	}
}

func TestAssetPositionAssetHolding(t *testing.T) {
	asset := &Asset{
		Type:         CorporateBond,
		Name:         "Papa Joe 2030",
		TickerSymbol: "PJ30",
		Currency:     "USD",
	}
	const u = UnitValue
	tests := []struct {
		E *LedgerEntry
		P *AssetPosition
	}{
		{
			// This entry should update Items.
			E: &LedgerEntry{
				Type:           AssetHolding,
				AssetID:        "PJ30",
				ValueDate:      DateVal(2024, 1, 1),
				QuantityMicros: 1000 * u,
				PriceMicros:    1 * u,
				CostMicros:     50 * u,
			},
			P: &AssetPosition{
				Asset:          asset,
				LastUpdated:    DateVal(2024, 1, 1),
				QuantityMicros: 1000 * u,
				PriceMicros:    1 * u,
				PriceDate:      DateVal(2024, 1, 1),
				Items: []AssetPositionItem{
					{ValueDate: DateVal(2024, 1, 1), QuantityMicros: 1000 * u, PriceMicros: 1 * u, CostMicros: 50 * u},
				},
			},
		},
		{
			// This entry should not update Items, as its quantity is the same as before.
			// The price has doubled, which should be reflected in the position.
			E: &LedgerEntry{
				Type:           AssetHolding,
				AssetID:        "PJ30",
				ValueDate:      DateVal(2024, 1, 2),
				QuantityMicros: 1000 * u,
				PriceMicros:    2 * u,
			},
			P: &AssetPosition{
				Asset:          asset,
				LastUpdated:    DateVal(2024, 1, 2),
				QuantityMicros: 1000 * u,
				PriceMicros:    2 * u,
				PriceDate:      DateVal(2024, 1, 2),
				Items: []AssetPositionItem{
					{ValueDate: DateVal(2024, 1, 1), QuantityMicros: 1000 * u, PriceMicros: 1 * u, CostMicros: 50 * u},
				},
			},
		},
	}
	p := &AssetPosition{Asset: asset}
	for i, tc := range tests {
		p.Update(tc.E)
		if diff := cmp.Diff(tc.P, p); diff != "" {
			t.Errorf("Update mismatch at element %d (-want, +got): %s", i, diff)
		}
	}
}

// newTestStore is a test helper to create a store from a list of ledger entries.
// All assets will use CustomID as the ID field and have asset type t.
// The ledger's base currency is EUR.
func newTestStore(entries []*LedgerEntry, t AssetType) (*Store, error) {
	symbols := make(map[string]bool)
	for _, e := range entries {
		if e.AssetID != "" {
			symbols[e.AssetID] = true
		}
	}
	assets := make([]*Asset, len(symbols))
	i := 0
	for s := range symbols {
		assets[i] = &Asset{
			Name:     fmt.Sprintf("Test_%s", s),
			Type:     t,
			CustomID: s,
			Currency: "EUR",
		}
		i++
	}
	s, err := NewStore(&Ledger{
		Header: &LedgerHeader{BaseCurrency: "EUR"},
		Assets: assets}, "/test")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if err := s.Add(e); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func TestAssetPositionsBetween(t *testing.T) {
	entries := []*LedgerEntry{
		{
			ValueDate:      DateVal(2024, 1, 1),
			Type:           AssetPurchase,
			QuantityMicros: 1 * UnitValue,
			PriceMicros:    1 * UnitValue,
			AssetID:        "KO",
		},
	}
	s, err := newTestStore(entries, Stock)
	if err != nil {
		t.Fatal("Cannot create store:", err)
	}
	ps := s.AssetPositionsBetween("KO", DateVal(2023, 1, 1), DateVal(2025, 1, 1))
	if len(ps) != 1 {
		t.Fatalf("Wrong number of positions: %d", len(ps))
	}
	if !ps[0].LastUpdated.Equal(DateVal(2024, 1, 1)) {
		t.Errorf("Wrong date: got: %v, want: %v", ps[0].LastUpdated, DateVal(2024, 1, 1))
	}
	if ps[0].MarketValue() != 1*UnitValue {
		t.Errorf("Wrong value: got: %v, want: %v", ps[0].MarketValue(), 1*UnitValue)
	}
}

func TestAssetPositionsBetweenPast(t *testing.T) {
	// Previous entries outside the requested period need to be
	// included in the calculation of position values.
	entries := []*LedgerEntry{
		// The first two purchases should be included in the requested
		// position on 2024-02-01, the third one should not.
		{
			ValueDate:      DateVal(2024, 1, 1),
			Type:           AssetPurchase,
			QuantityMicros: 1 * UnitValue,
			PriceMicros:    1 * UnitValue,
			AssetID:        "KO",
		},
		{
			ValueDate:      DateVal(2024, 2, 1),
			Type:           AssetPurchase,
			QuantityMicros: 1 * UnitValue,
			PriceMicros:    1 * UnitValue,
			AssetID:        "KO",
		},
		{
			ValueDate:      DateVal(2024, 3, 1),
			Type:           AssetPurchase,
			QuantityMicros: 1 * UnitValue,
			PriceMicros:    1 * UnitValue,
			AssetID:        "KO",
		},
	}
	s, err := newTestStore(entries, Stock)
	if err != nil {
		t.Fatal("Cannot create store:", err)
	}
	ps := s.AssetPositionsBetween("KO", DateVal(2024, 1, 15), DateVal(2024, 2, 15))
	if len(ps) != 1 {
		t.Fatalf("Wrong number of positions: %d", len(ps))
	}
	wantDate := DateVal(2024, 2, 1)
	if !ps[0].LastUpdated.Equal(wantDate) {
		t.Errorf("Wrong date: got: %v, want: %v", ps[0].LastUpdated, wantDate)
	}
	wantValue := Micros(2 * UnitValue)
	if ps[0].MarketValue() != wantValue {
		t.Errorf("Wrong value: got: %v, want: %v", ps[0].MarketValue(), wantValue)
	}
}

func TestLoadStoreSingleRecord(t *testing.T) {
	s, err := LoadStore("./testdata/testledger.json")
	if err != nil {
		t.Fatal("Cannot load ledger:", err)
	}
	if len(s.ledger.Entries) == 0 {
		t.Error("No entries in ledger")
	}
}

func TestLoadStoreRecords(t *testing.T) {
	s, err := LoadStore("./testdata/testledger.jsons")
	if err != nil {
		t.Fatal("Cannot load ledger:", err)
	}
	if len(s.ledger.Entries) == 0 {
		t.Error("No entries in ledger")
	}
}

func TestLoadSaveStoreRecords(t *testing.T) {
	// This test is only used to format the .jsons file after changes.
	t.SkipNow()
	s, err := LoadStore("./testdata/testledger.jsons")
	if err != nil {
		t.Fatal("Cannot load ledger:", err)
	}
	if len(s.ledger.Entries) == 0 {
		t.Error("No entries in ledger")
	}
	err = s.Save()
	if err != nil {
		t.Error("Cannot save ledger:", err)
	}
}

func TestSaveLoadStore(t *testing.T) {
	ref := &Ledger{
		Entries: []*LedgerEntry{
			{
				Created:       time.Date(2023, 1, 1, 17, 0, 0, 0, time.UTC),
				SequenceNum:   1,
				Type:          ExchangeRate,
				ValueDate:     DateVal(2024, 1, 1),
				QuoteCurrency: "CHF",
				PriceMicros:   1 * UnitValue,
			},
		},
	}
	path := filepath.Join(t.TempDir(), "ledger.json")
	s, err := NewStore(ref, path)
	if err != nil {
		t.Fatalf("Could not create store: %v", err)
	}
	if len(s.ledger.Entries) != 1 {
		t.Fatalf("Unexpected number of entries in ledger: %d", len(s.ledger.Entries))
	}
	if err := s.Save(); err != nil {
		t.Fatalf("could not save store: %v", err)
	}
	s2, err := LoadStore(path)
	if err != nil {
		t.Fatalf("could not load store: %v", err)
	}
	if diff := cmp.Diff(s.ledger, s2.ledger); diff != "" {
		t.Errorf("Loaded ledger differs (-want +got):\n%s", diff)
	}
}

func TestSaveLoadEmpty(t *testing.T) {
	l := Ledger{}
	path := filepath.Join(t.TempDir(), "ledger.json")
	s, err := NewStore(&l, path)
	if err != nil {
		t.Fatalf("Could not create store: %v", err)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("could not save store: %s", err)
	}
	s2, err := LoadStore(path)
	if err != nil {
		t.Fatalf("could not load store: %v", err)
	}
	if len(s2.ledger.Entries) != 0 {
		t.Errorf("Loaded ledger is not empty, has %d entries", len(s2.ledger.Entries))
	}
	if diff := cmp.Diff(s.ledger, s2.ledger); diff != "" {
		t.Errorf("Loaded ledger differs (-want +got):\n%s", diff)
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
				SequenceNum:    1,
				ValueDate:      DateVal(2024, 12, 31),
				Type:           AssetPurchase,
				QuantityMicros: 1 * UnitValue,
				PriceMicros:    1_500_000,
				Currency:       "EUR",
			},
		},
	}
	js, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		t.Fatalf("could not marshal: %v", err)
	}
	got := string(js)
	want := `{
  "Entries": [
    {
      "Created": "2023-12-31T00:00:00Z",
      "SequenceNum": 1,
      "ValueDate": "2024-12-31",
      "Type": "AssetPurchase",
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
