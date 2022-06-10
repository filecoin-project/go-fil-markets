package bindnodeutils

import (
	"fmt"
	"io"
	"reflect"

	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/codec"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/ipld/go-ipld-prime/node/bindnode"
	"github.com/ipld/go-ipld-prime/schema"
)

// We use the prototype map to store TypedPrototype and bindnode options mapped
// against the Go type so we only have to run the schema parse once and we
// can be sure to use the right options (converters) whenever operating on
// this type.

type prototypeData struct {
	proto   schema.TypedPrototype
	options []bindnode.Option
}

var prototype map[reflect.Type]prototypeData = make(map[reflect.Type]prototypeData)

func typeOf(ptrValue interface{}) reflect.Type {
	val := reflect.ValueOf(ptrValue).Type()
	for val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	return val
}

// lookup of cached TypedPrototype (and therefore Type) for a Go type, if not
// found, initial parse and setup and caching of the TypedPrototype will happen
func prototypeDataFor(ptrType interface{}) prototypeData {
	typ := typeOf(ptrType)
	proto, ok := prototype[typ]
	if !ok {
		panic(fmt.Sprintf("bindnode utils: type has not been registered: %s", typ.Name()))
	}
	return proto
}

// RegisterType registers ptrType with schema such that it can be wrapped and
// unwrapped without needing the schema, Type, or TypedPrototype.
// Typically the typeName will match the Go type name, but it can be whatever
// is defined in the schema for the type being registered.
//
// May panic if the schema is invalid or the type doesn't match the schema.
func RegisterType(ptrType interface{}, schema string, typeName string, options ...bindnode.Option) {
	typ := typeOf(ptrType)
	if _, ok := prototype[typ]; ok {
		panic(fmt.Sprintf("bindnode utils: type already registered: %s", typ.Name()))
	}
	typeSystem, err := ipld.LoadSchemaBytes([]byte(schema))
	if err != nil {
		panic(fmt.Sprintf("bindnode utils: failed to load schema: %s", err.Error()))
	}
	schemaType := typeSystem.TypeByName(typeName)
	if schemaType == nil {
		panic(fmt.Sprintf("bindnode utils: schema for [%T] does not contain that named type [%s]", ptrType, typ.Name()))
	}
	prototype[typ] = prototypeData{
		bindnode.Prototype(ptrType, schemaType, options...),
		options,
	}
}

// IsRegistered can be used to determine if the type has already been registered
// within this current application instance.
// Using RegisterType on an already registered type will cause a panic, so where
// this may be the case, IsRegistered can be used to check.
func IsRegistered(ptrType interface{}) bool {
	_, ok := prototype[typeOf(ptrType)]
	return ok
}

// TypeFromReader deserializes DAG-CBOR from a Reader and instantiates the Go
// type that's provided as a pointer via the ptrValue argument.
func TypeFromReader(r io.Reader, ptrValue interface{}, decoder codec.Decoder) (interface{}, error) {
	protoData := prototypeDataFor(ptrValue)
	node, err := ipld.DecodeStreamingUsingPrototype(r, decoder, protoData.proto)
	if err != nil {
		return nil, err
	}
	typ := bindnode.Unwrap(node)
	return typ, nil
}

// TypeFromNode converts an datamodel.Node into an appropriate Go type that's
// provided as a pointer via the ptrValue argument
func TypeFromNode(node datamodel.Node, ptrValue interface{}) (interface{}, error) {
	protoData := prototypeDataFor(ptrValue)
	if tn, ok := node.(schema.TypedNode); ok {
		node = tn.Representation()
	}
	builder := protoData.proto.Representation().NewBuilder()
	err := builder.AssignNode(node)
	if err != nil {
		return nil, err
	}
	typ := bindnode.Unwrap(builder.Build())
	return typ, nil
}

// TypeToNode converts a Go type that's provided as a pointer via the ptrValue
// argument to an schema.TypedNode.
func TypeToNode(ptrValue interface{}) schema.TypedNode {
	protoData := prototypeDataFor(ptrValue)
	return bindnode.Wrap(ptrValue, protoData.proto.Type(), protoData.options...)
}

// TypeToWriter is a utility method that serializes a Go type that's provided as a
// pointer via the ptrValue argument as DAG-CBOR to a Writer
func TypeToWriter(ptrValue interface{}, w io.Writer, encoder codec.Encoder) error {
	return ipld.EncodeStreaming(w, TypeToNode(ptrValue), encoder)
}
