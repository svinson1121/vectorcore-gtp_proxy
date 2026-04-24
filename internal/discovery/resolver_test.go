package discovery

import (
	"encoding/binary"
	"testing"
)

func TestCandidateFQDNs(t *testing.T) {
	got := candidateFQDNs("topon", "epc.example.net")
	if len(got) != 2 || got[0] != "topon" || got[1] != "topon.epc.example.net" {
		t.Fatalf("unexpected candidate FQDNs %#v", got)
	}
}

func TestServiceMatches(t *testing.T) {
	if !serviceMatches("x-3gpp-pgw:x-s8-gtp", "x-3gpp-pgw") {
		t.Fatal("expected service match")
	}
	if serviceMatches("x-3gpp-pgw:x-s8-gtp", "x-3gpp-ggsn") {
		t.Fatal("unexpected service match")
	}
}

func TestParseNAPTRResponse(t *testing.T) {
	msg := makeNAPTRResponse(t,
		"topon.s8.pgw.epc.example.net",
		naptrRecord{
			Order:       10,
			Preference:  20,
			Flags:       "S",
			Service:     "x-3gpp-pgw:x-s8-gtp",
			Replacement: "_x-3gpp-pgw._udp.pgw.example.net",
		},
	)

	records, id, err := parseNAPTRResponse(msg)
	if err != nil {
		t.Fatalf("parseNAPTRResponse() error = %v", err)
	}
	if id != 0x1234 {
		t.Fatalf("unexpected response ID %#x", id)
	}
	if len(records) != 1 {
		t.Fatalf("unexpected record count %d", len(records))
	}
	record := records[0]
	if record.Order != 10 || record.Preference != 20 || record.Flags != "S" || record.Service != "x-3gpp-pgw:x-s8-gtp" || record.Replacement != "_x-3gpp-pgw._udp.pgw.example.net" {
		t.Fatalf("unexpected NAPTR record %+v", record)
	}
}

func makeNAPTRResponse(t *testing.T, qname string, record naptrRecord) []byte {
	t.Helper()

	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], 0x1234)
	binary.BigEndian.PutUint16(header[2:4], 0x8180)
	binary.BigEndian.PutUint16(header[4:6], 1)
	binary.BigEndian.PutUint16(header[6:8], 1)

	name, err := encodeDNSName(qname)
	if err != nil {
		t.Fatalf("encodeDNSName(question) error = %v", err)
	}
	msg := append([]byte{}, header...)
	msg = append(msg, name...)
	question := make([]byte, 4)
	binary.BigEndian.PutUint16(question[0:2], dnsTypeNAPTR)
	binary.BigEndian.PutUint16(question[2:4], dnsClassIN)
	msg = append(msg, question...)

	answerName, err := encodeDNSName(qname)
	if err != nil {
		t.Fatalf("encodeDNSName(answer) error = %v", err)
	}
	msg = append(msg, answerName...)
	answerHeader := make([]byte, 10)
	binary.BigEndian.PutUint16(answerHeader[0:2], dnsTypeNAPTR)
	binary.BigEndian.PutUint16(answerHeader[2:4], dnsClassIN)
	binary.BigEndian.PutUint32(answerHeader[4:8], 60)

	rdata := make([]byte, 4)
	binary.BigEndian.PutUint16(rdata[0:2], record.Order)
	binary.BigEndian.PutUint16(rdata[2:4], record.Preference)
	rdata = appendCharString(rdata, record.Flags)
	rdata = appendCharString(rdata, record.Service)
	rdata = appendCharString(rdata, record.Regexp)
	replacement, err := encodeDNSName(record.Replacement)
	if err != nil {
		t.Fatalf("encodeDNSName(replacement) error = %v", err)
	}
	rdata = append(rdata, replacement...)
	binary.BigEndian.PutUint16(answerHeader[8:10], uint16(len(rdata)))
	msg = append(msg, answerHeader...)
	msg = append(msg, rdata...)
	return msg
}

func appendCharString(buf []byte, value string) []byte {
	buf = append(buf, byte(len(value)))
	buf = append(buf, value...)
	return buf
}
