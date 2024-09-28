package kontoo

import (
	"fmt"
	"regexp"
	"slices"
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
	raw          string
	terms        []string
	fieldTerms   []fieldTerm
	sequenceNums []int64 // 2-pairs of inclusive ranges of valid sequence numbers. empty means "all numbers".
	fromDate     Date
	untilDate    Date
	descending   bool // whether to return results in ascending (default) or descending order
	// Maximum number of entries to return per "group" (typically: asset)
	maxPerGroup int
}

func (q *Query) Empty() bool {
	return q.raw == ""
}

var (
	// Pre-defined query terms that can be referenced through
	// variable names (e.g., $main).
	queryVariables = map[string]string{
		"main": "!type~price|rate",
	}
)

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
// date:2024 or date:2024-03 or date:2024-03-07
// year:2024
// from:2024-03-07
// until:2024-12-31  (inclusive)
//
// SequenceNum for ledger entries is also special:
// num:3 or num:10-100 or num:10-20,80-90,100  (all inclusive)
func ParseQuery(rawQuery string) (*Query, error) {
	rawQuery = strings.TrimSpace(rawQuery)
	fts := strings.Fields(strings.ToLower(rawQuery))
	q := &Query{
		raw:       rawQuery,
		untilDate: DateVal(9999, 12, 31),
	}
	for _, ft := range fts {
		if ft[0] == '$' {
			if repl, ok := queryVariables[ft[1:]]; ok {
				ft = repl
			} else {
				return nil, fmt.Errorf("undefined variable: %q", ft)
			}
		}
		sep := strings.IndexAny(ft, ":~")
		if sep == -1 {
			// No field given => generic search term across standard fields
			q.terms = append(q.terms, ft)
			continue
		}
		// Field term
		start, neg := 0, false
		if ft[0] == '!' {
			start, neg = 1, true
		}
		f := ft[start:sep]
		t := ft[sep+1:]
		if len(t) == 0 {
			return nil, fmt.Errorf("no term specified for field %q", ft)
		}
		if f == "order" {
			// Ordering
			if ft[sep] != ':' {
				return nil, fmt.Errorf("only operator : is allowed for %q filter", f)
			}
			switch t {
			case "asc":
				q.descending = false
			case "desc":
				q.descending = true
			default:
				return nil, fmt.Errorf(`invalid ordering %q (must be "asc" or "desc")`, t)
			}
		} else if f == "max" {
			if ft[sep] != ':' {
				return nil, fmt.Errorf("only operator : is allowed for %q filter", f)
			}
			// Limits
			n, err := strconv.Atoi(t)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid argument for max: %q", t)
			}
			q.maxPerGroup = n
		} else if f == "num" {
			if ft[sep] != ':' {
				return nil, fmt.Errorf("only operator : is allowed for %q filter", f)
			}
			rs := strings.Split(t, ",")
			for _, r := range rs {
				if i := strings.Index(r, "-"); i >= 0 {
					n1, err := strconv.ParseInt(r[:i], 10, 64)
					if err != nil {
						return nil, fmt.Errorf("invalid range for num: %q", r)
					}
					n2, err := strconv.ParseInt(r[i+1:], 10, 64)
					if err != nil {
						return nil, fmt.Errorf("invalid range for num: %q", r)
					}
					q.sequenceNums = append(q.sequenceNums, n1, n2)
				} else {
					// No "-" => single number
					n, err := strconv.ParseInt(r, 10, 64)
					if err != nil {
						return nil, fmt.Errorf("invalid number for num: %q", r)
					}
					q.sequenceNums = append(q.sequenceNums, n, n)
				}
			}
		} else if f == "date" || f == "year" || f == "from" || f == "until" {
			// Dates
			if ft[sep] != ':' {
				return nil, fmt.Errorf("only operator : is allowed for %q filter", f)
			}
			switch f {
			case "date":
				if ymd, err := time.Parse("2006-01-02", t); err == nil {
					q.fromDate = Date{ymd}
					q.untilDate = Date{ymd}
				} else if ym, err := time.Parse("2006-01", t); err == nil {
					q.fromDate = Date{ym}
					q.untilDate = Date{ym.AddDate(0, 1, -1)}
				} else if y, err := strconv.Atoi(t); err == nil {
					q.fromDate = DateVal(y, 1, 1)
					q.untilDate = DateVal(y, 12, 31)
				} else {
					return nil, fmt.Errorf("invalid date: %q", t)
				}
			case "year":
				y, err := strconv.Atoi(t)
				if err != nil {
					return nil, fmt.Errorf("invalid year: %q", t)
				}
				q.fromDate = DateVal(y, 1, 1)
				q.untilDate = DateVal(y, 12, 31)
			case "from":
				from, err := time.Parse("2006-01-02", t)
				if err != nil {
					return nil, fmt.Errorf("invalid from: %q", t)
				}
				q.fromDate = Date{from}
			case "until":
				until, err := time.Parse("2006-01-02", t)
				if err != nil {
					return nil, fmt.Errorf("invalid until: %q", t)
				}
				q.untilDate = Date{until}
			}
		} else {
			// Search term for a specific entry field.
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
	}
	return q, nil
}

