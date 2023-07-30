// Code generated by "go-enum -type=AssetType -string -json -all=false"; DO NOT EDIT.

// Install go-enum by `go get -u github.com/searKing/golang/tools/go-enum`
package kontoo

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[UnspecifiedAssetType-0]
	_ = x[Stock-1]
	_ = x[StockExchangeTradedFund-2]
	_ = x[StockMutualFund-3]
	_ = x[BondExchangeTradedFund-4]
	_ = x[BondMutualFund-5]
	_ = x[CorporateBond-6]
	_ = x[GovernmentBond-7]
	_ = x[FixedDepositAccount-8]
	_ = x[MoneyMarketAccount-9]
	_ = x[SavingsAccount-10]
	_ = x[CheckingAccount-11]
	_ = x[PensionAccount-12]
	_ = x[Commodity-13]
	_ = x[OtherAssetType-999]
}

const (
	_AssetType_name_0 = "UnspecifiedAssetTypeStockStockExchangeTradedFundStockMutualFundBondExchangeTradedFundBondMutualFundCorporateBondGovernmentBondFixedDepositAccountMoneyMarketAccountSavingsAccountCheckingAccountPensionAccountCommodity"
	_AssetType_name_1 = "OtherAssetType"
)

var (
	_AssetType_index_0 = [...]uint8{0, 20, 25, 48, 63, 85, 99, 112, 126, 145, 163, 177, 192, 206, 215}
)

func (i AssetType) String() string {
	switch {
	case 0 <= i && i <= 13:
		return _AssetType_name_0[_AssetType_index_0[i]:_AssetType_index_0[i+1]]
	case i == 999:
		return _AssetType_name_1
	default:
		return "AssetType(" + strconv.FormatInt(int64(i), 10) + ")"
	}
}

var _AssetType_values = []AssetType{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 999}

var _AssetType_name_to_values = map[string]AssetType{
	_AssetType_name_0[0:20]:    0,
	_AssetType_name_0[20:25]:   1,
	_AssetType_name_0[25:48]:   2,
	_AssetType_name_0[48:63]:   3,
	_AssetType_name_0[63:85]:   4,
	_AssetType_name_0[85:99]:   5,
	_AssetType_name_0[99:112]:  6,
	_AssetType_name_0[112:126]: 7,
	_AssetType_name_0[126:145]: 8,
	_AssetType_name_0[145:163]: 9,
	_AssetType_name_0[163:177]: 10,
	_AssetType_name_0[177:192]: 11,
	_AssetType_name_0[192:206]: 12,
	_AssetType_name_0[206:215]: 13,
	_AssetType_name_1[0:14]:    999,
}

// ParseAssetTypeString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func ParseAssetTypeString(s string) (AssetType, error) {
	if val, ok := _AssetType_name_to_values[s]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to AssetType values", s)
}

// AssetTypeValues returns all values of the enum
func AssetTypeValues() []AssetType {
	return _AssetType_values
}

// IsAAssetType returns "true" if the value is listed in the enum definition. "false" otherwise
func (i AssetType) Registered() bool {
	for _, v := range _AssetType_values {
		if i == v {
			return true
		}
	}
	return false
}

func _() {
	var _nil_AssetType_value = func() (val AssetType) { return }()

	// An "cannot convert AssetType literal (type AssetType) to type json.Marshaler" compiler error signifies that the base type have changed.
	// Re-run the go-enum command to generate them again.
	var _ json.Marshaler = _nil_AssetType_value

	// An "cannot convert AssetType literal (type AssetType) to type encoding.Unmarshaler" compiler error signifies that the base type have changed.
	// Re-run the go-enum command to generate them again.
	var _ json.Unmarshaler = &_nil_AssetType_value
}

// MarshalJSON implements the json.Marshaler interface for AssetType
func (i AssetType) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface for AssetType
func (i *AssetType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("AssetType should be a string, got %s", data)
	}

	var err error
	*i, err = ParseAssetTypeString(s)
	return err
}
