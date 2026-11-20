package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
)

// decodeConfigObject preserves number tokens at every depth. It accepts exactly
// 1 JSON object, not null, and refuses any trailing value or malformed data.
// This is a configuration-only path; the receiving middleware still validates
// every incoming argument with decodeArguments before this decoder runs.
func decodeConfigObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("configuration must be a JSON object, not null")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("trailing configuration data: %w", err)
		}
		return nil, fmt.Errorf("unexpected trailing JSON value after configuration object")
	}
	return object, nil
}

// configValuesEqual compares values from decodeConfigObject, including numbers
// inside arrays. Numeric spelling does not affect equality: 1, 1.0 and 1e0
// denote the same value. No comparison converts a number to float64.
func configValuesEqual(before, after any) bool {
	switch b := before.(type) {
	case json.Number:
		a, ok := after.(json.Number)
		if !ok {
			return false
		}
		bc, be := normalizedConfigNumber(b)
		ac, ae := normalizedConfigNumber(a)
		return bc == ac && be == ae
	case map[string]any:
		a, ok := after.(map[string]any)
		if !ok || len(b) != len(a) {
			return false
		}
		for key, bv := range b {
			av, present := a[key]
			if !present || !configValuesEqual(bv, av) {
				return false
			}
		}
		return true
	case []any:
		a, ok := after.([]any)
		if !ok || len(b) != len(a) {
			return false
		}
		for i := range b {
			if !configValuesEqual(b[i], a[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(before, after)
	}
}

// normalizedConfigNumber represents a validated JSON number as a signed decimal
// coefficient without leading/trailing zeros and a decimal exponent. It stores
// the exponent itself as a big.Int, never a power of 10. Thus storage depends
// on the input token's length, not the exponent's numeric magnitude.
func normalizedConfigNumber(number json.Number) (string, string) {
	mantissa := string(number)
	var exponent big.Int
	if i := strings.IndexAny(mantissa, "eE"); i >= 0 {
		exponent.SetString(mantissa[i+1:], 10)
		mantissa = mantissa[:i]
	}
	negative := strings.HasPrefix(mantissa, "-")
	mantissa = strings.TrimPrefix(mantissa, "-")
	fractionDigits := 0
	if i := strings.IndexByte(mantissa, '.'); i >= 0 {
		fractionDigits = len(mantissa) - i - 1
		mantissa = mantissa[:i] + mantissa[i+1:]
	}
	mantissa = strings.TrimLeft(mantissa, "0")
	if mantissa == "" {
		return "0", "0"
	}
	coefficient := strings.TrimRight(mantissa, "0")
	trailingZeros := len(mantissa) - len(coefficient)
	exponent.Add(&exponent, big.NewInt(int64(trailingZeros)-int64(fractionDigits)))
	if negative {
		coefficient = "-" + coefficient
	}
	return coefficient, exponent.String()
}
