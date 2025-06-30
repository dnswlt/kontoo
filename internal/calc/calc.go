package calc

import (
	"fmt"
	"math"

	"github.com/dnswlt/kontoo/internal/civil"
	"github.com/dnswlt/kontoo/internal/ledger"
	"github.com/dnswlt/kontoo/internal/units"
)

// diffYears returns the difference d1-d2, expressed in years.
// It assumes a year has 365 days.
func diffYears(d1, d2 civil.Date) float64 {
	return float64(d1.DaysSince(d2)) / 365
}

// newton implements the Newton-Raphson root-finding algorithm to find the root
// of f(x)-y.
// ffp must return the tuple (f(x), f'(x)).
// x0 is the initial guess for the root.
func newton(y, x0 float64, ffp func(float64) (float64, float64)) (float64, error) {
	const maxIter = 10
	const precision = 1e-7
	x := x0
	for k := 0; k < maxIter; k++ {
		y1, u1 := ffp(x)
		if math.Abs(y-y1) <= precision {
			return x, nil
		}
		if u1 == 0 {
			return 0, fmt.Errorf("newton: zero derivative at %f", x)
		}
		x1 := x - (y1-y)/u1
		// fmt.Printf("x_%d = %.3f x_%d = %.3f\n", k, x, k+1, x1)
		x = x1
	}
	return 0, fmt.Errorf("newton: failed to converge after %d iterations", maxIter)
}

// bisect finds a zero of f(x)-y. f is assumed to be monotonically increasing.
// [low, high] is the initial interval that is assumed to contain a zero,
// but it will be adjusted dynamically if that is not the case.
// bisect returns an error if low >= high or if no zero was found after
// 50 iterations.
func bisect(y, low, high float64, f func(float64) float64) (float64, error) {
	const maxIter = 50
	const precision = 1e-7
	if low >= high {
		return 0, fmt.Errorf("bisect: low(%v) must be less than high(%v)", low, high)
	}
	// Adjust boundaries if necessary, to find a change in sign
	step := high - low
	k := 0
	for ; k < maxIter && f(low)-y > 0; k++ {
		high = low
		low -= step
		step *= 2
	}
	for ; k < maxIter && f(high)-y < 0; k++ {
		low = high
		high += step
		step *= 2
	}
	for ; k < maxIter; k++ { // Give up after maxIter attempts
		x := (low + high) / 2
		fx := f(x)
		if high-low < precision {
			return x, nil
		}
		if fx < y {
			low = x
		} else {
			high = x
		}
	}
	return 0, fmt.Errorf("bisect: failed to converge after %d iterations", maxIter)
}

// TotalEarningsAtMaturity calculates the predicted earnings of a fixed-income
// asset position by its maturity date. These earnings include both interest
// and capital gains (or losses) from price appreciation or depreciation.
// It assumes the asset matures at 100% of its nominal value.
func TotalEarningsAtMaturity(p *ledger.AssetPosition) units.Micros {
	md := p.Asset.MaturityDate
	if md == nil {
		return 0
	}
	interestRate := p.Asset.InterestMicros
	var interest, gains units.Micros
	for _, item := range p.Items {
		years := diffYears(*md, item.ValueDate)
		gains += item.QuantityMicros - item.PurchasePrice()
		switch p.Asset.InterestPayment {
		case ledger.AccruedPayment:
			interest += item.QuantityMicros.Mul(
				units.FloatAsMicros(math.Pow(1+interestRate.Float(), years))) - item.QuantityMicros
		case ledger.AnnualPayment:
			interest += item.QuantityMicros.Mul(interestRate).Mul(units.FloatAsMicros(years))
		default:
			// If no payment schedule is specified, we can't calculate the interest
		}
	}
	return interest + gains
}

// XIRR calculates the internal rate of return like Excel's XIRR function does.
// values and dates have to be of the same length (>= 2), and dates have to be sorted
// in ascending order.
func XIRR(values []units.Micros, dates []civil.Date) (units.Micros, error) {
	if len(values) != len(dates) {
		return 0, fmt.Errorf("values and dates are of different sizes")
	}
	l := len(values)
	if l < 2 {
		return 0, fmt.Errorf("too few values")
	}
	for i := 1; i < l; i++ {
		if dates[i-1].After(dates[i]) {
			return 0, fmt.Errorf("dates are not sorted")
		}
	}
	// Prepare dates (time in years, values as floats).
	ts := make([]float64, l)
	xs := make([]float64, l)
	endDate := dates[l-1]
	for i := 0; i < l; i++ {
		ts[i] = diffYears(endDate, dates[i])
		xs[i] = values[i].Float()
	}

	// returnsFunc calculates the returns y and first derivative yd
	// using r as the (accruing) interest rate.
	returnsFunc := func(r float64) (y float64, yd float64) {
		for i := 0; i < len(ts); i++ {
			y += xs[i] * math.Pow(1+r, ts[i])
			yd += ts[i] * xs[i] * math.Pow(1+r, ts[i]-1)
		}
		return y, yd
	}
	// Find the zero using Newton first.
	irr, err := newton(0, 0.05, returnsFunc)
	if err != nil {
		// Newton did not converge, try bisection.
		irr, err = bisect(0, 0, 0.1, func(r float64) float64 {
			y, _ := returnsFunc(r)
			return y
		})
		if err != nil {
			// No method found a solution, give up
			return 0, err
		}
	}
	return units.FloatAsMicros(irr), nil
}

