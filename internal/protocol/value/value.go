package value

import (
	"fmt"

	"github.com/scouter-project/scouter-server-go/internal/protocol"
)

// Value type codes (must match Java ValueEnum exactly)
const (
	TYPE_NULL           byte = 0
	TYPE_BOOLEAN        byte = 10
	TYPE_DECIMAL        byte = 20
	TYPE_FLOAT          byte = 30
	TYPE_DOUBLE         byte = 40
	TYPE_DOUBLE_SUMMARY byte = 45
	TYPE_LONG_SUMMARY   byte = 46
	TYPE_TEXT           byte = 50
	TYPE_TEXT_HASH      byte = 51
	TYPE_BLOB           byte = 60
	TYPE_IP4ADDR        byte = 61
	TYPE_LIST           byte = 70
	TYPE_ARRAY_INT      byte = 71
	TYPE_ARRAY_FLOAT    byte = 72
	TYPE_ARRAY_TEXT     byte = 73
	TYPE_ARRAY_LONG     byte = 74
	TYPE_MAP            byte = 80
)

type Value interface {
	ValueType() byte
	Write(o *protocol.DataOutputX)
	Read(d *protocol.DataInputX) error
}

func CreateValue(typeCode byte) (Value, error) {
	switch typeCode {
	case TYPE_NULL:
		return &NullValue{}, nil
	case TYPE_BOOLEAN:
		return &BooleanValue{}, nil
	case TYPE_DECIMAL:
		return &DecimalValue{}, nil
	case TYPE_FLOAT:
		return &FloatValue{}, nil
	case TYPE_DOUBLE:
		return &DoubleValue{}, nil
	case TYPE_DOUBLE_SUMMARY:
		return &DoubleSummary{}, nil
	case TYPE_LONG_SUMMARY:
		return &LongSummary{}, nil
	case TYPE_TEXT:
		return &TextValue{}, nil
	case TYPE_TEXT_HASH:
		return &TextHashValue{}, nil
	case TYPE_BLOB:
		return &BlobValue{}, nil
	case TYPE_IP4ADDR:
		return &IP4Value{}, nil
	case TYPE_LIST:
		return &ListValue{}, nil
	case TYPE_ARRAY_INT:
		return &IntArray{}, nil
	case TYPE_ARRAY_FLOAT:
		return &FloatArray{}, nil
	case TYPE_ARRAY_TEXT:
		return &TextArray{}, nil
	case TYPE_ARRAY_LONG:
		return &LongArray{}, nil
	case TYPE_MAP:
		return &MapValue{}, nil
	default:
		return nil, fmt.Errorf("unknown value type code: %d", typeCode)
	}
}

func WriteValue(o *protocol.DataOutputX, v Value) {
	if isNilValue(v) {
		v = &NullValue{}
	}
	o.WriteByte(v.ValueType())
	v.Write(o)
}

// isNilValue checks whether a Value interface is nil or holds a nil pointer.
// In Go, a nil *T stored in an interface is non-nil (v == nil is false),
// but calling methods that access fields will panic with nil pointer dereference.
func isNilValue(v Value) bool {
	if v == nil {
		return true
	}
	// Type-switch on concrete pointer types that may be nil.
	// This avoids reflect and covers all known Value implementations.
	switch v := v.(type) {
	case *MapValue:
		return v == nil
	case *ListValue:
		return v == nil
	case *NullValue:
		return v == nil
	case *BooleanValue:
		return v == nil
	case *DecimalValue:
		return v == nil
	case *FloatValue:
		return v == nil
	case *DoubleValue:
		return v == nil
	case *DoubleSummary:
		return v == nil
	case *LongSummary:
		return v == nil
	case *TextValue:
		return v == nil
	case *TextHashValue:
		return v == nil
	case *BlobValue:
		return v == nil
	case *IP4Value:
		return v == nil
	case *IntArray:
		return v == nil
	case *FloatArray:
		return v == nil
	case *TextArray:
		return v == nil
	case *LongArray:
		return v == nil
	default:
		return false
	}
}

func ReadValue(d *protocol.DataInputX) (Value, error) {
	typeByte, err := d.ReadByte()
	if err != nil {
		return nil, err
	}
	v, err := CreateValue(typeByte)
	if err != nil {
		return nil, err
	}
	err = v.Read(d)
	return v, err
}
