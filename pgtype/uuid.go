package pgtype

import (
	"bytes"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
)

type UUID struct {
	Bytes [16]byte
	Valid bool

	UUIDDecoderWrapper func(interface{}) UUIDDecoder
	Getter             func(UUID) interface{}
}

func (n *UUID) NewTypeValue() Value {
	return &UUID{
		UUIDDecoderWrapper: n.UUIDDecoderWrapper,
		Getter:             n.Getter,
	}
}

func (n *UUID) TypeName() string {
	return "uuid"
}

func (dst *UUID) setNil() {
	dst.Bytes = [16]byte{}
	dst.Valid = false
}

func (dst *UUID) setByteArray(value [16]byte) {
	dst.Bytes = value
	dst.Valid = true
}

func (dst *UUID) setByteSlice(value []byte) error {
	if value != nil {
		if len(value) != 16 {
			return fmt.Errorf("[]byte must be 16 bytes to convert to UUID: %d", len(value))
		}
		copy(dst.Bytes[:], value)
		dst.Valid = true
	} else {
		dst.setNil()
	}

	return nil
}

func (dst *UUID) setString(value string) error {
	uuid, err := parseUUID(value)
	if err != nil {
		return err
	}
	dst.setByteArray(uuid)
	return nil
}

func (dst *UUID) Set(src interface{}) error {
	if src == nil {
		dst.setNil()
		return nil
	}

	if value, ok := src.(interface{ Get() interface{} }); ok {
		value2 := value.Get()
		if value2 != value {
			return dst.Set(value2)
		}
	}

	switch value := src.(type) {
	case [16]byte:
		dst.setByteArray(value)
	case []byte:
		return dst.setByteSlice(value)
	case string:
		return dst.setString(value)
	case *string:
		if value == nil {
			dst.setNil()
		} else {
			return dst.setString(*value)
		}
	default:
		if originalSrc, ok := underlyingUUIDType(src); ok {
			return dst.Set(originalSrc)
		}
		return fmt.Errorf("cannot convert %v to UUID", value)
	}

	return nil
}

func (dst UUID) Get() interface{} {
	if dst.Getter != nil {
		return dst.Getter(dst)
	}

	if !dst.Valid {
		return nil
	}

	return dst.Bytes
}

type UUIDDecoder interface {
	DecodeUUID(*UUID) error
}

func (src *UUID) AssignTo(dst interface{}) error {
	if d, ok := dst.(UUIDDecoder); ok {
		return d.DecodeUUID(src)
	} else {
		if src.UUIDDecoderWrapper != nil {
			d = src.UUIDDecoderWrapper(dst)
			if d != nil {
				return d.DecodeUUID(src)
			}
		}
	}

	if !src.Valid {
		return NullAssignTo(dst)
	}

	switch v := dst.(type) {
	case *[16]byte:
		*v = src.Bytes
		return nil
	case *[]byte:
		*v = make([]byte, 16)
		copy(*v, src.Bytes[:])
		return nil
	case *string:
		*v = encodeUUID(src.Bytes)
		return nil
	default:
		if nextDst, retry := GetAssignToDstType(v); retry {
			return src.AssignTo(nextDst)
		}
	}

	return nil
}

// parseUUID converts a string UUID in standard form to a byte array.
func parseUUID(src string) (dst [16]byte, err error) {
	switch len(src) {
	case 36:
		src = src[0:8] + src[9:13] + src[14:18] + src[19:23] + src[24:]
	case 32:
		// dashes already stripped, assume valid
	default:
		// assume invalid.
		return dst, fmt.Errorf("cannot parse UUID %v", src)
	}

	buf, err := hex.DecodeString(src)
	if err != nil {
		return dst, err
	}

	copy(dst[:], buf)
	return dst, err
}

// encodeUUID converts a uuid byte array to UUID standard string form.
func encodeUUID(src [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", src[0:4], src[4:6], src[6:8], src[8:10], src[10:16])
}

func (dst *UUID) DecodeText(ci *ConnInfo, src []byte) error {
	if src == nil {
		dst.setNil()
		return nil
	}

	if len(src) != 36 {
		return fmt.Errorf("invalid length for UUID: %v", len(src))
	}

	buf, err := parseUUID(string(src))
	if err != nil {
		return err
	}

	dst.setByteArray(buf)
	return nil
}

func (dst *UUID) DecodeBinary(ci *ConnInfo, src []byte) error {
	if src == nil {
		dst.setNil()
		return nil
	}

	if len(src) != 16 {
		return fmt.Errorf("invalid length for UUID: %v", len(src))
	}

	return dst.setByteSlice(src)
}

func (src UUID) EncodeText(ci *ConnInfo, buf []byte) ([]byte, error) {
	if !src.Valid {
		return nil, nil
	}

	return append(buf, encodeUUID(src.Bytes)...), nil
}

func (src UUID) EncodeBinary(ci *ConnInfo, buf []byte) ([]byte, error) {
	if !src.Valid {
		return nil, nil
	}

	return append(buf, src.Bytes[:]...), nil
}

// Scan implements the database/sql Scanner interface.
func (dst *UUID) Scan(src interface{}) error {
	if src == nil {
		dst.setNil()
		return nil
	}

	switch src := src.(type) {
	case string:
		return dst.DecodeText(nil, []byte(src))
	case []byte:
		srcCopy := make([]byte, len(src))
		copy(srcCopy, src)
		return dst.DecodeText(nil, srcCopy)
	}

	return fmt.Errorf("cannot scan %T", src)
}

// Value implements the database/sql/driver Valuer interface.
func (src UUID) Value() (driver.Value, error) {
	return EncodeValueText(src)
}

func (src UUID) MarshalJSON() ([]byte, error) {
	if !src.Valid {
		return []byte("null"), nil
	}

	var buff bytes.Buffer
	buff.WriteByte('"')
	buff.WriteString(encodeUUID(src.Bytes))
	buff.WriteByte('"')
	return buff.Bytes(), nil
}

func (dst *UUID) UnmarshalJSON(src []byte) error {
	if bytes.Compare(src, []byte("null")) == 0 {
		return dst.Set(nil)
	}
	if len(src) != 38 {
		return fmt.Errorf("invalid length for UUID: %v", len(src))
	}
	return dst.Set(string(src[1 : len(src)-1]))
}