// Returns true if s to lower case contains t, which is expected to be in lower case.
func matchLower(s, t string) bool {
	return strings.Contains(strings.ToLower(s), t)
}

func matchAsset(t string, a *Asset) bool {
	if matchLower(a.AccountNumber, t) ||
		matchLower(a.IBAN, t) || matchLower(a.ISIN, t) ||
		matchLower(a.Name, t) || matchLower(a.ShortName, t) ||
		matchLower(a.TickerSymbol, t) || matchLower(a.WKN, t) ||
		matchLower(a.CustomID, t) || matchLower(a.Comment, t) {
		return true
	}
	for _, s := range a.QuoteServiceSymbols {
		if matchLower(s, t) {
			return true
		}
	}
	return false
}

func (q *Query) Match(e *LedgerEntryRow) bool {
	if q.Empty() {
		return true
	}
	// Terms matching any of the "standard" fields.
	for _, t := range q.terms {
		if matchLower(e.Label(), t) || matchLower(e.Comment(), t) {
			continue
		}
		if e.A != nil && matchAsset(t, e.A) {
			continue
		}
		return false
	}
	// Terms matching specific fields.
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
			fval = e.EntryType().String()
		case "class":
			fval = e.AssetType().DisplayName()
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
	// Time range
	if !q.fromDate.IsZero() && q.fromDate.After(e.ValueDate().Time) {
		return false
	}
	if !q.untilDate.IsZero() && q.untilDate.Before(e.ValueDate().Time) {
		return false
	}
	// Sequence number
	if len(q.sequenceNums) > 0 {
		found := false
		for i := 1; i < len(q.sequenceNums); i += 2 {
			if q.sequenceNums[i-1] <= e.SequenceNum() && q.sequenceNums[i] >= e.SequenceNum() {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Sort ledger rows by (ValueDate, SequenceNum), ascending or descending.
func (q *Query) Sort(rows []*LedgerEntryRow) {
	ascCmp := func(a, b *LedgerEntryRow) int {
		c := a.E.ValueDate.Time.Compare(b.E.ValueDate.Time)
		if c != 0 {
			return c
		}
		return int(a.E.SequenceNum - b.E.SequenceNum)
	}
	cmp := ascCmp
	if q.descending {
		cmp = func(a, b *LedgerEntryRow) int {
			return -ascCmp(a, b)
		}
	}
	slices.SortFunc(rows, cmp)
}

func groupID(r *LedgerEntryRow) string {
	if r.HasAsset() {
		return r.AssetID()
	}
	if r.EntryType() == ExchangeRate {
		return string(r.E.Currency) + "/" + string(r.E.QuoteCurrency)
	}
	return ""
}

// Returns only the first N entries from rows for each "group" (asset or exchange rate).
func (q *Query) LimitGroups(rows []*LedgerEntryRow) []*LedgerEntryRow {
	if q.maxPerGroup == 0 {
		// No limit set
		return rows
	}
	var res []*LedgerEntryRow
	counts := map[string]int{}
	for _, row := range rows {
		gID := groupID(row)
		if counts[gID] >= q.maxPerGroup {
			continue
		}
		counts[gID]++
		res = append(res, row)
	}
	return res
}
