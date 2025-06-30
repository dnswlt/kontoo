package civil

import (
	"strings"
	"time"
)

// Dates without a time component.
// Internally represented as a time.Time with hours, minutes, seconds, etc.
// set to zero in time zone UTC.
type Date struct {
	t time.Time
}

func (d Date) String() string {
	return d.t.Format("2006-01-02")
}

// In returns a time.Time on the date that d represents at 00:00:00 in the given time zone.
func (d Date) In(loc *time.Location) time.Time {
	year, month, day := d.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

func (d Date) Compare(other Date) int {
	return d.t.Compare(other.t)
}

func (d Date) Before(other Date) bool {
	return d.t.Before(other.t)
}

func (d Date) After(other Date) bool {
	return d.t.After(other.t)
}

func (d Date) Date() (year int, month time.Month, day int) {
	year, month, day = d.t.Date()
	return
}

// DaysSince returns the number of full days between d and other.
// It is positive if d is after other, negative if d is before other.
func (d Date) DaysSince(other Date) int {
	const day = 24 * time.Hour
	diff := d.t.Sub(other.t)
	return int(diff / day)
}

func (d Date) Year() int {
	return d.t.Year()
}

func (d Date) Month() time.Month {
	return d.t.Month()
}

func (d Date) Day() int {
	return d.t.Day()
}

func (d Date) IsZero() bool {
	return d.t.IsZero()
}

func (d Date) Between(start, end Date) bool {
	return !start.t.After(d.t) && !end.t.Before(d.t)
}

func (d Date) AddDays(n int) Date {
	return d.AddDate(0, 0, n)
}

func (d Date) AddDate(years, months, days int) Date {
	return Date{t: d.t.AddDate(years, months, days)}
}

func (d Date) UnixMilli() int64 {
	return d.t.UnixMilli()
}

func DateVal(year int, month time.Month, day int) Date {
	return Date{t: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}
func NewDate(year int, month time.Month, day int) *Date {
	return &Date{t: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
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
	return Date{t: t}, nil
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
	return []byte("\"" + d.t.Format("2006-01-02") + "\""), nil
}

func (d Date) Equal(e Date) bool {
	return d.t.Equal(e.t)
}
