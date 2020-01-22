package tokenamount_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "github.com/filecoin-project/go-fil-markets/shared/tokenamount"
)

func TestBigIntSerializationRoundTrip(t *testing.T) {
	testValues := []string{
		"0", "1", "10", "-10", "9999", "12345678901234567891234567890123456789012345678901234567890",
	}

	for _, v := range testValues {
		bi, err := FromString(v)
		if err != nil {
			t.Fatal(err)
		}

		buf := new(bytes.Buffer)
		if err := bi.MarshalCBOR(buf); err != nil {
			t.Fatal(err)
		}

		var out TokenAmount
		if err := out.UnmarshalCBOR(buf); err != nil {
			t.Fatal(err)
		}

		if Cmp(out, bi) != 0 {
			t.Fatal("failed to round trip BigInt through cbor")
		}

	}
}

func TestFilRoundTrip(t *testing.T) {
	testValues := []string{
		"0", "1", "1.001", "100.10001", "101100", "5000.01", "5000",
	}

	for _, v := range testValues {
		fval, err := ParseTokenAmount(v)
		if err != nil {
			t.Fatal(err)
		}

		if fval.String() != v {
			t.Fatal("mismatch in values!", v, fval.String())
		}
	}
}

func TestFromInt(t *testing.T) {
	a := uint64(999)
	ta := FromInt(a)
	b := big.NewInt(999)
	tb := TokenAmount{Int: b}
	assert.True(t, ta.Equals(tb))
	assert.Equal(t, "0.000000000000000999", ta.String())
}

func TestTokenAmount_MarshalUnmarshalJSON(t *testing.T) {
	ta := FromInt(54321)
	tb := FromInt(0)

	res, err := ta.MarshalJSON()
	require.NoError(t, err)
	assert.Equal(t, "\"0.000000000000054321\"", string(res[:]))

	require.NoError(t, tb.UnmarshalJSON(res))
	assert.Equal(t, ta, tb)
}

func TestOperations(t *testing.T) {
	testCases := []struct{
		name string
		f func(TokenAmount, TokenAmount) TokenAmount
		expected TokenAmount
	}{
		{name: "Sum", f: Add, expected:FromInt(7000)},
		{name: "Sub", f: Sub, expected:FromInt(3000)},
		{name: "Mul", f: Mul, expected: FromInt(10000000)},
		{name: "Div", f: Div, expected: FromInt(2)},
		{name: "Mod", f: Mod, expected: FromInt(1000)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ta := TokenAmount{ Int: big.NewInt(5000)}
			tb := TokenAmount{ Int: big.NewInt(2000)}
			assert.Equal(t, testCase.expected, testCase.f(ta, tb))
		})
	}

	ta := FromInt(5000)
	tb := FromInt(2000)
	tc := FromInt(2000)
	assert.Equal(t, Cmp(ta, tb), 1)
	assert.Equal(t, Cmp(tb, ta), -1)
	assert.Equal(t, Cmp(tb, tc), 0)
	assert.True(t, ta.GreaterThan(tb))
	assert.False(t, ta.LessThan(tb))
	assert.True(t, tb.Equals(tc))
}

func TestTokenAmount_Scan(t *testing.T) {
	ta := FromFil(0)

	err := ta.Scan(54321)
	assert.EqualError(t, err, "non-string types unsupported: int")

	err = ta.Scan(int64(54321))
	require.NoError(t, err)
	assert.Equal(t, FromInt(54321), ta)

	err = ta.Scan("54321")
	require.NoError(t, err)
	assert.Equal(t, FromInt(54321), ta)
}

func TestParseTokenAmount(t *testing.T) {
	res, err := ParseTokenAmount("123.45")
	require.NoError(t, err)
	assert.Equal(t, "123.45", res.String())

	res, err = ParseTokenAmount("12345")
	assert.Equal(t, FromFil(12345), res)

	_, err = ParseTokenAmount("123badnum")
	assert.EqualError(t, err, "failed to parse \"123badnum\" as a decimal number")

	_, err = ParseTokenAmount("0.0000000000000000000000003")
	assert.EqualError(t, err, "invalid FIL value: \"0.0000000000000000000000003\"")
}