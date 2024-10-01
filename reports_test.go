package kontoo

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestQuarterlyPeriods(t *testing.T) {
	tests := []struct {
		end  Date
		n    int
		want []*ReportingPeriod
	}{
		{
			end:  DateVal(2024, 10, 3),
			n:    0,
			want: nil,
		},
		{
			end: DateVal(2024, 1, 1),
			n:   1,
			want: []*ReportingPeriod{
				{
					Start: DateVal(2024, 1, 1),
					End:   DateVal(2024, 1, 1),
					Label: "24Q1",
				},
			},
		},
		{
			end: DateVal(2024, 10, 3),
			n:   2,
			want: []*ReportingPeriod{
				{
					Start: DateVal(2024, 7, 1),
					End:   DateVal(2024, 9, 30),
					Label: "24Q3",
				},
				{
					Start: DateVal(2024, 10, 1),
					End:   DateVal(2024, 10, 3),
					Label: "24Q4",
				},
			},
		},
		{
			end: DateVal(2024, 9, 30),
			n:   4,
			want: []*ReportingPeriod{
				{
					Start: DateVal(2023, 10, 1),
					End:   DateVal(2023, 12, 31),
					Label: "23Q4",
				},
				{
					Start: DateVal(2024, 1, 1),
					End:   DateVal(2024, 3, 31),
					Label: "24Q1",
				},
				{
					Start: DateVal(2024, 4, 1),
					End:   DateVal(2024, 6, 30),
					Label: "24Q2",
				},
				{
					Start: DateVal(2024, 7, 1),
					End:   DateVal(2024, 9, 30),
					Label: "24Q3",
				},
			},
		},
	}
	for _, tc := range tests {
		ps := quarterlyPeriods(tc.end, tc.n)
		if diff := cmp.Diff(tc.want, ps); diff != "" {
			t.Errorf("Periods differ: (-want +got): %s", diff)
		}
	}
}

func TestReportEnsureLength(t *testing.T) {
	// Calling ensureLength should enlarge all data fields.
	var data ReportingPeriodData
	data.ensureLength(10)

	value := reflect.ValueOf(data)
	typ := reflect.TypeOf(data)
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		structField := typ.Field(i)
		// Check that all fields of type []Micros have the expected length
		if field.Kind() == reflect.Slice && field.Type().Elem() == reflect.TypeOf(Micros(0)) {
			if field.Len() != 10 {
				// If this fails, you probably forgot to include a new field in the ensureLength code.
				t.Errorf("Unexpected length for field %s after ensureLength(10): %d", structField.Name, field.Len())
			}
		}
	}
}
