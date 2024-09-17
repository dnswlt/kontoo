package kontoo

import (
	"fmt"
	"html/template"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
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

func yearOptions(url url.URL, date Date, minDate, maxDate Date) DropdownOptions {
	res := DropdownOptions{
		Selected: &NamedOption{
			Name:  fmt.Sprintf("%d", date.Year()),
			Value: date.Year(),
		},
	}
	for y := maxDate.Year(); y >= minDate.Year(); y-- {
		d := DateVal(y, date.Month(), date.Day())
		q := url.Query()
		q.Set("date", d.Format("2006-01-02"))
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

func monthOptions(url url.URL, date Date, maxDate Date) DropdownOptions {
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
	if date.Year() > maxDate.Year() {
		return res // No options if no data
	}
	maxMonth := 12
	if maxDate.Year() == date.Year() {
		maxMonth = int(maxDate.Month())
	}
	for i := 0; i < maxMonth; i++ {
		d := DateVal(date.Year(), time.Month(i+1), 1).AddDate(0, 1, -1)
		q := url.Query()
		q.Set("date", d.Format("2006-01-02"))
		url.RawQuery = q.Encode()
		res.Options = append(res.Options, NamedOption{
			Name:  months[i],
			Value: i + 1,
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
func parsePeriod(end Date, p string) (Date, error) {
	if p == "" {
		return Date{}, fmt.Errorf("empty period given")
	}
	if p == "Max" {
		return Date{}, nil
	}
	if p == "YTD" {
		return DateVal(end.Year(), 1, 1), nil
	}
	n, err := strconv.Atoi(p[:len(p)-1])
	if err != nil {
		return Date{}, fmt.Errorf("invalid number in period %s", p)
	}
	switch p[len(p)-1] {
	case 'D':
		return Date{end.AddDate(0, 0, -n)}, nil
	case 'W':
		return Date{end.AddDate(0, 0, -7*n)}, nil
	case 'M':
		return Date{end.AddDate(0, -n, 0)}, nil
	case 'Y':
		return Date{end.AddDate(-n, 0, 0)}, nil
	default:
		return Date{}, fmt.Errorf("invalid period: %s", p)
	}
}

func commonFuncs() template.FuncMap {
	return template.FuncMap{
		"concat": func(s, t string) string {
			return s + t
		},
		"nonzero": func(m Micros) bool {
			return m != 0
		},
		"negative": func(m Micros) bool {
			return m < 0
		},
		"money": func(m Micros) string {
			return m.Format("()'.2")
		},
		"price": func(m Micros) string {
			return m.Format("'.3")
		},
		"quantity": func(m Micros) string {
			if _, f := m.SplitFrac(); f != 0 {
				return m.Format("'.2")
			}
			return m.Format("'.0")
		},
		// Percent in accounting contexts (with brackets for negative values).
		"percentAcc": func(m Micros) string {
			return m.Format("()'.2%")
		},
		"percent": func(m Micros) string {
			return m.Format(".2%")
		},
		"yyyymmdd": func(t any) (string, error) {
			switch d := t.(type) {
			case time.Time:
				return d.Format("2006-01-02"), nil
			case Date:
				return d.Time.Format("2006-01-02"), nil
			}
			return "", fmt.Errorf("yyyymmdd called with invalid type %t", t)
		},
		"ymdhm": func(t time.Time) string {
			return t.Format("2006-01-02 15:04")
		},
		"assetType": func(t AssetType) string {
			return t.DisplayName()
		},
		"assetCategory": func(t AssetType) string {
			return t.Category().String()
		},
		"days": func(d time.Duration) int {
			return int(math.Round(d.Seconds() / 60 / 60 / 24))
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
