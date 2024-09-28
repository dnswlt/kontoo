package kontoo

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		q               string
		wantTerms       []string
		wantFields      map[string]string
		wantNegFields   map[string]string
		wantRegexFields map[string]string
	}{
		{
			q:         "foo",
			wantTerms: []string{"foo"},
		},
		{
			q:         " foo bar ",
			wantTerms: []string{"foo", "bar"},
		},
		{
			q: "f:foo r:bar",
			wantFields: map[string]string{
				"f": "foo",
				"r": "bar",
			},
		},
		{
			q: "!f:foo !r:bar",
			wantNegFields: map[string]string{
				"f": "foo",
				"r": "bar",
			},
		},
		{
			q: "f~foo:bar jack:fruit",
			wantRegexFields: map[string]string{
				"f": "foo:bar",
			},
			wantFields: map[string]string{
				"jack": "fruit",
			},
		},
	}
	for _, tc := range tests {
		q, err := ParseQuery(tc.q)
		if err != nil {
			t.Fatalf("Cannot parse query %q: %v", tc.q, err)
		}
		if diff := cmp.Diff(tc.wantTerms, q.terms); diff != "" {
			t.Errorf("wrong terms (-want +got): %s", diff)
		}
		wantLen := len(tc.wantFields) + len(tc.wantNegFields) + len(tc.wantRegexFields)
		if wantLen != len(q.fieldTerms) {
			t.Errorf("wrong number of field terms: want %d, got: %d", wantLen, len(q.fieldTerms))
		}
		for _, ft := range q.fieldTerms {
			if ft.re != nil {
				if tc.wantRegexFields[ft.field] != ft.re.String() {
					t.Errorf("wrong regexp for field %q: want %q got %q", ft.field, tc.wantRegexFields[ft.field], ft.re.String())
				}
				continue
			}
			m := tc.wantFields
			if ft.negated {
				m = tc.wantNegFields
			}
			if m[ft.field] != ft.term {
				t.Errorf("wrong value for field %q (negated: %t): want %q got %q", ft.field, ft.negated, m[ft.field], ft.term)
			}
		}
	}
}

func TestParseQueryEmpty(t *testing.T) {
	q, err := ParseQuery("    ")
	if err != nil {
		t.Fatalf("Cannot parse query: %v", err)
	}
	if !q.Empty() {
		t.Errorf("query is not empty: %q", q.raw)
	}
}

func TestParseQueryDates(t *testing.T) {
	tests := []struct {
		q         string
		wantFrom  Date
		wantUntil Date
	}{
		{
			q:        "from:2023-12-31",
			wantFrom: DateVal(2023, 12, 31),
		},
		{
			q:         "from:2023-12-30 until:2024-01-02",
			wantFrom:  DateVal(2023, 12, 30),
			wantUntil: DateVal(2024, 1, 2),
		},
		{
			q:         "year:1999",
			wantFrom:  DateVal(1999, 1, 1),
			wantUntil: DateVal(1999, 12, 31),
		},
		{
			q:         "date:2000",
			wantFrom:  DateVal(2000, 1, 1),
			wantUntil: DateVal(2000, 12, 31),
		},
		{
			q:         "date:2020-02",
			wantFrom:  DateVal(2020, 2, 1),
			wantUntil: DateVal(2020, 2, 29),
		},
		{
			q:         "date:2020-01",
			wantFrom:  DateVal(2020, 1, 1),
			wantUntil: DateVal(2020, 1, 31),
		},
		{
			q:         "date:2019-12",
			wantFrom:  DateVal(2019, 12, 1),
			wantUntil: DateVal(2019, 12, 31),
		},
		{
			q:         "date:2024-03-31",
			wantFrom:  DateVal(2024, 3, 31),
			wantUntil: DateVal(2024, 3, 31),
		},
	}
	for _, tc := range tests {
		q, err := ParseQuery(tc.q)
		if err != nil {
			t.Fatalf("Cannot parse query %q: %v", tc.q, err)
		}
		if !tc.wantFrom.IsZero() && q.fromDate != tc.wantFrom {
			t.Errorf("wrong from date: want %q got %q", tc.wantFrom, q.fromDate)
		}
		if !tc.wantUntil.IsZero() && q.untilDate != tc.wantUntil {
			t.Errorf("wrong until date: want %q got %q", tc.wantUntil, q.untilDate)
		}
	}
}

