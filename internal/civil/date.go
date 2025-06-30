package civil

import (
	"strings"
	"time"
)

// Dates without a time component.
type Date struct {
	time.Time
}

func (d Date) String() string {
	return d.Format("2006-01-02")
}

func (d Date) Compare(other Date) int {
	return d.Time.Compare(other.Time)
}

func (d Date) Between(start, end Date) bool {
	return !start.After(d.Time) && !end.Before(d.Time)
}

func (d Date) AddDays(n int) Date {
	return d.AddDate(0, 0, n)
}

func (d Date) AddDate(years, months, days int) Date {
	return Date{d.Time.AddDate(years, months, days)}
}

func DateVal(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}
func NewDate(year int, month time.Month, day int) *Date {
	return &Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}
func Today() Date {
	return ToDate(time.Now())
}
func ToDate(t time.Time) Date {
	y, m, d := t.Date()
	return DateVal(y, m, d)
}

func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, err
	}
	return Date{t}, nil
}

func (d *Date) UnmarshalJSON(data []byte) error {
	var err error
	s := strings.Trim(string(data), "\"")
	*d, err = ParseDate(s)
	if err != nil {
		return err
	}
	return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte("\"" + d.Format("2006-01-02") + "\""), nil
}

func (d Date) Equal(e Date) bool {
	return d.Time.Equal(e.Time)
}
