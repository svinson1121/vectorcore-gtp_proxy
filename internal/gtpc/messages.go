package gtpc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

const (
	messageTypeEchoRequest          = 1
	messageTypeEchoResponse         = 2
	messageTypeCreateSessionRequest = 32
	messageTypeCreateSessionResp    = 33
	messageTypeModifyBearerRequest  = 34
	messageTypeModifyBearerResp     = 35
	messageTypeDeleteSessionRequest = 36
	messageTypeDeleteSessionResp    = 37
	messageTypeCreateBearerRequest  = 95
	messageTypeCreateBearerResp     = 96
	messageTypeUpdateBearerRequest  = 97
	messageTypeUpdateBearerResp     = 98
	messageTypeDeleteBearerRequest  = 99
	messageTypeDeleteBearerResp     = 100
	messageTypeReleaseAccessReq     = 170
	messageTypeReleaseAccessResp    = 171

	ieTypeIMSI          = 1
	ieTypeCause         = 2
	ieTypeRecovery      = 3
	ieTypeAPN           = 71
	ieTypeFTEID         = 87
	ieTypeBearerContext = 93
)

type Packet struct {
	Flags       uint8
	MessageType uint8
	Length      uint16
	HasTEID     bool
	TEID        uint32
	Sequence    uint32
	Payload     []byte
}

type IE struct {
	Type     uint8
	Instance uint8
	Payload  []byte
}

type FTEID struct {
	InterfaceType uint8
	TEID          uint32
	IPv4          net.IP
	IPv6          net.IP
}

func ParsePacket(data []byte) (Packet, error) {
	if len(data) < 8 {
		return Packet{}, fmt.Errorf("packet too short")
	}
	p := Packet{
		Flags:       data[0],
		MessageType: data[1],
		Length:      binary.BigEndian.Uint16(data[2:4]),
		HasTEID:     data[0]&0x08 != 0,
	}
	offset := 4
	if p.HasTEID {
		if len(data) < 12 {
			return Packet{}, fmt.Errorf("packet too short for TEID header")
		}
		p.TEID = binary.BigEndian.Uint32(data[4:8])
		offset = 8
	}
	p.Sequence = uint32(data[offset])<<16 | uint32(data[offset+1])<<8 | uint32(data[offset+2])
	offset += 4
	if len(data) < offset {
		return Packet{}, fmt.Errorf("packet missing payload")
	}
	p.Payload = bytes.Clone(data[offset:])
	return p, nil
}

func (p Packet) Marshal() []byte {
	payloadLen := len(p.Payload) + 4
	offset := 4
	if p.HasTEID {
		payloadLen += 4
		offset = 8
	}
	out := make([]byte, offset+4+len(p.Payload))
	out[0] = p.Flags
	out[1] = p.MessageType
	binary.BigEndian.PutUint16(out[2:4], uint16(payloadLen))
	if p.HasTEID {
		binary.BigEndian.PutUint32(out[4:8], p.TEID)
	}
	out[offset] = byte(p.Sequence >> 16)
	out[offset+1] = byte(p.Sequence >> 8)
	out[offset+2] = byte(p.Sequence)
	copy(out[offset+4:], p.Payload)
	return out
}

func ParseIEs(payload []byte) ([]IE, error) {
	var ies []IE
	for len(payload) > 0 {
		if len(payload) < 4 {
			return nil, fmt.Errorf("IE header truncated")
		}
		ieLen := int(binary.BigEndian.Uint16(payload[1:3]))
		if len(payload) < 4+ieLen {
			return nil, fmt.Errorf("IE payload truncated")
		}
		ies = append(ies, IE{
			Type:     payload[0],
			Instance: payload[3] & 0x0f,
			Payload:  bytes.Clone(payload[4 : 4+ieLen]),
		})
		payload = payload[4+ieLen:]
	}
	return ies, nil
}

func MarshalIEs(ies []IE) []byte {
	var out []byte
	for _, ie := range ies {
		buf := make([]byte, 4+len(ie.Payload))
		buf[0] = ie.Type
		binary.BigEndian.PutUint16(buf[1:3], uint16(len(ie.Payload)))
		buf[3] = ie.Instance & 0x0f
		copy(buf[4:], ie.Payload)
		out = append(out, buf...)
	}
	return out
}

func DecodeAPN(payload []byte) string {
	var labels []string
	for i := 0; i < len(payload); {
		l := int(payload[i])
		i++
		if l == 0 || i+l > len(payload) {
			break
		}
		labels = append(labels, string(payload[i:i+l]))
		i += l
	}
	return strings.Join(labels, ".")
}

func ParseIMSI(payload []byte) string {
	var out strings.Builder
	for _, b := range payload {
		lo := b & 0x0f
		hi := b >> 4
		if lo <= 9 {
			out.WriteByte('0' + lo)
		}
		if hi != 0x0f && hi <= 9 {
			out.WriteByte('0' + hi)
		}
	}
	return out.String()
}

func ParseFTEID(payload []byte) (FTEID, error) {
	if len(payload) < 5 {
		return FTEID{}, fmt.Errorf("F-TEID too short")
	}
	flags := payload[0]
	offset := 5
	out := FTEID{
		InterfaceType: flags & 0x3f,
		TEID:          binary.BigEndian.Uint32(payload[1:5]),
	}
	if flags&0x80 != 0 {
		if len(payload) < offset+4 {
			return FTEID{}, fmt.Errorf("F-TEID missing IPv4 address")
		}
		out.IPv4 = net.IPv4(payload[offset], payload[offset+1], payload[offset+2], payload[offset+3]).To4()
		offset += 4
	}
	if flags&0x40 != 0 {
		if len(payload) < offset+16 {
			return FTEID{}, fmt.Errorf("F-TEID missing IPv6 address")
		}
		out.IPv6 = net.IP(append([]byte(nil), payload[offset:offset+16]...))
		offset += 16
	}
	if out.IPv4 == nil && out.IPv6 == nil {
		return FTEID{}, fmt.Errorf("F-TEID must include an IPv4 or IPv6 address")
	}
	return out, nil
}

