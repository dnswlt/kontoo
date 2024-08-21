package kontoo

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type fieldTerm struct {
	field   string
	negated bool
	term    string
	re      *regexp.Regexp
}

type Query struct {
	raw        string
	terms      []string
	fieldTerms []fieldTerm
	fromDate   Date
	untilDate  Date
}

func (q *Query) Empty() bool {
	return q.raw == ""
}

// ParseQuery parses rawQuery as a query expression.
// A query expression consists of whitespace-separated query terms.
// A query term can be one of:
// foo    ==> Any of the "main" fields must contain the substring "foo".
// f:foo  ==> Field f must exist and contain the substring "foo"
// f~foo  ==> Field f must regexp-match foo (which can be a full regexp, e.g. "bar|baz")
// Negations of the above: !f:foo, !f~foo
//
// Supported field names are dependent on the entity being matched (ledger entry, asset position, ...).
// Well-known field names are: {id, type, name}
//
// Date-related filters are special:
// year:2024
// from:2024-03-07
// until:2024-12-31  (inclusive)
func ParseQuery(rawQuery string) (*Query, error) {
	rawQuery = strings.TrimSpace(rawQuery)
	fts := strings.Fields(strings.ToLower(rawQuery))
	q := &Query{
		raw:       rawQuery,
		untilDate: DateVal(9999, 12, 31),
	}
	for _, ft := range fts {
		sep := strings.IndexAny(ft, ":~")
		if sep == -1 {
			q.terms = append(q.terms, ft)
			continue
		}
		start, neg := 0, false
		if ft[0] == '!' {
			start, neg = 1, true
		}
		f := ft[start:sep]
		t := ft[sep+1:]
		if len(t) == 0 {
			return nil, fmt.Errorf("no term specified for field %q", ft)
		}
		// Special case: dates
		if f == "year" {
			y, err := strconv.Atoi(t)
			if err != nil {
				return nil, fmt.Errorf("invalid year: %q", t)
			}
			q.fromDate = DateVal(y, 1, 1)
			q.untilDate = DateVal(y, 12, 31)
			continue
		} else if f == "from" {
			from, err := time.Parse("2006-01-02", t)
			if err != nil {
				return nil, fmt.Errorf("invalid from: %q", t)
			}
			q.fromDate = Date{from}
			continue
		} else if f == "until" {
			until, err := time.Parse("2006-01-02", t)
			if err != nil {
				return nil, fmt.Errorf("invalid until: %q", t)
			}
			q.untilDate = Date{until}
			continue
		}
		r := fieldTerm{
			field:   f,
			negated: neg,
		}
		if ft[sep] == '~' {
			var err error
			r.re, err = regexp.Compile(t)
			if err != nil {
				return nil, fmt.Errorf("invalid regexp for field %q: %v", f, err)
			}
		} else {
			r.term = t
		}
		q.fieldTerms = append(q.fieldTerms, r)
	}
	return q, nil
}

// Returns true if s to lower case contains t, which is expected to be in lower case.
func matchLower(s, t string) bool {
	return strings.Contains(strings.ToLower(s), t)
}

func matchAsset(t string, a *Asset) bool {
	return matchLower(a.AccountNumber, t) ||
		matchLower(a.IBAN, t) || matchLower(a.ISIN, t) ||
		matchLower(a.Name, t) || matchLower(a.ShortName, t) ||
		matchLower(a.TickerSymbol, t) || matchLower(a.WKN, t) ||
		matchLower(a.CustomID, t) || matchLower(a.Comment, t)
}

func (q *Query) Match(e *LedgerEntryRow) bool {
	if q.Empty() {
		return true
	}
	for _, t := range q.terms {
		if matchLower(e.Label(), t) || matchLower(e.Comment(), t) {
			continue
		}
		if e.A != nil && matchAsset(t, e.A) {
			continue
		}
		return false
	}
	for i := range q.fieldTerms {
		t := &q.fieldTerms[i]
		expectMatch := !t.negated
		fval := ""
		switch t.field {
		case "id":
			if e.HasAsset() {
				fval = e.AssetID()
			}
		case "name":
			fval = e.Label()
		case "type":
			fval = e.EntryType()
		case "class":
			fval = e.AssetType()
		}
		if fval == "" {
			//  Match fails for unsupported (and empty) fields
			return false
		}
		if t.re != nil {
			if t.re.MatchString(strings.ToLower(fval)) != expectMatch {
				return false
			}
		} else {
			if matchLower(fval, t.term) != expectMatch {
				return false
			}
		}
	}
	return true
}
