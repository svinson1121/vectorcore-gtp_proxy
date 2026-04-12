package gtpc

import (
	"net"
	"testing"
)

func TestParseAndRewriteFTEIDs(t *testing.T) {
	payload := MarshalIEs([]IE{
		{Type: ieTypeIMSI, Payload: []byte{0x00, 0x01, 0x01, 0x21, 0x43, 0x65, 0x87, 0xf9}},
		{Type: ieTypeAPN, Payload: []byte{8, 'i', 'n', 't', 'e', 'r', 'n', 'e', 't'}},
		{Type: ieTypeFTEID, Payload: MarshalFTEID(FTEID{
			InterfaceType: 10,
			TEID:          1234,
			IPv4:          net.IPv4(10, 0, 0, 1),
		})},
	})

	packet := Packet{
		Flags:       0x48,
		MessageType: messageTypeCreateSessionRequest,
		HasTEID:     true,
		TEID:        0,
		Sequence:    99,
		Payload:     payload,
	}

	parsed, err := ParsePacket(packet.Marshal())
	if err != nil {
		t.Fatalf("ParsePacket() error = %v", err)
	}
	if parsed.MessageType != messageTypeCreateSessionRequest {
		t.Fatalf("unexpected message type %d", parsed.MessageType)
	}

	imsi, apn, teid, err := ExtractCreateSessionMetadata(parsed.Payload)
	if err != nil {
		t.Fatalf("ExtractCreateSessionMetadata() error = %v", err)
	}
	if apn != "internet" {
		t.Fatalf("unexpected APN %q", apn)
	}
	if teid != 1234 {
		t.Fatalf("unexpected visited TEID %d", teid)
	}
	if imsi == "" {
		t.Fatal("expected IMSI to be decoded")
	}

	rewritten, err := RewriteFTEIDs(parsed.Payload, func(index int, current FTEID) (FTEID, bool, error) {
		current.TEID = 5678
		current.IPv4 = net.IPv4(192, 0, 2, 10)
		return current, true, nil
	})
	if err != nil {
		t.Fatalf("RewriteFTEIDs() error = %v", err)
	}

	fteid, ok := ExtractFirstFTEID(rewritten)
	if !ok {
		t.Fatal("expected rewritten F-TEID")
	}
	if fteid.TEID != 5678 || !fteid.IPv4.Equal(net.IPv4(192, 0, 2, 10)) {
		t.Fatalf("unexpected rewritten F-TEID %#v", fteid)
	}
}

func TestParseAndRewriteIPv6FTEID(t *testing.T) {
	ipv6 := net.ParseIP("2001:db8::1")
	payload := MarshalIEs([]IE{
		{Type: ieTypeFTEID, Payload: MarshalFTEID(FTEID{
			InterfaceType: 10,
			TEID:          4321,
			IPv6:          ipv6,
		})},
	})

	fteid, ok := ExtractFirstFTEID(payload)
	if !ok {
		t.Fatal("expected IPv6 F-TEID")
	}
	if !fteid.IPv6.Equal(ipv6) {
		t.Fatalf("unexpected parsed IPv6 F-TEID %#v", fteid)
	}

	rewritten, err := RewriteFTEIDs(payload, func(index int, current FTEID) (FTEID, bool, error) {
		current.TEID = 8765
		current.IPv6 = net.ParseIP("2001:db8::99")
		return current, true, nil
	})
	if err != nil {
		t.Fatalf("RewriteFTEIDs() IPv6 error = %v", err)
	}

	got, ok := ExtractFirstFTEID(rewritten)
	if !ok {
		t.Fatal("expected rewritten IPv6 F-TEID")
	}
	if got.TEID != 8765 || !got.IPv6.Equal(net.ParseIP("2001:db8::99")) {
		t.Fatalf("unexpected rewritten IPv6 F-TEID %#v", got)
	}
}

func TestParseAndMarshalDualStackFTEID(t *testing.T) {
	original := FTEID{
		InterfaceType: 7,
		TEID:          2468,
		IPv4:          net.IPv4(192, 0, 2, 1),
		IPv6:          net.ParseIP("2001:db8::44"),
	}

	parsed, err := ParseFTEID(MarshalFTEID(original))
	if err != nil {
		t.Fatalf("ParseFTEID() dual-stack error = %v", err)
	}
	if parsed.TEID != original.TEID {
		t.Fatalf("unexpected TEID %d", parsed.TEID)
	}
	if !parsed.IPv4.Equal(original.IPv4) || !parsed.IPv6.Equal(original.IPv6) {
		t.Fatalf("unexpected parsed F-TEID %#v", parsed)
	}
}
