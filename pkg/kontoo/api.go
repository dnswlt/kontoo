package kontoo

import (
	"fmt"
	"regexp"
	"time"
)

//go:generate go-enum -type=AssetType -string -json -all=false
type AssetType int32

// Values for AssetType constants may change over time. Only the name
// should be assumed constant (and persisted in the JSON ledger).
const (
	UnspecifiedAssetType AssetType = iota

	Stock                   // Aktie
	StockExchangeTradedFund // Aktienfonds (ETF)
	StockMutualFund         // Aktienfonds (Investment)
	BondExchangeTradedFund  // Rentenfonds (ETF)
	BondMutualFund          // Rentenfonds (Investment)
	CorporateBond           // Unternehmensanleihe
	GovernmentBond          // Staatsanleihe
	FixedDepositAccount     // Festgeldkonto
	MoneyMarketAccount      // Tagesgeldkonto
	SavingsAccount          // Sparkonto
	CheckingAccount         // Girokonto
	BrokerageAccount        // Verrechnungskonto
	PensionAccount          // Altersvorsorgekonten (z.B. Säule 3a)
	Commodity               // Edelmetalle, Rohstoffe
	Cash                    // Bargeld
	TaxLiability            // Steuerschuld
	TaxPayment              // Steuer(voraus)zahlung
	CreditCardDebt          // Schulden auf Kreditkarte
	OtherDebt               // allg. Schulden
)

type AssetCategory int

const (
	UnspecfiedAssetCategory AssetCategory = iota
	Equity
	FixedIncome
	CashEquivalents
	RetirementSavings
	Commodities
	Taxes
	Debt
)

func (ac AssetCategory) String() string {
	switch ac {
	case UnspecfiedAssetCategory:
		return "Unspecified"
	case Equity:
		return "Equity"
	case FixedIncome:
		return "Fixed-income"
	case CashEquivalents:
		return "Cash equivalents"
	case RetirementSavings:
		return "Retirement savings"
	case Commodities:
		return "Commodities"
	case Taxes:
		return "Taxes"
	case Debt:
		return "Debt"
	default:
		panic(fmt.Sprintf("invalid AssetCategory: %d", ac))
	}
}

type assetTypeInfo struct {
	typ             AssetType
	category        AssetCategory
	displayName     string
	validEntryTypes []EntryType
	// True if the asset type tracks invididual credit/debit
	// transactions in asset positions. The alternative is to only
	// track the current balance.
	useTransactionTracking bool
	// True if the asset type is "account-like". Should be set to true
	// for all types for which AccountBalance is a common ledger entry type.
	isAccountType bool
	// True if the asset type supports adding repeated ledger entries
	// (e.g. tax debit repeated for the next 12 months).
	supportsRepeatedLedgerEntries bool
}

