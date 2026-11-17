package sip

import (
	"bytes"
	"net"
	"testing"
)

func TestG711(t *testing.T) {
	// Generate 16-bit linear PCM test samples
	samples := []int16{0, 100, -100, 1000, -1000, 15000, -15000}
	pcm := make([]byte, len(samples)*2)
	for i, s := range samples {
		pcm[2*i] = byte(s)
		pcm[2*i+1] = byte(s >> 8)
	}

	// Test u-law
	ulaw := LinearToUlaw(pcm)
	if len(ulaw) != len(samples) {
		t.Fatalf("expected ulaw length %d, got %d", len(samples), len(ulaw))
	}
	recoveredPCM := UlawToLinear(ulaw)
	if len(recoveredPCM) != len(pcm) {
		t.Fatalf("expected pcm length %d, got %d", len(pcm), len(recoveredPCM))
	}

	// Test a-law
	alaw := LinearToAlaw(pcm)
	if len(alaw) != len(samples) {
		t.Fatalf("expected alaw length %d, got %d", len(samples), len(alaw))
	}
	recoveredAlawPCM := AlawToLinear(alaw)
	if len(recoveredAlawPCM) != len(pcm) {
		t.Fatalf("expected pcm length %d, got %d", len(pcm), len(recoveredAlawPCM))
	}
}

func TestRTPPacket(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03, 0x04}
	pkt := &RTPPacket{
		Header: RTPHeader{
			Version:        2,
			PayloadType:    PayloadPCMU,
			SequenceNumber: 1234,
			Timestamp:      5678,
			SSRC:           9999,
		},
		Payload: payload,
	}

	raw := pkt.Marshal()
	if len(raw) != 12+len(payload) {
		t.Fatalf("unexpected rtp packet length: %d", len(raw))
	}

	parsed, err := ParseRTP(raw)
	if err != nil {
		t.Fatalf("failed to parse rtp: %v", err)
	}

	if parsed.Header.SequenceNumber != 1234 || parsed.Header.Timestamp != 5678 || parsed.Header.PayloadType != PayloadPCMU {
		t.Fatalf("header mismatch: %+v", parsed.Header)
	}
	if !bytes.Equal(parsed.Payload, payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestSIPMessageParse(t *testing.T) {
	raw := "REGISTER sip:192.168.1.100:5060 SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 192.168.1.50:5060;branch=z9hG4bK123456\r\n" +
		"From: <sip:1001@192.168.1.100>;tag=abcde\r\n" +
		"To: <sip:1001@192.168.1.100>\r\n" +
		"Call-ID: 987654321@192.168.1.50\r\n" +
		"CSeq: 1 REGISTER\r\n" +
		"Contact: <sip:1001@192.168.1.50:5060>\r\n" +
		"Expires: 3600\r\n" +
		"Content-Length: 0\r\n\r\n"

	msg, err := ParseSIPMessage([]byte(raw))
	if err != nil {
		t.Fatalf("failed to parse sip message: %v", err)
	}

	if msg.Method != "REGISTER" {
		t.Errorf("expected REGISTER, got %s", msg.Method)
	}
	if msg.GetHeader("From") != "<sip:1001@192.168.1.100>;tag=abcde" {
		t.Errorf("header From mismatch")
	}
	if ParseExpires(msg) != 3600 {
		t.Errorf("expected 3600 expires, got %d", ParseExpires(msg))
	}
}

func TestRegistrarAuth(t *testing.T) {
	reg := NewRegistrar("test_realm", "1001", "secret123")

	nonce := reg.GenerateNonce()
	uri := "sip:192.168.1.100:5060"
	resp := CalculateDigestResponse("1001", "test_realm", "secret123", "REGISTER", uri, nonce, "", "", "")

	req := &SIPMessage{
		Method: "REGISTER",
		URI:    uri,
		Headers: map[string]string{
			"Authorization": `Digest username="1001", realm="test_realm", nonce="` + nonce + `", uri="` + uri + `", response="` + resp + `"`,
		},
	}

	user, ok := reg.Authenticate(req)
	if !ok || user != "1001" {
		t.Fatalf("authentication failed: ok=%v user=%s", ok, user)
	}

	// Test registration
	addr := &net.UDPAddr{IP: net.ParseIP("192.168.1.50"), Port: 5060}
	reg.Register("1001", "<sip:1001@192.168.1.50:5060>", addr, 3600, "call-1", "Linphone/5.2.0")

	active := reg.GetActiveExtensions()
	if len(active) != 1 || active[0].Username != "1001" {
		t.Fatalf("expected 1 active extension, got %d", len(active))
	}

	clients := reg.ListActive()
	if len(clients) != 1 || clients[0].UserAgent != "Linphone/5.2.0" {
		t.Fatalf("expected 1 active client with user agent, got %+v", clients)
	}
}
