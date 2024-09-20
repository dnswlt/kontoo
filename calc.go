package kontoo

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

// totalEarningsAtMaturity calculates the predicted earnings of a fixed-income
// asset position by its maturity date. These earnings include both interest
// and capital gains (or losses) from price appreciation or depreciation.
// It assumes the asset matures at 100% of its nominal value.
func totalEarningsAtMaturity(p *AssetPosition) Micros {
	md := p.Asset.MaturityDate
	if md == nil {
		return 0
	}
	interestRate := p.Asset.InterestMicros
	var interest, gains Micros
	for _, item := range p.Items {
		years := md.Sub(item.ValueDate.Time).Hours() / 24 / 365
		gains += item.QuantityMicros - item.PurchasePrice()
		switch p.Asset.InterestPayment {
		case AccruedPayment:
			interest += item.QuantityMicros.Mul(FloatAsMicros(math.Pow(1+interestRate.Float(), years))) - item.QuantityMicros
		case AnnualPayment:
			interest += item.QuantityMicros.Mul(interestRate).Mul(FloatAsMicros(years))
		default:
			// If no payment schedule is specified, we can't calculate the interest
		}
	}
	return interest + gains
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

// internalRateOfReturn calculates the internal rate of return (IRR) of the
// given asset position. Its semantics are analogous to Excel's XIRR function.
func internalRateOfReturn(p *AssetPosition) Micros {
	md := p.Asset.MaturityDate
	if md == nil || len(p.Items) == 0 {
		return 0
	}
	tem := totalEarningsAtMaturity(p).Float()
	if tem == 0 {
		return 0
	}
	if len(p.Items) == 1 {
		// Fast path: with one position item we can use a closed form.
		// y = x * (1+irr)^t
		// ==>
		// irr = (y/x)^(1/t) - 1
		t := md.Sub(p.Items[0].ValueDate.Time).Hours() / 24 / 365
		x := p.Items[0].PurchasePrice().Float()
		y := x + tem
		return FloatAsMicros(math.Pow(y/x, 1/t) - 1)
	}
	// More than one item: use bisection.
	ts := make([]float64, len(p.Items))
	xs := make([]float64, len(p.Items))
	var xsSum float64
	for i, item := range p.Items {
		ts[i] = md.Sub(item.ValueDate.Time).Hours() / 24 / 365
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
	return FloatAsMicros(irr)
}

// Big ints used in IBAN validation.
var (
	bigInts36 [36]*big.Int
	big100    = big.NewInt(100)
	big97     = big.NewInt(97)
)

func init() {
	for i := 0; i < len(bigInts36); i++ {
		bigInts36[i] = big.NewInt(int64(i))
	}
}

// validIBAN reports whether the given IBAN is valid according to the
// ISO 13616:2020 validation rules. In particular, it checks whether
// the ISO code has the right format and whether the checksum is valid.
func validIBAN(iban string) bool {
	if iban == "" {
		return false
	}
	if iban[0] == ' ' || iban[len(iban)-1] == ' ' {
		return false // No whitespace at the beginning or end.
	}
	// Ignore whitespace in the middle.
	iban = strings.ReplaceAll(iban, " ", "")
	// Needs to have ISO code + checksum + at least something.
	if len(iban) < 5 {
		return false
	}
	// Check ISO code and checksum.
	for i := 0; i < 4; i++ {
		if !(iban[i] >= 'A' && iban[i] <= 'Z' || i >= 2 && iban[i] >= '0' && iban[i] <= '9') {
			return false
		}
	}
	// Build number for "mod 97" validation.
	k := big.NewInt(0)
	for _, r := range iban[4:] {
		if r >= 'A' && r <= 'Z' {
			k.Mul(k, big100) // Shift left by two digits.
			k.Add(k, bigInts36[int(r-'A')+10])
		} else if r >= '0' && r <= '9' {
			k.Mul(k, bigInts36[10])
			k.Add(k, bigInts36[int(r-'0')])
		} else {
			return false
		}
	}
	// Add the first 4 characters at the end of the validation number.
	for i := 0; i < 4; i++ {
		c := iban[i]
		if c >= 'A' && c <= 'Z' {
			k.Mul(k, big100) // Shift left by two digits.
			k.Add(k, bigInts36[int(c-'A')+10])
		} else {
			// Must be a digit, was checked above.
			k.Mul(k, bigInts36[10])
			k.Add(k, bigInts36[int(c-'0')])
		}
	}
	k.Mod(k, big97)
	return k.Cmp(bigInts36[1]) == 0
}