var (
	// For fast info lookup. Each AssetType's info is at the index
	// of its own int value. Populated in init() from _assetTypeInfosList.
	assetTypeInfos []assetTypeInfo

	_assetTypeInfosList = []assetTypeInfo{
		{
			typ:             Stock,
			category:        Equity,
			displayName:     "Stock",
			validEntryTypes: []EntryType{AssetPurchase, AssetSale, AssetPrice, AssetHolding, DividendPayment},
		},
		{
			typ:             StockExchangeTradedFund,
			category:        Equity,
			displayName:     "ETF",
			validEntryTypes: []EntryType{AssetPurchase, AssetSale, AssetPrice, AssetHolding, DividendPayment},
		},
		{
			typ:             StockMutualFund,
			category:        Equity,
			displayName:     "Mutual fund",
			validEntryTypes: []EntryType{AssetPurchase, AssetSale, AssetPrice, AssetHolding, DividendPayment},
		},
		{
			typ:             BondExchangeTradedFund,
			category:        FixedIncome,
			displayName:     "Bond ETF",
			validEntryTypes: []EntryType{AssetPurchase, AssetSale, AssetPrice, AssetHolding, InterestPayment},
		},
		{
			typ:             BondMutualFund,
			category:        FixedIncome,
			displayName:     "Bond mutual fund",
			validEntryTypes: []EntryType{AssetPurchase, AssetSale, AssetPrice, AssetHolding, InterestPayment},
		},
		{
			typ:             CorporateBond,
			category:        FixedIncome,
			displayName:     "Corp bond",
			validEntryTypes: []EntryType{AssetPurchase, AssetSale, AssetPrice, AssetHolding, InterestPayment, AssetMaturity},
		},
		{
			typ:             GovernmentBond,
			category:        FixedIncome,
			displayName:     "Gov bond",
			validEntryTypes: []EntryType{AssetPurchase, AssetSale, AssetPrice, AssetHolding, InterestPayment, AssetMaturity},
		},
		{
			typ:                    FixedDepositAccount,
			category:               FixedIncome,
			displayName:            "Fixed deposit",
			validEntryTypes:        []EntryType{AccountCredit, AccountDebit, AccountBalance, InterestPayment, AssetMaturity},
			useTransactionTracking: true,
			isAccountType:          true,
		},
		{
			typ:             MoneyMarketAccount,
			category:        CashEquivalents,
			displayName:     "Money mkt acct",
			validEntryTypes: []EntryType{AccountCredit, AccountDebit, AccountBalance, InterestPayment},
			isAccountType:   true,
		},
		{
			typ:             SavingsAccount,
			category:        CashEquivalents,
			displayName:     "Savings account",
			validEntryTypes: []EntryType{AccountCredit, AccountDebit, AccountBalance, InterestPayment},
			isAccountType:   true,
		},
		{
			typ:             CheckingAccount,
			category:        CashEquivalents,
			displayName:     "Checking account",
			validEntryTypes: []EntryType{AccountCredit, AccountDebit, AccountBalance, InterestPayment},
			isAccountType:   true,
		},
		{
			typ:             BrokerageAccount,
			category:        CashEquivalents,
			displayName:     "Brokerage acct",
			validEntryTypes: []EntryType{AccountCredit, AccountDebit, AccountBalance, InterestPayment},
			isAccountType:   true,
		},
		{
			typ:                    PensionAccount,
			category:               RetirementSavings,
			displayName:            "Pension account",
			validEntryTypes:        []EntryType{AccountCredit, AccountDebit, AccountBalance, InterestPayment},
			useTransactionTracking: true,
			isAccountType:          true,
		},
		{
			typ:             Commodity,
			category:        Commodities,
			displayName:     "Commodity",
			validEntryTypes: []EntryType{AssetPurchase, AssetSale, AssetPrice, AssetHolding},
		},
		{
			typ:             Cash,
			category:        CashEquivalents,
			displayName:     "Cash",
			validEntryTypes: []EntryType{AccountCredit, AccountDebit, AccountBalance},
		},
		{
			typ:                           TaxLiability,
			category:                      Taxes,
			displayName:                   "Tax liability",
			validEntryTypes:               []EntryType{AccountCredit, AccountDebit, AccountBalance},
			useTransactionTracking:        true,
			supportsRepeatedLedgerEntries: true,
		},
		{
			typ:                    TaxPayment,
			category:               Taxes,
			displayName:            "Tax payment",
			validEntryTypes:        []EntryType{AccountCredit, AccountDebit, AccountBalance},
			useTransactionTracking: true,
		},
		{
			typ:             CreditCardDebt,
			category:        Debt,
			displayName:     "Credit card",
			validEntryTypes: []EntryType{AccountCredit, AccountDebit, AccountBalance},
			isAccountType:   true,
		},
		{
			typ:             OtherDebt,
			category:        Debt,
			displayName:     "Other debt",
			validEntryTypes: []EntryType{AccountCredit, AccountDebit, AccountBalance},
		},
	}
)

func init() {
	assetTypeInfos = make([]assetTypeInfo, len(AssetTypeValues()))
	for _, a := range _assetTypeInfosList {
		assetTypeInfos[a.typ] = a
	}
}

func (t AssetType) ValidEntryTypes() []EntryType {
	return assetTypeInfos[t].validEntryTypes
}

func (t AssetType) SupportsRepeatedLedgerEntries() bool {
	return assetTypeInfos[t].supportsRepeatedLedgerEntries
}

func (t AssetType) category() AssetCategory {
	return assetTypeInfos[t].category
}

func (t AssetType) DisplayName() string {
	return assetTypeInfos[t].displayName
}

func (t AssetType) UseTransactionTracking() bool {
	return assetTypeInfos[t].useTransactionTracking
}
func (t AssetType) IsAccountType() bool {
	return assetTypeInfos[t].isAccountType
}

type InterestPaymentSchedule string

