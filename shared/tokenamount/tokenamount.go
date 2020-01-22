package tokenamount

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/filecoin-project/go-fil-markets/shared/params"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/polydawn/refmt/obj/atlas"

	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

// BigIntMaxSerializedLen is the maximum number of bytes a big int can use when
// serialized to CBOR
const BigIntMaxSerializedLen = 128 // is this big enough? or too big?

// TotalFilecoinAmount is all filecoin in the system, as a token amount
var TotalFilecoinAmount = FromFil(params.TotalFilecoin)

func init() {
	cbor.RegisterCborType(atlas.BuildEntry(TokenAmount{}).Transform().
		TransformMarshal(atlas.MakeMarshalTransformFunc(
			func(i TokenAmount) ([]byte, error) {
				return i.cborBytes(), nil
			})).
		TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
			func(x []byte) (TokenAmount, error) {
				return fromCborBytes(x)
			})).
		Complete())
}

// Empty is an empty token
var Empty = TokenAmount{}

// TokenAmount is an amount of filecoin, represented as a big int
type TokenAmount struct {
	*big.Int
}

// FromInt creates a token amount from an integer
func FromInt(i uint64) TokenAmount {
	return TokenAmount{big.NewInt(0).SetUint64(i)}
}

// FromFil creates a token amount from a whole amount of filecoin
func FromFil(i uint64) TokenAmount {
	return Mul(FromInt(i), FromInt(params.FilecoinPrecision))
}

// FromBytes creates a token amount from a byte string
func FromBytes(b []byte) TokenAmount {
	i := big.NewInt(0).SetBytes(b)
	return TokenAmount{i}
}

// FromString creates a token amount from a string representation of a big int
func FromString(s string) (TokenAmount, error) {
	v, ok := big.NewInt(0).SetString(s, 10)
	if !ok {
		return TokenAmount{}, fmt.Errorf("failed to parse string as a big int")
	}

	return TokenAmount{v}, nil
}

// Mul multiples two token amounts
func Mul(a, b TokenAmount) TokenAmount {
	zero := big.NewInt(0)
	return TokenAmount{zero.Mul(a.Int, b.Int)}
}

// Div divides two token amounts
func Div(a, b TokenAmount) TokenAmount {
	return TokenAmount{big.NewInt(0).Div(a.Int, b.Int)}
}

// Mod computes the remainder of two token amounts
func Mod(a, b TokenAmount) TokenAmount {
	return TokenAmount{big.NewInt(0).Mod(a.Int, b.Int)}
}

// Add adds two token amounts together
func Add(a, b TokenAmount) TokenAmount {
	return TokenAmount{big.NewInt(0).Add(a.Int, b.Int)}
}

// Sub subtracts the second token amount from the first
func Sub(a, b TokenAmount) TokenAmount {
	return TokenAmount{big.NewInt(0).Sub(a.Int, b.Int)}
}

// Cmp compares two token amounts (for sorting)
func Cmp(a, b TokenAmount) int {
	return a.Int.Cmp(b.Int)
}

// Nil is true if there is no underlying token amount
func (ta TokenAmount) Nil() bool {
	return ta.Int == nil
}

// LessThan returns true if ta < o
func (ta TokenAmount) LessThan(o TokenAmount) bool {
	return Cmp(ta, o) < 0
}

// GreaterThan returns true if ta > o
func (ta TokenAmount) GreaterThan(o TokenAmount) bool {
	return Cmp(ta, o) > 0
}

// Equals returns true if ta == o
func (ta TokenAmount) Equals(o TokenAmount) bool {
	return Cmp(ta, o) == 0
}

// MarshalJSON converts a token amount to a json string
func (ta *TokenAmount) MarshalJSON() ([]byte, error) {
	return json.Marshal(ta.String())
}

// UnmarshalJSON decodes a token amount from json
func (ta *TokenAmount) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	res, err := ParseTokenAmount(s)
	if err != nil { return err }
	ta.Set(res.Int)
	return nil
}

// Scan sets a token amount value from any type
func (ta *TokenAmount) Scan(value interface{}) error {
	switch value := value.(type) {
	case string:
		i, ok := big.NewInt(0).SetString(value, 10)
		if !ok {
			if value == "<nil>" {
				return nil
			}
			return xerrors.Errorf("failed to parse bigint string: '%s'", value)
		}

		ta.Int = i

		return nil
	case int64:
		ta.Int = big.NewInt(value)
		return nil
	default:
		return xerrors.Errorf("non-string types unsupported: %T", value)
	}
}

func (ta *TokenAmount) cborBytes() []byte {
	if ta.Int == nil {
		return []byte{}
	}

	switch {
	case ta.Sign() > 0:
		return append([]byte{0}, ta.Bytes()...)
	case ta.Sign() < 0:
		return append([]byte{1}, ta.Bytes()...)
	default: //  ta.Sign() == 0:
		return []byte{}
	}
}

func fromCborBytes(buf []byte) (TokenAmount, error) {
	if len(buf) == 0 {
		return FromInt(0), nil
	}

	var negative bool
	switch buf[0] {
	case 0:
		negative = false
	case 1:
		negative = true
	default:
		return Empty, fmt.Errorf("big int prefix should be either 0 or 1, got %d", buf[0])
	}

	i := big.NewInt(0).SetBytes(buf[1:])
	if negative {
		i.Neg(i)
	}

	return TokenAmount{i}, nil
}

// MarshalCBOR encodes a TokenAmount to a CBOR byte array
func (ta *TokenAmount) MarshalCBOR(w io.Writer) error {
	if ta.Int == nil {
		zero := FromInt(0)
		return zero.MarshalCBOR(w)
	}

	enc := ta.cborBytes()

	header := cbg.CborEncodeMajorType(cbg.MajByteString, uint64(len(enc)))
	if _, err := w.Write(header); err != nil {
		return err
	}

	if _, err := w.Write(enc); err != nil {
		return err
	}

	return nil
}

// UnmarshalCBOR decodes a TokenAmount from a CBOR byte array
func (ta *TokenAmount) UnmarshalCBOR(br io.Reader) error {
	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if maj != cbg.MajByteString {
		return fmt.Errorf("cbor input for fil big int was not a byte string (%x)", maj)
	}

	if extra == 0 {
		ta.Int = big.NewInt(0)
		return nil
	}

	if extra > BigIntMaxSerializedLen {
		return fmt.Errorf("big integer byte array too long")
	}

	buf := make([]byte, extra)
	if _, err := io.ReadFull(br, buf); err != nil {
		return err
	}

	i, err := fromCborBytes(buf)
	if err != nil {
		return err
	}

	*ta = i

	return nil
}

// String outputs the token amount as a readable string
func (ta TokenAmount) String() string {
	r := new(big.Rat).SetFrac(ta.Int, big.NewInt(params.FilecoinPrecision))
	if r.Sign() == 0 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(r.FloatString(18), "0"), ".")
}

// Format converts a token amount to a string and then formats according to the
// given format
func (ta TokenAmount) Format(s fmt.State, ch rune) {
	switch ch {
	case 's', 'v':
		fmt.Fprint(s, ta.String())
	default:
		ta.Int.Format(s, ch)
	}
}

// ParseTokenAmount parses a token amount from a formatted string
func ParseTokenAmount(s string) (TokenAmount, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return TokenAmount{}, fmt.Errorf("failed to parse %q as a decimal number", s)
	}

	r = r.Mul(r, big.NewRat(params.FilecoinPrecision, 1))
	if !r.IsInt() {
		return TokenAmount{}, fmt.Errorf("invalid FIL value: %q", s)
	}

	return TokenAmount{r.Num()}, nil
}
