package tokenamount_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"

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

// func TestAdd(t *testing.T) {
// 	a := token
// }