type IRRParams struct {
	NominalValue    units.Micros
	Price           units.Micros
	InterestRate    units.Micros
	Cost            units.Micros
	PurchaseDate    civil.Date
	MaturityDate    civil.Date
	InterestPayment ledger.InterestPaymentSchedule
}

// irrWithInterest calculates the IRR for a single purchase of a fixed-income asset.
// The price must be given in %.
// This function is used in the Calculator UI.
func IRRWithInterest(p IRRParams) (units.Micros, error) {

	var values []units.Micros
	var dates []civil.Date
	values = append(values, -p.Price.Mul(p.NominalValue)-p.Cost)
	dates = append(dates, p.PurchaseDate)

	switch p.InterestPayment {
	case ledger.AnnualPayment:
		// Append interest payments each year on the (MM/DD) day of maturity.
		var iDates []civil.Date
		d := p.MaturityDate
		for d.After(p.PurchaseDate) {
			iDates = append(iDates, d)
			d = d.AddDate(-1, 0, 0)
		}
		l := len(iDates)
		for i := l - 1; i >= 0; i-- {
			interest := p.InterestRate
			if i == l-1 {
				// Pro-rate interest of first year. There may be leap year anomalies (s == 366/365),
				// but we don't account for that in accrued payments below either.
				s := diffYears(iDates[i], p.PurchaseDate)
				interest = interest.Mul(units.FloatAsMicros(s))
			}
			values = append(values, p.NominalValue.Mul(interest))
			dates = append(dates, iDates[i])
		}
	case ledger.AccruedPayment:
		// Append a single (accrued) interest payment on the date of maturity.
		years := diffYears(p.MaturityDate, p.PurchaseDate)
		interest := units.FloatAsMicros(math.Pow(1+p.InterestRate.Float(), years) - 1)
		values = append(values, p.NominalValue.Mul(interest))
		dates = append(dates, p.MaturityDate)
	default:
		return 0, fmt.Errorf("unsupported payment schedule: %v", p.InterestPayment)
	}
	// Append redemption payment.
	values = append(values, p.NominalValue)
	dates = append(dates, p.MaturityDate)

	return XIRR(values, dates)
}

// InternalRateOfReturn calculates the internal rate of return (IRR) of the
// given asset position. Its semantics are similar to Excel's XIRR function;
// in contrast to XIRR (and irrWithInterest above), this function does not
// include compound interest payments for positions with annual interest
// payment schedules. Instead, all interest is assumed to be paid out in full
// at the date of maturity (without accrual).
// The intuition is that while interest from bonds with annual payments
// are probably reinvested somehow, they are typically not reinvested in the
// same bond and may therefore yield very different returns.
func InternalRateOfReturn(p *ledger.AssetPosition) units.Micros {
	md := p.Asset.MaturityDate
	if md == nil || len(p.Items) == 0 {
		return 0
	}
	tem := TotalEarningsAtMaturity(p).Float()
	if tem == 0 {
		return 0
	}
	if len(p.Items) == 1 {
		// Fast path: with one position item we can use a closed form.
		// y = x * (1+irr)^t
		// ==>
		// irr = (y/x)^(1/t) - 1
		t := diffYears(*md, p.Items[0].ValueDate)
		x := p.Items[0].PurchasePrice().Float()
		y := x + tem
		return units.FloatAsMicros(math.Pow(y/x, 1/t) - 1)
	}
	// More than one item: use bisection.
	ts := make([]float64, len(p.Items))
	xs := make([]float64, len(p.Items))
	var xsSum float64
	for i, item := range p.Items {
		ts[i] = diffYears(*md, item.ValueDate)
		xs[i] = item.PurchasePrice().Float()
		xsSum += xs[i]
	}
	returnsFunc := func(r float64) (float64, float64) {
		// Calculate returns y using r as the (accruing) interest rate.
		var y, yd float64
		for i := 0; i < len(ts); i++ {
			y += xs[i] * math.Pow(1+r, ts[i])
			yd += ts[i] * xs[i] * math.Pow(1+r, ts[i]-1)
		}
		return y, yd
	}
	irr, err := newton(xsSum+tem, 0.05, returnsFunc)
	if err != nil {
		// Newton did not converge, try bisection.
		fmt.Println("INFO Newton did not converge")
		irr, err = bisect(xsSum+tem, 0, 0.1, func(r float64) float64 {
			y, _ := returnsFunc(r)
			return y
		})
		if err != nil {
			// No method found a solution, give up
			return 0
		}
	}
	return units.FloatAsMicros(irr)
}