func MarshalFTEID(f FTEID) []byte {
	flags := f.InterfaceType & 0x3f
	size := 5
	if ip := f.IPv4.To4(); ip != nil {
		flags |= 0x80
		size += 4
	}
	if ip := f.IPv6.To16(); ip != nil && ip.To4() == nil {
		flags |= 0x40
		size += 16
	}
	out := make([]byte, size)
	out[0] = flags
	binary.BigEndian.PutUint32(out[1:5], f.TEID)
	offset := 5
	if ip := f.IPv4.To4(); ip != nil {
		copy(out[offset:], ip)
		offset += 4
	}
	if ip := f.IPv6.To16(); ip != nil && ip.To4() == nil {
		copy(out[offset:], ip)
	}
	return out
}

func ExtractCreateSessionMetadata(payload []byte) (imsi, apn string, visitedControlTEID uint32, err error) {
	ies, err := ParseIEs(payload)
	if err != nil {
		return "", "", 0, err
	}
	for _, ie := range ies {
		switch ie.Type {
		case ieTypeIMSI:
			if imsi == "" {
				imsi = ParseIMSI(ie.Payload)
			}
		case ieTypeAPN:
			if apn == "" {
				apn = DecodeAPN(ie.Payload)
			}
		case ieTypeFTEID:
			if visitedControlTEID == 0 {
				fteid, parseErr := ParseFTEID(ie.Payload)
				if parseErr == nil && isControlPlaneInterfaceType(fteid.InterfaceType) {
					visitedControlTEID = fteid.TEID
				}
			}
		}
	}
	return imsi, apn, visitedControlTEID, nil
}

func ExtractFirstFTEID(payload []byte) (FTEID, bool) {
	all, err := ExtractAllFTEIDs(payload)
	if err != nil || len(all) == 0 {
		return FTEID{}, false
	}
	return all[0], true
}

func ExtractFirstControlPlaneFTEID(payload []byte) (FTEID, bool) {
	all, err := ExtractAllFTEIDs(payload)
	if err != nil {
		return FTEID{}, false
	}
	for _, fteid := range all {
		if isControlPlaneInterfaceType(fteid.InterfaceType) {
			return fteid, true
		}
	}
	return FTEID{}, false
}

func ExtractCause(payload []byte) (uint8, bool) {
	ies, err := ParseIEs(payload)
	if err != nil {
		return 0, false
	}
	for _, ie := range ies {
		if ie.Type == ieTypeCause && len(ie.Payload) > 0 {
			return ie.Payload[0], true
		}
	}
	return 0, false
}

func ExtractAllFTEIDs(payload []byte) ([]FTEID, error) {
	var out []FTEID
	if err := walkFTEIDs(payload, func(fteid FTEID) error {
		out = append(out, fteid)
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func walkFTEIDs(payload []byte, fn func(FTEID) error) error {
	ies, err := ParseIEs(payload)
	if err != nil {
		return err
	}
	for _, ie := range ies {
		if ie.Type == ieTypeFTEID {
			fteid, err := ParseFTEID(ie.Payload)
			if err == nil {
				if err := fn(fteid); err != nil {
					return err
				}
			}
		}
		if ie.Type == ieTypeBearerContext {
			if err := walkFTEIDs(ie.Payload, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func RewriteFTEIDs(payload []byte, rewrite func(index int, current FTEID) (FTEID, bool, error)) ([]byte, error) {
	index := 0
	return rewritePayload(payload, &index, rewrite)
}

func rewritePayload(payload []byte, index *int, rewrite func(index int, current FTEID) (FTEID, bool, error)) ([]byte, error) {
	ies, err := ParseIEs(payload)
	if err != nil {
		return nil, err
	}
	for i := range ies {
		switch ies[i].Type {
		case ieTypeFTEID:
			current, err := ParseFTEID(ies[i].Payload)
			if err != nil {
				return nil, err
			}
			next, changed, err := rewrite(*index, current)
			if err != nil {
				return nil, err
			}
			*index = *index + 1
			if changed {
				ies[i].Payload = MarshalFTEID(next)
			}
		case ieTypeBearerContext:
			next, err := rewritePayload(ies[i].Payload, index, rewrite)
			if err != nil {
				return nil, err
			}
			ies[i].Payload = next
		}
	}
	return MarshalIEs(ies), nil
}

func EchoResponse(request Packet) Packet {
	return Packet{
		Flags:       0x40,
		MessageType: messageTypeEchoResponse,
		HasTEID:     false,
		Sequence:    request.Sequence,
		Payload: []byte{
			ieTypeRecovery, 0x00, 0x01, 0x00, 0x00,
		},
	}
}

func (f FTEID) HasIPv4() bool {
	return f.IPv4.To4() != nil
}

func (f FTEID) HasIPv6() bool {
	ip := f.IPv6.To16()
	return ip != nil && ip.To4() == nil
}

func isControlPlaneInterfaceType(interfaceType uint8) bool {
	switch interfaceType {
	case 6, 7, 10, 11:
		return true
	default:
		return false
	}
}

func isUserPlaneInterfaceType(interfaceType uint8) bool {
	switch interfaceType {
	case 0, 1, 4, 5:
		return true
	default:
		return false
	}
}
