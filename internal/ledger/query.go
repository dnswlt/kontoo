package ledger

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dnswlt/kontoo/internal/civil"
)

type fieldTerm struct {
	field     string
	negated   bool
	term      string
	fullMatch bool // true means match the full string, not just a substring.
	re        *regexp.Regexp
}

type fieldOrdering struct {
	name       string
	descending bool
}

type Query struct {
	raw          string
	terms        []string
	fieldTerms   []fieldTerm
	sequenceNums []int64 // 2-pairs of inclusive ranges of valid sequence numbers. empty means "all numbers".
	fromDate     civil.Date
	untilDate    civil.Date
	descending   bool            // whether to return results in ascending (default) or descending order
	ordering     []fieldOrdering // Fields to order by. If set, the `descending` field is ignored.
	// Maximum number of entries to return per "group" (typically: asset)
	maxPerGroup int
}

func (q *Query) Raw() string {
	return q.raw
}

func (q *Query) Empty() bool {
	return q.raw == ""
}

var (
	// Pre-defined query terms that can be referenced through
	// variable names (e.g., $main).
	queryVariables = map[string][]string{
		"main": {"!type~price|rate", "order:newest"},
	}
)

var (
	// Field names by which LedgerEntryRow values can be compared.
	// Lower-case due to normalization.
	rowCompareFuncs = map[string]func(a, b *LedgerEntryRow) int{
		"assetid": func(a, b *LedgerEntryRow) int {
			return cmp.Compare(a.AssetID(), b.AssetID())
		},
		"assetname": func(a, b *LedgerEntryRow) int {
			return cmp.Compare(a.AssetName(), b.AssetName())
		},
		"newest": func(a, b *LedgerEntryRow) int {
			c := -a.ValueDate().Compare(b.ValueDate())
			if c != 0 {
				return c
			}
			c = cmp.Compare(a.Label(), b.Label())
			if c != 0 {
				return c
			}
			return int(b.SequenceNum() - a.SequenceNum())
		},
		"num": func(a, b *LedgerEntryRow) int {
			return int(a.SequenceNum() - b.SequenceNum())
		},
		"oldest": func(a, b *LedgerEntryRow) int {
			c := a.ValueDate().Compare(b.ValueDate())
			if c != 0 {
				return c
			}
			c = cmp.Compare(a.Label(), b.Label())
			if c != 0 {
				return c
			}
			return int(a.SequenceNum() - b.SequenceNum())
		},
		"valuedate": func(a, b *LedgerEntryRow) int {
			return a.ValueDate().Compare(b.ValueDate())
		},
	}
)

// parseOrdering parses the given string as a comma-separated
// list of supported field names to order results by.
// Each field name can be prefixed with a "-", indicating that the
// field should be ordered descending.
func parseOrdering(s string) ([]fieldOrdering, error) {
	var res []fieldOrdering
	fs := strings.Split(s, ",")
	for _, f := range fs {
		f = strings.TrimSpace(f)
		if len(f) == 0 || f == "-" {
			return nil, fmt.Errorf("empty field in ordering list %q", s)
		}
		desc := false
		if f[0] == '-' {
			f = f[1:]
			desc = true
		}
		if _, ok := rowCompareFuncs[f]; !ok {
			knownFields := make([]string, 0, len(rowCompareFuncs))
			for k := range rowCompareFuncs {
				knownFields = append(knownFields, k)
			}
			slices.Sort(knownFields)
			return nil, fmt.Errorf("invalid field %q (valid: %v)", f, strings.Join(knownFields, ","))
		}
		res = append(res, fieldOrdering{name: f, descending: desc})
	}
	return res, nil
}