func TestParseQuerySequenceNum(t *testing.T) {
	tests := []struct {
		q       string
		want    []int64
		wantErr bool
	}{
		{q: "num:1", want: []int64{1, 1}},
		{q: "num:1,5,9", want: []int64{1, 1, 5, 5, 9, 9}},
		{q: "num:10-100", want: []int64{10, 100}},
		{q: "num:10-100,999", want: []int64{10, 100, 999, 999}},
		{q: "num:10-100-900", wantErr: true},
		{q: "num~10", wantErr: true},
		{q: "num:10.0", wantErr: true},
		{q: "num:", wantErr: true},
	}
	for _, tc := range tests {
		q, err := ParseQuery(tc.q)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: wanted err, got none", tc.q)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Cannot parse query %q: %v", tc.q, err)
		}
		if !cmp.Equal(tc.want, q.sequenceNums) {
			t.Errorf("want sequenceNums %v, got %v", tc.want, q.sequenceNums)
		}
	}
}

func TestQueryMatch(t *testing.T) {
	r100 := &LedgerEntryRow{
		E: &LedgerEntry{
			SequenceNum: 100,
		},
	}
	rFoo := &LedgerEntryRow{
		E: &LedgerEntry{},
		A: &Asset{
			Name:    "Foo the asset",
			Comment: "bar any regret",
			ISIN:    "DE123",
		},
	}
	rDate := &LedgerEntryRow{
		E: &LedgerEntry{
			ValueDate: DateVal(2024, 1, 1),
		},
	}
	rType := &LedgerEntryRow{
		E: &LedgerEntry{
			Type: AssetPurchase,
		},
		A: &Asset{
			Type: Stock,
		},
	}
	tests := []struct {
		q    string
		e    *LedgerEntryRow
		want bool
	}{
		{q: "num:1", e: r100, want: false},
		{q: "num:100", e: r100, want: true},
		{q: "num:1-50,60-100", e: r100, want: true},
		{q: "num:1-99,101-110", e: r100, want: false},

		{q: "foo", e: rFoo, want: true},
		{q: "name:foo", e: rFoo, want: true},
		{q: "foo bar DE12", e: rFoo, want: true},
		{q: "foo bar DE124", e: rFoo, want: false},

		{q: "date:2024-01-01", e: rDate, want: true},
		{q: "date:2024-01", e: rDate, want: true},
		{q: "date:2024", e: rDate, want: true},
		{q: "date:2024-01-02", e: rDate, want: false},
		{q: "from:2023-12-01 until:2024-01-31", e: rDate, want: true},

		{q: "type:purch", e: rType, want: true},
		{q: "class:stock", e: rType, want: true},
		{q: "class:bond", e: rType, want: false},
		{q: "class:equi", e: rType, want: false},
	}
	for _, tc := range tests {
		q, err := ParseQuery(tc.q)
		if err != nil {
			t.Fatalf("Cannot parse query %q: %v", tc.q, err)
		}
		if got := q.Match(tc.e); got != tc.want {
			t.Errorf("wrong match result for %q: want %v, got %v", tc.q, tc.want, got)
		}
	}
}

func TestStringFields(t *testing.T) {
	fs := strings.Fields("   affee  banjo  ")
	if len(fs) != 2 {
		t.Errorf("invalid number of fields: %d", len(fs))
	}
	fs = strings.Fields("   \t    ")
	if len(fs) != 0 {
		t.Errorf("invalid number of fields: %d", len(fs))
	}
}
