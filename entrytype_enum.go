// Code generated by "go-enum -type=EntryType -string -json -all=false"; DO NOT EDIT.

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
	_ = x[UnspecifiedEntryType-0]
	_ = x[AssetPurchase-1]
	_ = x[AssetSale-2]
	_ = x[AssetPrice-3]
	_ = x[AssetHolding-4]
	_ = x[AccountCredit-5]
	_ = x[AccountDebit-6]
	_ = x[AccountBalance-7]
	_ = x[AssetMaturity-8]
	_ = x[DividendPayment-9]
	_ = x[InterestPayment-10]
	_ = x[ExchangeRate-11]
}

const _EntryType_name = "UnspecifiedEntryTypeAssetPurchaseAssetSaleAssetPriceAssetHoldingAccountCreditAccountDebitAccountBalanceAssetMaturityDividendPaymentInterestPaymentExchangeRate"

var _EntryType_index = [...]uint8{0, 20, 33, 42, 52, 64, 77, 89, 103, 116, 131, 146, 158}

func _() {
	var _nil_EntryType_value = func() (val EntryType) { return }()

	// An "cannot convert EntryType literal (type EntryType) to type fmt.Stringer" compiler error signifies that the base type have changed.
	// Re-run the go-enum command to generate them again.
	var _ fmt.Stringer = _nil_EntryType_value
}

func (i EntryType) String() string {
	if i < 0 || i >= EntryType(len(_EntryType_index)-1) {
		return "EntryType(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _EntryType_name[_EntryType_index[i]:_EntryType_index[i+1]]
}

var _EntryType_values = []EntryType{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}

var _EntryType_name_to_values = map[string]EntryType{
	_EntryType_name[0:20]:    0,
	_EntryType_name[20:33]:   1,
	_EntryType_name[33:42]:   2,
	_EntryType_name[42:52]:   3,
	_EntryType_name[52:64]:   4,
	_EntryType_name[64:77]:   5,
	_EntryType_name[77:89]:   6,
	_EntryType_name[89:103]:  7,
	_EntryType_name[103:116]: 8,
	_EntryType_name[116:131]: 9,
	_EntryType_name[131:146]: 10,
	_EntryType_name[146:158]: 11,
}

// ParseEntryTypeString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func ParseEntryTypeString(s string) (EntryType, error) {
	if val, ok := _EntryType_name_to_values[s]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to EntryType values", s)
}

// EntryTypeValues returns all values of the enum
func EntryTypeValues() []EntryType {
	return _EntryType_values
}

// IsAEntryType returns "true" if the value is listed in the enum definition. "false" otherwise
func (i EntryType) Registered() bool {
	for _, v := range _EntryType_values {
		if i == v {
			return true
		}
	}
	return false
}

func _() {
	var _nil_EntryType_value = func() (val EntryType) { return }()

	// An "cannot convert EntryType literal (type EntryType) to type json.Marshaler" compiler error signifies that the base type have changed.
	// Re-run the go-enum command to generate them again.
	var _ json.Marshaler = _nil_EntryType_value

	// An "cannot convert EntryType literal (type EntryType) to type encoding.Unmarshaler" compiler error signifies that the base type have changed.
	// Re-run the go-enum command to generate them again.
	var _ json.Unmarshaler = &_nil_EntryType_value
}

// MarshalJSON implements the json.Marshaler interface for EntryType
func (i EntryType) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface for EntryType
func (i *EntryType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("EntryType should be a string, got %s", data)
	}

	var err error
	*i, err = ParseEntryTypeString(s)
	return err
}