// ParseQuery parses rawQuery as a query expression.
// A query expression consists of whitespace-separated query terms.
// A query term can be one of:
// foo    ==> Any of the "main" fields must contain the substring "foo".
// f:foo  ==> Field f must exist and contain the substring "foo"
// f=foo  ==> Field f must be exactly equal to "foo"
// f~foo  ==> Field f must regexp-match foo (which can be a full regexp, e.g. "bar|baz")
// Negations of the above: !f:foo, !f~foo, !f=foo
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
	q := &Query{
		raw:       rawQuery,
		untilDate: civil.DateVal(9999, 12, 31),
	}
	// expand query variables
	var expandedTerms []string
	for _, t := range strings.Fields(strings.ToLower(rawQuery)) {
		if t[0] == '$' {
			if repl, ok := queryVariables[t[1:]]; ok {
				expandedTerms = append(expandedTerms, repl...)
			} else {
				return nil, fmt.Errorf("undefined variable: %q", t)
			}
		} else {
			expandedTerms = append(expandedTerms, t)
		}
	}
	for _, ft := range expandedTerms {
		sep := strings.IndexAny(ft, ":=~")
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
		switch f {
		case "order":
			// Ordering
			if neg {
				return nil, fmt.Errorf("cannot negate \"order:\"")
			}
			if ft[sep] != ':' {
				return nil, fmt.Errorf("only operator : is allowed for %q filter", f)
			}
			switch t {
			case "asc":
				q.descending = false
			case "desc":
				q.descending = true
			default:
				var err error
				q.ordering, err = parseOrdering(t)
				if err != nil {
					return nil, fmt.Errorf(`invalid ordering %q: (must be "asc" or "desc" or list of field names): %v`, t, err)
				}
			}
		case "max":
			if ft[sep] != ':' {
				return nil, fmt.Errorf("only operator : is allowed for %q filter", f)
			}
			// Limits
			n, err := strconv.Atoi(t)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid argument for max: %q", t)
			}
			q.maxPerGroup = n
		case "num":
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
		case "date", "year", "from", "until":
			// Dates
			if ft[sep] != ':' {
				return nil, fmt.Errorf("only operator : is allowed for %q filter", f)
			}
			switch f {
			case "date":
				if d, err := civil.ParseDate(t); err == nil {
					q.fromDate = d
					q.untilDate = d
				} else if ym, err := time.Parse("2006-01", t); err == nil {
					q.fromDate = civil.ToDate(ym)
					q.untilDate = civil.ToDate(ym.AddDate(0, 1, -1))
				} else if y, err := strconv.Atoi(t); err == nil {
					q.fromDate = civil.DateVal(y, 1, 1)
					q.untilDate = civil.DateVal(y, 12, 31)
				} else {
					return nil, fmt.Errorf("invalid date: %q", t)
				}
			case "year":
				y, err := strconv.Atoi(t)
				if err != nil {
					return nil, fmt.Errorf("invalid year: %q", t)
				}
				q.fromDate = civil.DateVal(y, 1, 1)
				q.untilDate = civil.DateVal(y, 12, 31)
			case "from":
				from, err := civil.ParseDate(t)
				if err != nil {
					return nil, fmt.Errorf("invalid from: %q", t)
				}
				q.fromDate = from
			case "until":
				until, err := civil.ParseDate(t)
				if err != nil {
					return nil, fmt.Errorf("invalid until: %q", t)
				}
				q.untilDate = until
			}
		default:
			// Search term for a specific entry field.
			r := fieldTerm{
				field:     f,
				negated:   neg,
				fullMatch: ft[sep] == '=',
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
			// '~' match
			if t.re.MatchString(strings.ToLower(fval)) != expectMatch {
				return false
			}
		} else if t.fullMatch {
			// '=' match
			if match := strings.ToLower(fval) == t.term; match != expectMatch {
				return false
			}
		} else {
			// ':' match
			if matchLower(fval, t.term) != expectMatch {
				return false
			}
		}
	}
	// Time range
	if !q.fromDate.IsZero() && q.fromDate.After(e.ValueDate()) {
		return false
	}
	if !q.untilDate.IsZero() && q.untilDate.Before(e.ValueDate()) {
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

// Sort ledger rows by specified ordering fields or the default (ValueDate, SequenceNum).
func (q *Query) Sort(rows []*LedgerEntryRow) {
	var cmp func(a *LedgerEntryRow, b *LedgerEntryRow) int
	if len(q.ordering) == 0 {
		sgn := 1
		if q.descending {
			sgn = -1
		}
		// No fields specified => order by (ValueDate, SequenceNum).
		cmp = func(a, b *LedgerEntryRow) int {
			c := a.E.ValueDate.Compare(b.E.ValueDate)
			if c != 0 {
				return sgn * c
			}
			return sgn * int(a.E.SequenceNum-b.E.SequenceNum)
		}
	} else {
		// Order by specified ordering fields
		cmp = func(a, b *LedgerEntryRow) int {
			for _, o := range q.ordering {
				c := rowCompareFuncs[o.name](a, b)
				if c != 0 {
					if o.descending {
						return -c
					}
					return c
				}
			}
			// tie-breaker: sequence number
			return int(a.SequenceNum() - b.SequenceNum())
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
