package sip

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"

	"github.com/chenpinetree/SimCallHub/internal/agent/service"
	"github.com/chenpinetree/SimCallHub/internal/agent/store"
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

func TestMultiExtensionBinding(t *testing.T) {
	reg := NewRegistrar("SimCallHub", "", "")
	accounts := map[string]string{
		"1001": "pass1001",
		"1002": "pass1002",
		"1003": "pass1003",
	}
	devices := map[string]string{
		"1001": "dev_imei_1",
		"1002": "dev_imei_2",
		"1003": "", // all devices
	}
	reg.SetAccounts(accounts, devices)

	if d := reg.GetDeviceBinding("1001"); d != "dev_imei_1" {
		t.Fatalf("expected 1001 to bind dev_imei_1, got %s", d)
	}
	if d := reg.GetDeviceBinding("1002"); d != "dev_imei_2" {
		t.Fatalf("expected 1002 to bind dev_imei_2, got %s", d)
	}
	if d := reg.GetDeviceBinding("1003"); d != "" {
		t.Fatalf("expected 1003 to have empty binding, got %s", d)
	}

	// Verify authentication for each extension
	for user, pass := range accounts {
		nonce := reg.GenerateNonce()
		uri := "sip:192.168.1.100:5060"
		resp := CalculateDigestResponse(user, "SimCallHub", pass, "REGISTER", uri, nonce, "", "", "")
		req := &SIPMessage{
			Method: "REGISTER",
			URI:    uri,
			Headers: map[string]string{
				"Authorization": `Digest username="` + user + `", realm="SimCallHub", nonce="` + nonce + `", uri="` + uri + `", response="` + resp + `"`,
			},
		}
		authedUser, ok := reg.Authenticate(req)
		if !ok || authedUser != user {
			t.Fatalf("failed to authenticate %s: ok=%v, authedUser=%s", user, ok, authedUser)
		}
	}
}

func TestRingTimeoutConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.RingTimeout != 20 {
		t.Fatalf("expected default ring timeout 20, got %d", cfg.RingTimeout)
	}

	srv := NewServer(cfg, nil, nil)
	if srv.Status().RingTimeout != 20 {
		t.Fatalf("expected status ring timeout 20, got %d", srv.Status().RingTimeout)
	}

	// Test immediate AI (0)
	srv.UpdateConfig("1001", "12345678", 0)
	if srv.Status().RingTimeout != 0 {
		t.Fatalf("expected ring timeout 0, got %d", srv.Status().RingTimeout)
	}

	// Test disable AI auto-takeover (-1)
	srv.UpdateConfig("1001", "12345678", -1)
	if srv.Status().RingTimeout != -1 {
		t.Fatalf("expected ring timeout -1, got %d", srv.Status().RingTimeout)
	}

	// Test custom timeout (e.g. 30s) via UpdateExtensions
	srv.UpdateExtensions([]ExtensionConfig{{ID: "1", Username: "1001", Password: "pwd"}}, 30)
	if srv.Status().RingTimeout != 30 {
		t.Fatalf("expected ring timeout 30, got %d", srv.Status().RingTimeout)
	}

	// Test 0 via UpdateExtensions
	srv.UpdateExtensions([]ExtensionConfig{{ID: "1", Username: "1001", Password: "pwd"}}, 0)
	if srv.Status().RingTimeout != 0 {
		t.Fatalf("expected ring timeout 0, got %d", srv.Status().RingTimeout)
	}

	// Test -1 via UpdateExtensions
	srv.UpdateExtensions([]ExtensionConfig{{ID: "1", Username: "1001", Password: "pwd"}}, -1)
	if srv.Status().RingTimeout != -1 {
		t.Fatalf("expected ring timeout -1, got %d", srv.Status().RingTimeout)
	}
}

func TestConcurrentConfigAndCallEvents(t *testing.T) {
	srv := NewServer(DefaultConfig(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			srv.UpdateExtensions([]ExtensionConfig{{ID: "1", Username: "1001", Password: "p"}}, n%3-1)
			_ = srv.Status()
		}(i)
		go func() {
			defer wg.Done()
			srv.NotifyCallEvent(service.CallEvent{
				Kind:   service.CallEventRinging,
				Record: store.CallRecord{ID: 1, DeviceID: "dev1", Direction: "in", Number: "10086"},
			})
			srv.NotifyCallEvent(service.CallEvent{
				Kind:   service.CallEventEnded,
				Record: store.CallRecord{ID: 1, DeviceID: "dev1", Direction: "in", Number: "10086"},
			})
		}()
	}
	wg.Wait()
}

func TestResponseHandlingMultiTarget(t *testing.T) {
	srv := NewServer(DefaultConfig(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	addr1 := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5061}
	addr2 := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5062}
	rtp, err := NewRTPStream("127.0.0.1", 0)
	if err != nil {
		t.Fatalf("NewRTPStream: %v", err)
	}
	defer rtp.Close()

	sess := NewCallSession("call-multi@SimCallHub", DirectionInbound, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	sess.RTPStream = rtp
	sess.Targets = []*InviteTarget{
		{Username: "1001", Addr: addr1, Branch: "z9hG4bKb1"},
		{Username: "1002", Addr: addr2, Branch: "z9hG4bKb2"},
	}
	srv.sessMu.Lock()
	srv.sessions[sess.CallID] = sess
	srv.sessMu.Unlock()

	// First target rejects (486): session should remain open for second target.
	srv.handleResponse(&SIPMessage{
		IsResponse: true,
		StatusCode: 486,
		Headers:    map[string]string{"Call-ID": sess.CallID},
	}, addr1)
	if sess.IsClosed() {
		t.Fatal("session closed on single target rejection while another target is still ringing")
	}

	// Second target also rejects: now session should close and be removed.
	srv.handleResponse(&SIPMessage{
		IsResponse: true,
		StatusCode: 486,
		Headers:    map[string]string{"Call-ID": sess.CallID},
	}, addr2)
	if !sess.IsClosed() {
		t.Fatal("session stayed open after all targets rejected")
	}
}