const (
	UnspecifiedPayment InterestPaymentSchedule = ""
	AccruedPayment     InterestPaymentSchedule = "accrued" // Interest paid at maturity
	AnnualPayment      InterestPaymentSchedule = "annual"  // Interest paid yearly
)

var allInterestPaymentSchedules = [...]InterestPaymentSchedule{
	UnspecifiedPayment,
	AccruedPayment,
	AnnualPayment,
}

type Asset struct {
	Created         time.Time
	Modified        time.Time
	Type            AssetType
	Name            string
	ShortName       string                  `json:",omitempty"`
	IssueDate       *Date                   `json:",omitempty"`
	MaturityDate    *Date                   `json:",omitempty"`
	InterestMicros  Micros                  `json:"Interest,omitempty"`
	InterestPayment InterestPaymentSchedule `json:",omitempty"`
	IBAN            string                  `json:",omitempty"`
	AccountNumber   string                  `json:",omitempty"`
	ISIN            string                  `json:",omitempty"`
	WKN             string                  `json:",omitempty"`
	TickerSymbol    string                  `json:",omitempty"`
	// More ticker symbols, to get stock quotes online.
	// Keyed by quote service. Not used as ID.
	QuoteServiceSymbols map[string]string `json:",omitempty"`
	// (Optional) time zone in which the main exchange trading the equity is located.
	ExchangeTimezone string `json:",omitempty"`
	CustomID         string `json:",omitempty"`
	Currency         Currency
	Comment          string `json:",omitempty"`
}

//go:generate go-enum -type=EntryType -string -json -all=false
type EntryType int32

const (
	UnspecifiedEntryType EntryType = iota

	AssetPurchase
	AssetSale
	AssetPrice
	AssetHolding
	AccountCredit
	AccountDebit
	AccountBalance
	AssetMaturity
	DividendPayment
	InterestPayment
	ExchangeRate
)

// Dates without a time component.
type Date struct {
	time.Time
}

type LedgerEntry struct {
	Created     time.Time
	SequenceNum int64
	ValueDate   Date      `json:",omitempty"`
	Type        EntryType `json:",omitempty"`
	AssetRef    string    `json:",omitempty"`
	AssetID     string    `json:",omitempty"`

	Currency Currency `json:",omitempty"`

	// Only set for ExchangeRate type entries. Currency represents the base currency in that case.
	QuoteCurrency Currency `json:",omitempty"`

	// All *Micros fields are given in either micros of the currency or micros of a fraction.
	// 1'000'000 in ValueMicros equals 1.00 CHF (or whatever the Currency),
	// 500'000 PriceMicros of a bond equal a price of 50% of the nominal value.

	// Value in micros of the currency. For currency CHF, 1'000'000 ValueMicros equals 1 CHF.
	// Except for accounts, ValueMicros is only informational. The current value of other asset positions
	// is calculated from its QuantityMicros and its PriceMicros.
	ValueMicros    Micros `json:"Value,omitempty"`    // Account balance or asset value as calculated from quantity and price.
	QuantityMicros Micros `json:"Quantity,omitempty"` // Number of stocks, oz of gold, nominal value of a bond
	PriceMicros    Micros `json:"Price,omitempty"`    // Price of a single quantity of the asset. (1 * UnitValue) means 100% for prices specified in percent.
	CostMicros     Micros `json:"Cost,omitempty"`     // Cost incurred by the transaction.

	Comment string `json:",omitempty"`
}

type LedgerHeader struct {
	BaseCurrency Currency `json:",omitempty"`
}

type AssetGroup struct {
	ID   string
	Name string
}

type Ledger struct {
	Header  *LedgerHeader  `json:",omitempty"`
	Assets  []*Asset       `json:",omitempty"`
	Entries []*LedgerEntry `json:",omitempty"`
}

const (
	Millis    = 1_000
	UnitValue = 1_000_000
)

// Three-letter code, e.g. CHF, EUR, USD.
type Currency string

var (
	// Regexp for ISO 3-letter currency codes.
	currencyRegexp = regexp.MustCompile("^[A-Z]{3}$")
	// Lists known currencies.
	// Used in ledger validation: other currencies will not be accepted.
	validCurrencies = map[Currency]bool{
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

// Reports whether c is a valid and known currency.
func ValidCurrency(c Currency) bool {
	return validCurrencies[c]
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
	return Date{d.AddDate(0, 0, n)}
}

func (t EntryType) NeedsAssetID() bool {
	return t != ExchangeRate && t != UnspecifiedEntryType
}
