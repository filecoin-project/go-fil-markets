package tokenamount

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/filecoin-project/go-fil-components/shared/params"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/polydawn/refmt/obj/atlas"

	cbg "github.com/whyrusleeping/cbor-gen"
	"golang.org/x/xerrors"
)

const BigIntMaxSerializedLen = 128 // is this big enough? or too big?

var TotalFilecoinInt = FromFil(params.TotalFilecoin)

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

var EmptyInt = TokenAmount{}

type TokenAmount struct {
	*big.Int
}

func NewInt(i uint64) TokenAmount {
	return TokenAmount{big.NewInt(0).SetUint64(i)}
}

func FromFil(i uint64) TokenAmount {
	return BigMul(NewInt(i), NewInt(params.FilecoinPrecision))
}

func BigFromBytes(b []byte) TokenAmount {
	i := big.NewInt(0).SetBytes(b)
	return TokenAmount{i}
}

func BigFromString(s string) (TokenAmount, error) {
	v, ok := big.NewInt(0).SetString(s, 10)
	if !ok {
		return TokenAmount{}, fmt.Errorf("failed to parse string as a big int")
	}

	return TokenAmount{v}, nil
}

func BigMul(a, b TokenAmount) TokenAmount {
	return TokenAmount{big.NewInt(0).Mul(a.Int, b.Int)}
}

func BigDiv(a, b TokenAmount) TokenAmount {
	return TokenAmount{big.NewInt(0).Div(a.Int, b.Int)}
}

func BigMod(a, b TokenAmount) TokenAmount {
	return TokenAmount{big.NewInt(0).Mod(a.Int, b.Int)}
}

func BigAdd(a, b TokenAmount) TokenAmount {
	return TokenAmount{big.NewInt(0).Add(a.Int, b.Int)}
}

func BigSub(a, b TokenAmount) TokenAmount {
	return TokenAmount{big.NewInt(0).Sub(a.Int, b.Int)}
}

func BigCmp(a, b TokenAmount) int {
	return a.Int.Cmp(b.Int)
}

func (bi TokenAmount) Nil() bool {
	return bi.Int == nil
}

// LessThan returns true if bi < o
func (bi TokenAmount) LessThan(o TokenAmount) bool {
	return BigCmp(bi, o) < 0
}

// GreaterThan returns true if bi > o
func (bi TokenAmount) GreaterThan(o TokenAmount) bool {
	return BigCmp(bi, o) > 0
}

// Equals returns true if bi == o
func (bi TokenAmount) Equals(o TokenAmount) bool {
	return BigCmp(bi, o) == 0
}

func (bi *TokenAmount) MarshalJSON() ([]byte, error) {
	return json.Marshal(bi.String())
}

func (bi *TokenAmount) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	i, ok := big.NewInt(0).SetString(s, 10)
	if !ok {
		if string(s) == "<nil>" {
			return nil
		}
		return xerrors.Errorf("failed to parse bigint string: '%s'", string(b))
	}

	bi.Int = i
	return nil
}

func (bi *TokenAmount) Scan(value interface{}) error {
	switch value := value.(type) {
	case string:
		i, ok := big.NewInt(0).SetString(value, 10)
		if !ok {
			if value == "<nil>" {
				return nil
			}
			return xerrors.Errorf("failed to parse bigint string: '%s'", value)
		}

		bi.Int = i

		return nil
	case int64:
		bi.Int = big.NewInt(value)
		return nil
	default:
		return xerrors.Errorf("non-string types unsupported: %T", value)
	}
}

func (bi *TokenAmount) cborBytes() []byte {
	if bi.Int == nil {
		return []byte{}
	}

	switch {
	case bi.Sign() > 0:
		return append([]byte{0}, bi.Bytes()...)
	case bi.Sign() < 0:
		return append([]byte{1}, bi.Bytes()...)
	default: //  bi.Sign() == 0:
		return []byte{}
	}
}

func fromCborBytes(buf []byte) (TokenAmount, error) {
	if len(buf) == 0 {
		return NewInt(0), nil
	}

	var negative bool
	switch buf[0] {
	case 0:
		negative = false
	case 1:
		negative = true
	default:
		return EmptyInt, fmt.Errorf("big int prefix should be either 0 or 1, got %d", buf[0])
	}

	i := big.NewInt(0).SetBytes(buf[1:])
	if negative {
		i.Neg(i)
	}

	return TokenAmount{i}, nil
}

func (bi *TokenAmount) MarshalCBOR(w io.Writer) error {
	if bi.Int == nil {
		zero := NewInt(0)
		return zero.MarshalCBOR(w)
	}

	enc := bi.cborBytes()

	header := cbg.CborEncodeMajorType(cbg.MajByteString, uint64(len(enc)))
	if _, err := w.Write(header); err != nil {
		return err
	}

	if _, err := w.Write(enc); err != nil {
		return err
	}

	return nil
}

func (bi *TokenAmount) UnmarshalCBOR(br io.Reader) error {
	maj, extra, err := cbg.CborReadHeader(br)
	if err != nil {
		return err
	}

	if maj != cbg.MajByteString {
		return fmt.Errorf("cbor input for fil big int was not a byte string (%x)", maj)
	}

	if extra == 0 {
		bi.Int = big.NewInt(0)
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

	*bi = i

	return nil
}

func (f TokenAmount) String() string {
	r := new(big.Rat).SetFrac(f.Int, big.NewInt(params.FilecoinPrecision))
	if r.Sign() == 0 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(r.FloatString(18), "0"), ".")
}

func (f TokenAmount) Format(s fmt.State, ch rune) {
	switch ch {
	case 's', 'v':
		fmt.Fprint(s, f.String())
	default:
		f.Int.Format(s, ch)
	}
}

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
