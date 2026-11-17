package esim

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// TLV represents a parsed BER-TLV element.
type TLV struct {
	Tag      uint32
	TagBytes []byte
	Value    []byte
	Children []*TLV
}

// ParseTLV parses BER-TLV data into a hierarchical tree.
func ParseTLV(data []byte) ([]*TLV, error) {
	var nodes []*TLV
	offset := 0

	for offset < len(data) {
		if data[offset] == 0x00 || data[offset] == 0xFF {
			offset++
			continue
		}

		start := offset
		var tag uint32
		b := data[offset]
		offset++
		tag = uint32(b)

		// Multibyte tag
		if (b & 0x1F) == 0x1F {
			for offset < len(data) {
				tb := data[offset]
				offset++
				tag = (tag << 8) | uint32(tb)
				if (tb & 0x80) == 0 {
					break
				}
			}
		}

		tagBytes := data[start:offset]

		if offset >= len(data) {
			break
		}

		// Length decoding
		lenByte := data[offset]
		offset++
		var length int

		if (lenByte & 0x80) == 0 {
			length = int(lenByte)
		} else {
			numBytes := int(lenByte & 0x7F)
			if numBytes > 4 || offset+numBytes > len(data) {
				return nil, errors.New("invalid BER-TLV length")
			}
			for i := 0; i < numBytes; i++ {
				length = (length << 8) | int(data[offset])
				offset++
			}
		}

		if offset+length > len(data) {
			// Trimmed or invalid length
			length = len(data) - offset
		}

		val := data[offset : offset+length]
		offset += length

		node := &TLV{
			Tag:      tag,
			TagBytes: tagBytes,
			Value:    val,
		}

		// If constructed tag (bit 6 of first tag byte is 1), try parsing children
		if (tagBytes[0] & 0x20) == 0x20 {
			children, err := ParseTLV(val)
			if err == nil && len(children) > 0 {
				node.Children = children
			}
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// FindFirst searches the TLV tree recursively for the first element with the given tag.
func (t *TLV) FindFirst(tag uint32) *TLV {
	if t.Tag == tag {
		return t
	}
	for _, child := range t.Children {
		if res := child.FindFirst(tag); res != nil {
			return res
		}
	}
	return nil
}

// FindAll searches the direct or indirect children for all elements matching the tag.
func (t *TLV) FindAll(tag uint32) []*TLV {
	var list []*TLV
	if t.Tag == tag {
		list = append(list, t)
	}
	for _, child := range t.Children {
		list = append(list, child.FindAll(tag)...)
	}
	return list
}

// DecodeBCD converts inverted BCD bytes (standard SIM ICCID/EID format) to string.
// E.g. [0x98, 0x68, 0x10] -> "898601"
func DecodeBCD(b []byte) string {
	var sb strings.Builder
	for _, byteVal := range b {
		low := byteVal & 0x0F
		high := (byteVal >> 4) & 0x0F
		if low <= 9 {
			sb.WriteString(fmt.Sprintf("%d", low))
		}
		if high <= 9 {
			sb.WriteString(fmt.Sprintf("%d", high))
		}
	}
	return sb.String()
}

// EncodeBCD converts a string to inverted BCD bytes.
func EncodeBCD(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		s += "F"
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		c1 := s[i]
		c2 := s[i+1]
		d1, err1 := parseHexNibble(c1)
		d2, err2 := parseHexNibble(c2)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid BCD character in %s", s)
		}
		out[i/2] = (d2 << 4) | d1
	}
	return out, nil
}

func parseHexNibble(c byte) (byte, error) {
	if c >= '0' && c <= '9' {
		return c - '0', nil
	}
	if c >= 'a' && c <= 'f' {
		return c - 'a' + 10, nil
	}
	if c >= 'A' && c <= 'F' {
		return c - 'A' + 10, nil
	}
	return 0, fmt.Errorf("invalid hex: %c", c)
}

// HexBytes returns formatted hex string.
func HexBytes(b []byte) string {
	return strings.ToUpper(hex.EncodeToString(b))
}
