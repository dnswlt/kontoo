package units

// Three-letter code, e.g. CHF, EUR, USD.
type Currency string

var (
	// Lists known currencies.
	// Used in ledger validation: other currencies will not be accepted.
	validCurrencies = map[string]bool{
		"EUR": true, // Euro
		"USD": true, // US Dollar
		"CHF": true, // Swiss Franc
		"GBP": true, // British Pound Sterling
		"NOK": true, // Norwegian Krone
		"SEK": true, // Swedish Krona
		"DKK": true, // Danish Krone
		"JPY": true, // Japanese Yen
		"CAD": true, // Canadian Dollar
		"AUD": true, // Australian Dollar
		"CNY": true, // Chinese Yuan
	}
)

func (c Currency) Valid() bool {
	return ValidCurrency(string(c))
}

// Reports whether c is a valid and known currency.
func ValidCurrency(c string) bool {
	return validCurrencies[c]
}
