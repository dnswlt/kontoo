package kontoo

import (
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
	TaxPrepayment           // Steuervorauszahlung
	CreditCardDebt          // Schulden auf Kreditkarte
	OtherDebt               // allg. Schulden
)

type AssetCategory string

type assetTypeInfo struct {
	typ         AssetType
	category    string
	displayName string
}

var (
	assetTypeInfoList = []assetTypeInfo{
		{
			typ:         Stock,
			category:    "Equity",
			displayName: "Stock",
		},
		{
			typ:         StockExchangeTradedFund,
			category:    "Equity",
			displayName: "ETF",
		},
		{
			typ:         StockMutualFund,
			category:    "Equity",
			displayName: "Mutual fund",
		},
		{
			typ:         BondExchangeTradedFund,
			category:    "Fixed-income",
			displayName: "Bond ETF",
		},
		{
			typ:         BondMutualFund,
			category:    "Fixed-income",
			displayName: "Bond mutual fund",
		},
		{
			typ:         CorporateBond,
			category:    "Fixed-income",
			displayName: "Corp bond",
		},
		{
			typ:         GovernmentBond,
			category:    "Fixed-income",
			displayName: "Gov bond",
		},
		{
			typ:         FixedDepositAccount,
			category:    "Account",
			displayName: "Fixed deposit",
		},
		{
			typ:         MoneyMarketAccount,
			category:    "Account",
			displayName: "Money market account",
		},
		{
			typ:         SavingsAccount,
			category:    "Account",
			displayName: "Savings account",
		},
		{
			typ:         CheckingAccount,
			category:    "Account",
			displayName: "Checking account",
		},
		{
			typ:         BrokerageAccount,
			category:    "Account",
			displayName: "Brokerage account",
		},
		{
			typ:         PensionAccount,
			category:    "Account",
			displayName: "Pension account",
		},
		{
			typ:         Commodity,
			category:    "Commodity",
			displayName: "Commodity",
		},
		{
			typ:         Cash,
			category:    "Cash",
			displayName: "Cash",
		},
		{
			typ:         TaxLiability,
			category:    "Taxes",
			displayName: "Tax liability",
		},
		{
			typ:         TaxPrepayment,
			category:    "Taxes",
			displayName: "Tax prepayment",
		},
		{
			typ:         CreditCardDebt,
			category:    "Debt",
			displayName: "Credit card",
		},
		{
			typ:         OtherDebt,
			category:    "Debt",
			displayName: "Other debt",
		},
	}
	// Map for fast lookup, populated in init():
	assetTypeInfos = make(map[AssetType]*assetTypeInfo)
)

func init() {
	for i := range assetTypeInfoList {
		a := &assetTypeInfoList[i]
		assetTypeInfos[a.typ] = a
	}
}

type Asset struct {
	Type           AssetType
	Name           string
	ShortName      string `json:",omitempty"`
	IssueDate      *Date  `json:",omitempty"`
	MaturityDate   *Date  `json:",omitempty"`
	InterestMicros Micros `json:"Interest,omitempty"`
	IBAN           string `json:",omitempty"`
	AccountNumber  string `json:",omitempty"`
	ISIN           string `json:",omitempty"`
	WKN            string `json:",omitempty"`
	TickerSymbol   string `json:",omitempty"`
	// More ticker symbols, to get stock quotes online.
	// Keyed by quote service. Not used as ID.
	QuoteServiceSymbols map[string]string `json:",omitempty"`
	CustomID            string            `json:",omitempty"`
	Currency            Currency
	AssetGroup          string `json:",omitempty"`
	Comment             string `json:",omitempty"`
}

//go:generate go-enum -type=EntryType -string -json -all=false
type EntryType int32

const (
	UnspecifiedEntryType EntryType = 0
	BuyTransaction       EntryType = 1
	SellTransaction      EntryType = 2
	AssetMaturity        EntryType = 3
	DividendPayment      EntryType = 4
	InterestPayment      EntryType = 5
	AssetPrice           EntryType = 6
	AccountBalance       EntryType = 7
	AssetHolding         EntryType = 8
	ExchangeRate         EntryType = 9
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
	Header      *LedgerHeader  `json:",omitempty"`
	Assets      []*Asset       `json:",omitempty"`
	AssetGroups []*AssetGroup  `json:",omitempty"`
	Entries     []*LedgerEntry `json:",omitempty"`
}

const (
	Millis    = 1_000
	UnitValue = 1_000_000
)

// Three-letter code, e.g. CHF, EUR, USD.
type Currency string

const (
	CHF Currency = "CHF"
	USD Currency = "USD"
	EUR Currency = "EUR"
)

var (
	currencyRegexp = regexp.MustCompile("^[A-Z]{3}$")
)

func (d Date) String() string {
	return d.Format("2006-01-02")
}

func (d Date) Compare(other Date) int {
	return d.Time.Compare(other.Time)
}

// Convenience function for sorting *Date.
func CompareDatePtr(l, r *Date) int {
	if l == nil {
		if r == nil {
			return 0
		}
		return -1
	}
	if r == nil {
		return 1
	}
	return l.Compare(*r)
}
