package shared

import (
	"bytes"
	"fmt"
	"io"
	"reflect"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec/dagcbor"
	"github.com/ipld/go-ipld-prime/datamodel"
	basicnode "github.com/ipld/go-ipld-prime/node/basic"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/go-state-types/crypto"
)

type typeWithBindnodeSchema interface {
	BindnodeSchema() string
}

// We use the prototype map to store TypedPrototype and Type information
// mapped against Go type names so we only have to run the schema parse once.
// Currently there's not much additional benefit of storing this but there
// may be in the future.
var prototype map[string]schema.TypedPrototype = make(map[string]schema.TypedPrototype)

// The set of converters that we need to turn some Filecoin Go types into plain
// values that bindnode can serialize for us. Mostly []byte forms, but the
// Any converter is used to give us the ability to retain cbor-gen compatibility
// on a Deferred.
var bindnodeOptions = []bindnode.Option{
	bindnode.TypedBytesConverter(&big.Int{}, bigIntFromBytes, bigIntToBytes),
	bindnode.TypedBytesConverter(&abi.TokenAmount{}, tokenAmountFromBytes, tokenAmountToBytes),
	bindnode.TypedBytesConverter(&address.Address{}, addressFromBytes, addressToBytes),
	bindnode.TypedBytesConverter(&crypto.Signature{}, signatureFromBytes, signatureToBytes),
	bindnode.TypedAnyConverter(&CborGenCompatibleNode{}, cborGenCompatibleNodeFromAny, cborGenCompatibleNodeToAny),
}

func typeName(ptrValue interface{}) string {
	val := reflect.ValueOf(ptrValue).Type()
	for val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	return val.Name()
}

// lookup of cached TypedPrototype (and therefore Type) for a Go type, if not
// found, initial parse and setup and caching of the TypedPrototype will happen
func prototypeFor(typeName string, ptrType interface{}) (schema.TypedPrototype, error) {
	proto, ok := prototype[typeName]
	if !ok {
		schemaType, err := schemaTypeFor(typeName, ptrType)
		if err != nil {
			return nil, err
		}
		if schemaType == nil {
			return nil, fmt.Errorf("could not find type [%s] in schema", typeName)
		}
		proto = bindnode.Prototype(ptrType, schemaType, bindnodeOptions...)
		prototype[typeName] = proto
	}
	return proto, nil
}

// load the schema for a Go type, which must have a BindnodeSchema() method
// attached to it
func schemaTypeFor(typeName string, ptrType interface{}) (schema.Type, error) {
	tws, ok := ptrType.(typeWithBindnodeSchema)
	if !ok {
		return nil, fmt.Errorf("attempted to perform IPLD mapping on type without BindnodeSchema(): %T", ptrType)
	}
	schema := tws.BindnodeSchema()
	typeSystem, err := ipld.LoadSchemaBytes([]byte(schema))
	if err != nil {
		return nil, err
	}
	schemaType := typeSystem.TypeByName(typeName)
	if schemaType == nil {
		if !ok {
			return nil, fmt.Errorf("schema for [%T] does not contain that named type [%s]", ptrType, typeName)
		}
	}
	return schemaType, nil
}

// FromReader deserializes DAG-CBOR from a Reader and instantiates the Go type
// that's provided as a pointer via the ptrValue argument.
func TypeFromReader(r io.Reader, ptrValue interface{}) (interface{}, error) {
	name := typeName(ptrValue)
	proto, err := prototypeFor(name, ptrValue)
	if err != nil {
		return nil, err
	}
	node, err := ipld.DecodeStreamingUsingPrototype(r, dagcbor.Decode, proto)
	if err != nil {
		return nil, err
	}
	typ := bindnode.Unwrap(node)
	return typ, nil
}

// FromNode converts an datamodel.Node into an appropriate Go type that's provided as
// a pointer via the ptrValue argument
func TypeFromNode(node datamodel.Node, ptrValue interface{}) (interface{}, error) {
	name := typeName(ptrValue)
	proto, err := prototypeFor(name, ptrValue)
	if err != nil {
		return nil, err
	}
	if tn, ok := node.(schema.TypedNode); ok {
		node = tn.Representation()
	}
	builder := proto.Representation().NewBuilder()
	err = builder.AssignNode(node)
	if err != nil {
		return nil, err
	}
	typ := bindnode.Unwrap(builder.Build())
	return typ, nil
}

// ToNode converts a Go type that's provided as a pointer via the ptrValue
// argument to an schema.TypedNode.
func TypeToNode(ptrValue interface{}) (schema.TypedNode, error) {
	name := typeName(ptrValue)
	proto, err := prototypeFor(name, ptrValue)
	if err != nil {
		return nil, err
	}
	return bindnode.Wrap(ptrValue, proto.Type(), bindnodeOptions...), err
}

