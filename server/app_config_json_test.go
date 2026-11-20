package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeConfigObject(t *testing.T) {
	for _, raw := range []string{
		``, `null`, `[]`, `1`, `"text"`, `{"n":}`, `{"n":1} {}`, `{"n":1} null`, `{"n":1} garbage`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := decodeConfigObject([]byte(raw)); err == nil {
				t.Fatal("expected invalid configuration error")
			}
		})
	}
	got, err := decodeConfigObject([]byte(" {\"n\":[9007199254740993,0.12345678901234567890123456789,1e-400]} \n"))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []json.Number{"9007199254740993", "0.12345678901234567890123456789", "1e-400"} {
		if got["n"].([]any)[i] != want {
			t.Errorf("number %d = %v, want exact %s", i, got["n"].([]any)[i], want)
		}
	}
}

func TestConfigValuesEqual_ExactNumbers(t *testing.T) {
	for _, tc := range []struct {
		before string
		after  string
		equal  bool
	}{
		{"1", "1.0", true},
		{"1.0", "1e0", true},
		{"1", "1E+0", true},
		{"12.30", "123e-1", true},
		{"-12.30", "-123e-1", true},
		{"0", "-0.000e999999999999999999999999999999", true},
		{"1e-400", "10e-401", true},
		{"1e999999999999999999999999999999", "10e999999999999999999999999999998", true},
		{"1e-999999999999999999999999999999", "10e-1000000000000000000000000000000", true},
		{"9007199254740992", "9007199254740993", false},
		{"0.12345678901234567890123456789", "0.12345678901234567890123456790", false},
		{"0", "1e-400", false},
		{"1e-400", "2e-400", false},
		{"1", "-1", false},
	} {
		t.Run(tc.before+"/"+tc.after, func(t *testing.T) {
			if got := configValuesEqual(json.Number(tc.before), json.Number(tc.after)); got != tc.equal {
				t.Errorf("equality = %v, want %v", got, tc.equal)
			}
		})
	}
}

func TestConfigValuesEqual_NestedArrays(t *testing.T) {
	for _, tc := range []struct {
		after string
		equal bool
	}{
		{`{"v":[1.0,{"n":[1e0,12.30]},null,"x",true]}`, true},
		{`{"v":[1,{"n":[1,12.31]},null,"x",true]}`, false},
		{`{"v":[1,{"n":[1,12.3]},null,"x",false]}`, false},
		{`{"v":[1,{"n":[1,12.3]},null,"x"]}`, false},
		{`{"v":[1,{"other":[1,12.3]},null,"x",true]}`, false},
		{`{"v":["1",{"n":[1,12.3]},null,"x",true]}`, false},
		{`{"v":null}`, false},
	} {
		t.Run(tc.after, func(t *testing.T) {
			before, err := decodeConfigObject([]byte(`{"v":[1,{"n":[1,12.3]},null,"x",true]}`))
			if err != nil {
				t.Fatal(err)
			}
			after, err := decodeConfigObject([]byte(tc.after))
			if err != nil {
				t.Fatal(err)
			}
			if got := configValuesEqual(before, after); got != tc.equal {
				t.Errorf("equality = %v, want %v", got, tc.equal)
			}
		})
	}
}

func TestNormalizedConfigNumber_ExtremeExponentStaysCompact(t *testing.T) {
	exponent := strings.Repeat("9", 1000)
	coefficient, gotExponent := normalizedConfigNumber(json.Number("1e" + exponent))
	if coefficient != "1" || gotExponent != exponent {
		t.Fatal("normalization must retain a compact coefficient and exponent")
	}
	if !configValuesEqual(json.Number("1e-"+exponent), json.Number("1.0e-"+exponent)) {
		t.Fatal("extreme negative exponents must compare without expansion")
	}
}
