package ui

import (
	"fmt"
	"html/template"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/units"
)

type DropdownOptions struct {
	Selected *NamedOption
	Options  []NamedOption
}
type NamedOption struct {
	Name  string
	Value any
	Data  map[string]any
}

func YearOptions(url url.URL, date civil.Date, minDate, maxDate civil.Date) DropdownOptions {
	res := DropdownOptions{
		Selected: &NamedOption{
			Name:  fmt.Sprintf("%d", date.Year()),
			Value: date.Year(),
		},
	}
	minYear, maxYear := date.Year(), date.Year()
	if !minDate.IsZero() {
		minYear = min(minYear, minDate.Year())
	}
	if !maxDate.IsZero() {
		maxYear = max(maxYear, maxDate.Year())
	}
	monthEnd := date.Equal(civil.DateVal(date.Year(), date.Month(), 1).AddDate(0, 1, -1))
	for y := maxYear; y >= minYear; y-- {
		lastDay := civil.DateVal(y, date.Month(), 1).AddDate(0, 1, -1)
		day := min(date.Day(), lastDay.Day())
		if monthEnd {
			day = lastDay.Day()
		}
		d := civil.DateVal(y, date.Month(), day)
		q := url.Query()
		q.Set("date", d.String())
		url.RawQuery = q.Encode()
		res.Options = append(res.Options, NamedOption{
			Name:  fmt.Sprintf("%d", y),
			Value: y,
			Data: map[string]any{
				"URL": url.String(),
			},
		})
	}
	return res
}

func MonthOptions(url url.URL, date civil.Date) DropdownOptions {
	months := []string{
		"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
	}
	res := DropdownOptions{
		Selected: &NamedOption{
			Name:  months[date.Month()-1],
			Value: int(date.Month()),
		},
	}
	for i := 1; i <= 12; i++ {
		// Reports are snapshots at the last day of the selected month.
		d := civil.DateVal(date.Year(), time.Month(i), 1).AddDate(0, 1, -1)
		q := url.Query()
		q.Set("date", d.String())
		url.RawQuery = q.Encode()
		res.Options = append(res.Options, NamedOption{
			Name:  months[i-1],
			Value: i,
			Data: map[string]any{
				"URL": url.String(),
			},
		})
	}
	return res
}

func joinAny(items any, sep string) (string, error) {
	if bs, ok := items.([]string); ok {
		// Fast path: join strings
		return strings.Join(bs, sep), nil
	}
	val := reflect.ValueOf(items)
	if val.Kind() != reflect.Slice {
		return "", fmt.Errorf("first argument to join must be a slice, got %v", val.Type())
	}
	elems := make([]string, val.Len())
	for i := 0; i < val.Len(); i++ {
		elems[i] = fmt.Sprintf("%v", val.Index(i))
	}
	return strings.Join(elems, sep), nil
}

// Parses the period from a request as a duration.
// Valid values are "Max", "YTD", and "<N><Unit>", where <Unit> must be one of
// "D", "W", "M", "Y".
func ParsePeriod(end civil.Date, p string) (civil.Date, error) {
	if p == "" {
		return civil.Date{}, fmt.Errorf("empty period given")
	}
	if p == "Max" {
		return civil.Date{}, nil
	}
	if p == "YTD" {
		return civil.DateVal(end.Year(), 1, 1), nil
	}
	n, err := strconv.Atoi(p[:len(p)-1])
	if err != nil {
		return civil.Date{}, fmt.Errorf("invalid number in period %s", p)
	}
	switch p[len(p)-1] {
	case 'D':
		return end.AddDate(0, 0, -n), nil
	case 'W':
		return end.AddDate(0, 0, -7*n), nil
	case 'M':
		return end.AddDate(0, -n, 0), nil
	case 'Y':
		return end.AddDate(-n, 0, 0), nil
	default:
		return civil.Date{}, fmt.Errorf("invalid period: %s", p)
	}
}

func CommonFuncs() template.FuncMap {
	return template.FuncMap{
		"concat": func(s, t string) string {
			return s + t
		},
		"nonzero": func(m units.Micros) bool {
			return m != 0
		},
		"negative": func(m units.Micros) bool {
			return m < 0
		},
		"micros": func(m units.Micros) int64 {
			return int64(m)
		},
		"money": func(m units.Micros) string {
			return m.Format("()'.2")
		},
		"price": func(m units.Micros) string {
			return m.Format("'.3")
		},
		"quantity": func(m units.Micros) string {
			if _, f := m.SplitFrac(); f != 0 {
				return m.Format("'.2")
			}
			return m.Format("'.0")
		},
		// Percent in accounting contexts (with brackets for negative values).
		"percentAcc": func(m units.Micros) string {
			return m.Format("()'.2%")
		},
		"percent": func(m units.Micros) string {
			return m.Format(".2%")
		},
		"yyyymmdd": func(t any) (string, error) {
			switch d := t.(type) {
			case time.Time:
				return d.Format("2006-01-02"), nil
			case civil.Date:
				return d.String(), nil
			}
			return "", fmt.Errorf("yyyymmdd called with invalid type %t", t)
		},
		"ymdhm": func(t time.Time) string {
			return t.Format("2006-01-02 15:04")
		},
		"isodate": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05-07:00")
		},
		"assetType": func(t ledger.AssetType) string {
			return t.DisplayName()
		},
		"assetCategory": func(t ledger.AssetType) string {
			return t.Category().DisplayName()
		},
		"setp": func(rawURL, param, value string) (string, error) {
			u, err := url.Parse(rawURL)
			if err != nil {
				return "", err
			}
			q := u.Query()
			q.Set(param, value)
			u.RawQuery = q.Encode()
			return u.String(), nil
		},
		"setpvar": func(rawURL, pathParam, value string) (string, error) {
			u, err := url.Parse(rawURL)
			if err != nil {
				return "", err
			}
			u.Path = strings.ReplaceAll(u.Path, "{"+pathParam+"}", url.PathEscape(value))
			return u.String(), nil
		},
		"join": joinAny,
	}
}