// TypeToWriter is a utility method that serializes a Go type that's provided as a
// pointer via the ptrValue argument as DAG-CBOR to a Writer
func TypeToWriter(ptrValue interface{}, w io.Writer) error {
	node, err := TypeToNode(ptrValue)
	if err != nil {
		return err
	}
	return ipld.EncodeStreaming(w, node, dagcbor.Encode)
}

// CborGenCompatibleNode is for cbor-gen / go-ipld-prime compatibility, to
// replace Deferred types that are used to represent datamodel.Nodes.
// This shouldn't be used as a pointer (nullable/optional) as it can consume
// "Null" tokens and therefore be a Null. Instead, use CborGenCompatibleNode#IsNull to
// check for null status.
type CborGenCompatibleNode struct {
	Node datamodel.Node
}

func (sn CborGenCompatibleNode) IsNull() bool {
	return sn.Node == nil || sn.Node == datamodel.Null
}

// UnmarshalCBOR is for cbor-gen compatibility
func (sn *CborGenCompatibleNode) UnmarshalCBOR(r io.Reader) error {
	// use cbg.Deferred.UnmarshalCBOR to figure out how much to pull
	def := cbg.Deferred{}
	if err := def.UnmarshalCBOR(r); err != nil {
		return err
	}
	// convert it to a Node
	na := basicnode.Prototype.Any.NewBuilder()
	if err := dagcbor.Decode(na, bytes.NewReader(def.Raw)); err != nil {
		return err
	}
	sn.Node = na.Build()
	return nil
}

// MarshalCBOR is for cbor-gen compatibility
func (sn *CborGenCompatibleNode) MarshalCBOR(w io.Writer) error {
	node := datamodel.Null
	if sn != nil && sn.Node != nil {
		node = sn.Node
		if tn, ok := node.(schema.TypedNode); ok {
			node = tn.Representation()
		}
	}
	return dagcbor.Encode(node, w)
}

// --- Go type converter functions for bindnode for common Filecoin data types

func tokenAmountFromBytes(b []byte) (interface{}, error) {
	return bigIntFromBytes(b)
}

func bigIntFromBytes(b []byte) (interface{}, error) {
	if len(b) == 0 {
		return big.NewInt(0), nil
	}
	return big.FromBytes(b)
}

func tokenAmountToBytes(iface interface{}) ([]byte, error) {
	return bigIntToBytes(iface)
}

func bigIntToBytes(iface interface{}) ([]byte, error) {
	bi, ok := iface.(*big.Int)
	if !ok {
		return nil, fmt.Errorf("expected *big.Int value")
	}
	if bi == nil || bi.Int == nil {
		*bi = big.Zero()
	}
	return bi.Bytes()
}

func cborGenCompatibleNodeFromAny(node datamodel.Node) (interface{}, error) {
	return &CborGenCompatibleNode{Node: node}, nil
}

func cborGenCompatibleNodeToAny(iface interface{}) (datamodel.Node, error) {
	sn, ok := iface.(*CborGenCompatibleNode)
	if !ok {
		return nil, fmt.Errorf("expected *CborGenCompatibleNode value")
	}
	if sn.Node == nil {
		return datamodel.Null, nil
	}
	return sn.Node, nil
}

func addressFromBytes(b []byte) (interface{}, error) {
	return address.NewFromBytes(b)
}

func addressToBytes(iface interface{}) ([]byte, error) {
	addr, ok := iface.(*address.Address)
	if !ok {
		return nil, fmt.Errorf("expected *CborGenCompatibleNode value")
	}
	return addr.Bytes(), nil
}

// Signature is a byteprefix union
func signatureFromBytes(b []byte) (interface{}, error) {
	if len(b) > crypto.SignatureMaxLength {
		return nil, fmt.Errorf("string too long")
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("string empty")
	}
	var s crypto.Signature
	switch crypto.SigType(b[0]) {
	default:
		return nil, fmt.Errorf("invalid signature type in cbor input: %d", b[0])
	case crypto.SigTypeSecp256k1:
		s.Type = crypto.SigTypeSecp256k1
	case crypto.SigTypeBLS:
		s.Type = crypto.SigTypeBLS
	}
	s.Data = b[1:]
	return &s, nil
}

func signatureToBytes(iface interface{}) ([]byte, error) {
	s, ok := iface.(*crypto.Signature)
	if !ok {
		return nil, fmt.Errorf("expected *CborGenCompatibleNode value")
	}
	ba := append([]byte{byte(s.Type)}, s.Data...)
	return ba, nil
}
