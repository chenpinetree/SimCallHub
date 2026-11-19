package sip

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/chenpinetree/SimCallHub/internal/agent/service"
)

// Config configures the embedded SIP PBX extension server.
type Config struct {
	Enabled      bool   `json:"enabled"`
	ListenAddr   string `json:"listen"`       // e.g. "0.0.0.0:5060"
	PublicIP     string `json:"public_ip"`    // IP reported in SDP (defaults to local LAN IP)
	Domain       string `json:"domain"`       // SIP domain/realm (default "SimCallHub")
	Username     string `json:"username"`     // Default extension username (e.g. "1001")
	Password     string `json:"password"`     // Extension password
	RTPPortStart int    `json:"rtp_start"`    // Starting RTP port (e.g. 20000)
	RTPPortEnd   int    `json:"rtp_end"`      // Ending RTP port (e.g. 20100)
	RingTimeout  int    `json:"ring_timeout"` // Seconds to ring extensions before letting AI answer (default 20)
}

// DefaultConfig returns recommended SIP server settings.
func DefaultConfig() Config {
	return Config{
		Enabled:      true,
		ListenAddr:   "0.0.0.0:5060",
		Domain:       "SimCallHub",
		Username:     "1001",
		Password:     "12345678",
		RTPPortStart: 20000,
		RTPPortEnd:   20100,
		RingTimeout:  20,
	}
}

// Server provides the SIP VoIP extension gateway for SimCallHub.
type Server struct {
	cfg       Config
	devices   *service.Manager
	registrar *Registrar
	log       *slog.Logger

	conn     *net.UDPConn
	sessions map[string]*CallSession // Call-ID -> session
	sessMu   sync.Mutex

	nextRTPPort int
	rtpMu       sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewServer creates a new SIP server instance.
func NewServer(cfg Config, devices *service.Manager, log *slog.Logger) *Server {
	if cfg.Domain == "" {
		cfg.Domain = "SimCallHub"
	}
	if cfg.RTPPortStart <= 0 {
		cfg.RTPPortStart = 20000
	}
	if cfg.RTPPortEnd <= cfg.RTPPortStart {
		cfg.RTPPortEnd = cfg.RTPPortStart + 100
	}
	if cfg.RingTimeout <= 0 {
		cfg.RingTimeout = 20
	}

	reg := NewRegistrar(cfg.Domain, cfg.Username, cfg.Password)

	return &Server{
		cfg:         cfg,
		devices:     devices,
		registrar:   reg,
		log:         log,
		sessions:    make(map[string]*CallSession),
		nextRTPPort: cfg.RTPPortStart,
	}
}

// ServerStatus describes the runtime state of the SIP server.
type ServerStatus struct {
	Enabled     bool           `json:"enabled"`
	ListenAddr  string         `json:"listen_addr"`
	Domain      string         `json:"domain"`
	Username    string         `json:"username"`
	Password    string         `json:"password"`
	RingTimeout int            `json:"ring_timeout"`
	Clients     []ClientStatus `json:"clients"`
}

// Status returns current server configuration and connected extensions.
func (s *Server) Status() ServerStatus {
	s.sessMu.Lock()
	cfg := s.cfg
	s.sessMu.Unlock()

	return ServerStatus{
		Enabled:     cfg.Enabled,
		ListenAddr:  cfg.ListenAddr,
		Domain:      cfg.Domain,
		Username:    cfg.Username,
		Password:    cfg.Password,
		RingTimeout: cfg.RingTimeout,
		Clients:     s.registrar.ListActive(),
	}
}

// UpdateConfig updates the username, password and ring timeout dynamically.
func (s *Server) UpdateConfig(username, password string, ringTimeout int) {
	s.sessMu.Lock()
	oldUser := s.cfg.Username
	s.cfg.Username = username
	s.cfg.Password = password
	if ringTimeout > 0 {
		s.cfg.RingTimeout = ringTimeout
	}
	s.sessMu.Unlock()

	s.registrar.UpdateAccount(oldUser, username, password)
}

// Start begins listening on the SIP UDP socket and dispatches incoming messages.
func (s *Server) Start(ctx context.Context) error {
	if !s.cfg.Enabled {
		s.log.Info("sip server is disabled in configuration")
		return nil
	}

	addr, err := net.ResolveUDPAddr("udp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("resolve sip listen addr %s: %w", s.cfg.ListenAddr, err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen sip udp on %s: %w", s.cfg.ListenAddr, err)
	}
	s.conn = conn

	s.ctx, s.cancel = context.WithCancel(ctx)

	s.log.Info("sip server started",
		"listen", conn.LocalAddr().String(),
		"extension", s.cfg.Username,
		"domain", s.cfg.Domain,
		"rtp_range", fmt.Sprintf("%d-%d", s.cfg.RTPPortStart, s.cfg.RTPPortEnd))

	go s.listenLoop()
	return nil
}

// Stop gracefully shuts down the SIP server.
func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		s.conn.Close()
	}

	s.sessMu.Lock()
	for _, sess := range s.sessions {
		sess.Close()
	}
	s.sessions = make(map[string]*CallSession)
	s.sessMu.Unlock()
}

func (s *Server) allocRTPPort() int {
	s.rtpMu.Lock()
	defer s.rtpMu.Unlock()
	port := s.nextRTPPort
	s.nextRTPPort += 2
	if s.nextRTPPort >= s.cfg.RTPPortEnd {
		s.nextRTPPort = s.cfg.RTPPortStart
	}
	return port
}

func (s *Server) publicIP(remote net.IP) string {
	if s.cfg.PublicIP != "" {
		return s.cfg.PublicIP
	}
	// Try to find local interface IP routable to client
	if remote != nil && !remote.IsLoopback() {
		if conn, err := net.Dial("udp", remote.String()+":1234"); err == nil {
			defer conn.Close()
			if local, ok := conn.LocalAddr().(*net.UDPAddr); ok {
				return local.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func (s *Server) send(msg *SIPMessage, to *net.UDPAddr) error {
	raw := []byte(msg.String())
	_, err := s.conn.WriteToUDP(raw, to)
	return err
}

func (s *Server) listenLoop() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			n, from, err := s.conn.ReadFromUDP(buf)
			if err != nil {
				if s.ctx.Err() != nil {
					return
				}
				continue
			}

			packet := make([]byte, n)
			copy(packet, buf[:n])
			go s.handlePacket(packet, from)
		}
	}
}

func (s *Server) handlePacket(data []byte, from *net.UDPAddr) {
	msg, err := ParseSIPMessage(data)
	if err != nil {
		return
	}

	if msg.IsResponse {
		s.handleResponse(msg, from)
	} else {
		s.handleRequest(msg, from)
	}
}

func (s *Server) handleRequest(msg *SIPMessage, from *net.UDPAddr) {
	switch msg.Method {
	case "REGISTER":
		s.handleRegister(msg, from)
	case "INVITE":
		s.handleInvite(msg, from)
	case "ACK":
		s.handleAck(msg, from)
	case "BYE":
		s.handleBye(msg, from)
	case "CANCEL":
		s.handleCancel(msg, from)
	case "OPTIONS":
		s.handleOptions(msg, from)
	default:
		s.replySimple(msg, from, 405, "Method Not Allowed")
	}
}

func (s *Server) replySimple(req *SIPMessage, from *net.UDPAddr, code int, reason string) {
	resp := &SIPMessage{
		IsResponse:   true,
		StatusCode:   code,
		ReasonPhrase: reason,
		Headers: map[string]string{
			"Via":     req.GetHeader("Via"),
			"From":    req.GetHeader("From"),
			"To":      req.GetHeader("To"),
			"Call-ID": req.GetHeader("Call-ID"),
			"CSeq":    req.GetHeader("CSeq"),
		},
	}
	_ = s.send(resp, from)
}

func (s *Server) handleOptions(req *SIPMessage, from *net.UDPAddr) {
	resp := &SIPMessage{
		IsResponse:   true,
		StatusCode:   200,
		ReasonPhrase: "OK",
		Headers: map[string]string{
			"Via":     req.GetHeader("Via"),
			"From":    req.GetHeader("From"),
			"To":      req.GetHeader("To"),
			"Call-ID": req.GetHeader("Call-ID"),
			"CSeq":    req.GetHeader("CSeq"),
			"Allow":   "INVITE, ACK, CANCEL, BYE, OPTIONS, REGISTER",
		},
	}
	_ = s.send(resp, from)
}

func (s *Server) handleRegister(req *SIPMessage, from *net.UDPAddr) {
	username, ok := s.registrar.Authenticate(req)
	if !ok {
		// Challenge with 401
		nonce := s.registrar.GenerateNonce()
		resp := &SIPMessage{
			IsResponse:   true,
			StatusCode:   401,
			ReasonPhrase: "Unauthorized",
			Headers: map[string]string{
				"Via":              req.GetHeader("Via"),
				"From":             req.GetHeader("From"),
				"To":               req.GetHeader("To"),
				"Call-ID":          req.GetHeader("Call-ID"),
				"CSeq":             req.GetHeader("CSeq"),
				"WWW-Authenticate": fmt.Sprintf(`Digest realm="%s", nonce="%s", algorithm=MD5`, s.registrar.Realm(), nonce),
			},
		}
		_ = s.send(resp, from)
		return
	}

	contact := req.GetHeader("Contact")
	expires := ParseExpires(req)
	callID := req.GetHeader("Call-ID")
	userAgent := req.GetHeader("User-Agent")

	s.registrar.Register(username, contact, from, expires, callID, userAgent)
	s.log.Info("sip extension registered", "extension", username, "client", from.String(), "user_agent", userAgent, "expires", expires)

	resp := &SIPMessage{
		IsResponse:   true,
		StatusCode:   200,
		ReasonPhrase: "OK",
		Headers: map[string]string{
			"Via":     req.GetHeader("Via"),
			"From":    req.GetHeader("From"),
			"To":      req.GetHeader("To"),
			"Call-ID": req.GetHeader("Call-ID"),
			"CSeq":    req.GetHeader("CSeq"),
			"Contact": contact,
			"Expires": strconv.Itoa(expires),
		},
	}
	_ = s.send(resp, from)
}

func (s *Server) handleInvite(req *SIPMessage, from *net.UDPAddr) {
	// Client is calling out through the cellular modem
	_, ok := s.registrar.Authenticate(req)
	if !ok {
		nonce := s.registrar.GenerateNonce()
		resp := &SIPMessage{
			IsResponse:   true,
			StatusCode:   401,
			ReasonPhrase: "Unauthorized",
			Headers: map[string]string{
				"Via":              req.GetHeader("Via"),
				"From":             req.GetHeader("From"),
				"To":               req.GetHeader("To"),
				"Call-ID":          req.GetHeader("Call-ID"),
				"CSeq":             req.GetHeader("CSeq"),
				"WWW-Authenticate": fmt.Sprintf(`Digest realm="%s", nonce="%s", algorithm=MD5`, s.registrar.Realm(), nonce),
			},
		}
		_ = s.send(resp, from)
		return
	}

	callID := req.GetHeader("Call-ID")
	dialedNum := ExtractURIUsername(req.URI)

	// Send 100 Trying
	s.replySimple(req, from, 100, "Trying")

	// Find available modem
	modems := s.devices.Services()
	if len(modems) == 0 {
		s.replySimple(req, from, 503, "Service Unavailable (No 4G Modem)")
		return
	}
	modem := modems[0]

	sdpInfo, err := ParseSDP(string(req.Body))
	if err != nil {
		s.replySimple(req, from, 400, "Bad SDP")
		return
	}

	// Prepare RTP stream for this call
	rtpPort := s.allocRTPPort()
	rtpStream, err := NewRTPStream("0.0.0.0", rtpPort)
	if err != nil {
		s.replySimple(req, from, 500, "Failed to allocate RTP port")
		return
	}

	sess := NewCallSession(callID, DirectionOutbound, modem, s.log)
	sess.CallerNum = ExtractURIUsername(req.GetHeader("From"))
	sess.CalleeNum = dialedNum
	sess.ClientAddr = from
	sess.FromHeader = req.GetHeader("From")
	sess.ToHeader = req.GetHeader("To")
	sess.ViaHeader = req.GetHeader("Via")
	sess.CSeq = req.GetHeader("CSeq")
	sess.Codec = sdpInfo.PayloadType

	s.sessMu.Lock()
	s.sessions[callID] = sess
	s.sessMu.Unlock()

	// Send 180 Ringing
	ringing := &SIPMessage{
		IsResponse:   true,
		StatusCode:   180,
		ReasonPhrase: "Ringing",
		Headers: map[string]string{
			"Via":     req.GetHeader("Via"),
			"From":    req.GetHeader("From"),
			"To":      req.GetHeader("To"),
			"Call-ID": callID,
			"CSeq":    req.GetHeader("CSeq"),
		},
	}
	_ = s.send(ringing, from)

	// Dial cellular network in background
	go func() {
		dialCtx, dialCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dialCancel()

		s.log.Info("dialing out through 4G modem", "number", dialedNum, "call_id", callID)
		if err := modem.Dial(dialCtx, dialedNum); err != nil {
			s.log.Error("modem dial failed", "error", err)
			s.replySimple(req, from, 503, "Modem Dial Failed: "+err.Error())
			sess.Close()
			s.sessMu.Lock()
			delete(s.sessions, callID)
			s.sessMu.Unlock()
			return
		}

		// Modem dialing succeeded -> send 200 OK with SDP
		localIP := s.publicIP(from.IP)
		sdpBody := GenerateSDP(localIP, rtpPort)

		okResp := &SIPMessage{
			IsResponse:   true,
			StatusCode:   200,
			ReasonPhrase: "OK",
			Headers: map[string]string{
				"Via":            req.GetHeader("Via"),
				"From":           req.GetHeader("From"),
				"To":             req.GetHeader("To") + ";tag=" + hex.EncodeToString(randomBytes(4)),
				"Call-ID":        callID,
				"CSeq":           req.GetHeader("CSeq"),
				"Contact":        fmt.Sprintf("<sip:%s@%s:%d>", s.cfg.Username, localIP, addrPort(s.conn.LocalAddr())),
				"Content-Type":   "application/sdp",
				"Content-Length": strconv.Itoa(len(sdpBody)),
			},
			Body: []byte(sdpBody),
		}
		_ = s.send(okResp, from)

		remoteRTP := &net.UDPAddr{IP: net.ParseIP(sdpInfo.IP), Port: sdpInfo.Port}
		sess.StartAudioBridge(rtpStream, remoteRTP)
	}()
}

func (s *Server) handleAck(req *SIPMessage, from *net.UDPAddr) {
	callID := req.GetHeader("Call-ID")
	s.sessMu.Lock()
	sess, ok := s.sessions[callID]
	s.sessMu.Unlock()
	if ok && sess != nil {
		s.log.Info("received ACK for call session", "call_id", callID)
	}
}

func (s *Server) handleBye(req *SIPMessage, from *net.UDPAddr) {
	callID := req.GetHeader("Call-ID")
	s.sessMu.Lock()
	sess, ok := s.sessions[callID]
	delete(s.sessions, callID)
	s.sessMu.Unlock()

	s.replySimple(req, from, 200, "OK")

	if ok && sess != nil {
		s.log.Info("call hung up by SIP client", "call_id", callID)
		sess.Close()
		if sess.ModemSvc != nil {
			go func() {
				hCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = sess.ModemSvc.Hangup(hCtx)
			}()
		}
	}
}

func (s *Server) handleCancel(req *SIPMessage, from *net.UDPAddr) {
	callID := req.GetHeader("Call-ID")
	s.sessMu.Lock()
	sess, ok := s.sessions[callID]
	delete(s.sessions, callID)
	s.sessMu.Unlock()

	s.replySimple(req, from, 200, "OK")
	s.replySimple(req, from, 487, "Request Terminated")

	if ok && sess != nil {
		sess.Close()
		if sess.ModemSvc != nil {
			go func() {
				hCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = sess.ModemSvc.Hangup(hCtx)
			}()
		}
	}
}

func (s *Server) handleResponse(resp *SIPMessage, from *net.UDPAddr) {
	callID := resp.GetHeader("Call-ID")
	s.sessMu.Lock()
	sess, ok := s.sessions[callID]
	s.sessMu.Unlock()

	if !ok || sess == nil {
		return
	}

	switch {
	case resp.StatusCode == 180:
		s.log.Info("sip extension ringing", "call_id", callID, "client", from.String())
	case resp.StatusCode == 200:
		// SIP extension answered the incoming call!
		s.log.Info("sip extension answered incoming call!", "call_id", callID, "client", from.String())

		// Send ACK back to the client
		ack := &SIPMessage{
			IsResponse: false,
			Method:     "ACK",
			URI:        fmt.Sprintf("sip:%s@%s", s.cfg.Username, from.String()),
			Headers: map[string]string{
				"Via":     fmt.Sprintf("SIP/2.0/UDP %s;branch=z9hG4bK%s", s.publicIP(from.IP), hex.EncodeToString(randomBytes(6))),
				"From":    resp.GetHeader("From"),
				"To":      resp.GetHeader("To"),
				"Call-ID": callID,
				"CSeq":    resp.GetHeader("CSeq") + " ACK",
			},
		}
		_ = s.send(ack, from)

		// Parse SDP answer from extension
		sdpInfo, err := ParseSDP(string(resp.Body))
		if err == nil && sess.RTPStream != nil {
			sess.Codec = sdpInfo.PayloadType
			remoteRTP := &net.UDPAddr{IP: net.ParseIP(sdpInfo.IP), Port: sdpInfo.Port}

			// Answer the cellular modem
			if sess.ModemSvc != nil {
				go func() {
					aCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = sess.ModemSvc.Answer(aCtx)
				}()
			}

			sess.StartAudioBridge(sess.RTPStream, remoteRTP)
		}
	case resp.StatusCode >= 400:
		s.log.Info("sip extension rejected or failed call", "code", resp.StatusCode, "call_id", callID)
		sess.Close()
		s.sessMu.Lock()
		delete(s.sessions, callID)
		s.sessMu.Unlock()
	}
}

// NotifyCallEvent is triggered when a cellular call state changes (e.g. Ringing or Missed).
func (s *Server) NotifyCallEvent(event service.CallEvent) {
	if !s.cfg.Enabled {
		return
	}

	if event.Kind == service.CallEventRinging && event.Record.Direction == "in" {
		if !event.Repeat {
			s.ringExtensions(event)
		}
	} else if event.Kind == service.CallEventMissed {
		// Cancel any active ringing sessions for this call
		s.sessMu.Lock()
		for id, sess := range s.sessions {
			if sess.Direction == DirectionInbound && !sess.IsActive() {
				sess.Close()
				delete(s.sessions, id)
			}
		}
		s.sessMu.Unlock()
	}
}

// ringExtensions sends an INVITE with SDP offer to all currently registered SIP clients.
func (s *Server) ringExtensions(event service.CallEvent) {
	active := s.registrar.GetActiveExtensions()
	if len(active) == 0 {
		s.log.Info("incoming 4G call ringing, but no SIP extensions currently registered")
		return
	}

	caller := event.Record.Number
	if caller == "" {
		caller = "Unknown"
	}

	modems := s.devices.Services()
	var modem *service.Service
	for _, m := range modems {
		if m.DeviceID() == event.Record.DeviceID {
			modem = m
			break
		}
	}
	if modem == nil && len(modems) > 0 {
		modem = modems[0]
	}

	callID := hex.EncodeToString(randomBytes(16)) + "@" + s.cfg.Domain
	rtpPort := s.allocRTPPort()
	rtpStream, err := NewRTPStream("0.0.0.0", rtpPort)
	if err != nil {
		s.log.Error("failed to allocate RTP stream for inbound call", "error", err)
		return
	}

	sess := NewCallSession(callID, DirectionInbound, modem, s.log)
	sess.CallerNum = caller
	sess.RTPStream = rtpStream

	s.sessMu.Lock()
	s.sessions[callID] = sess
	s.sessMu.Unlock()

	s.log.Info("ringing registered SIP extensions for incoming call", "caller", caller, "extensions", len(active))

	for _, ext := range active {
		localIP := s.publicIP(ext.Addr.IP)
		sdpBody := GenerateSDP(localIP, rtpPort)

		invite := &SIPMessage{
			IsResponse: false,
			Method:     "INVITE",
			URI:        fmt.Sprintf("sip:%s@%s", ext.Username, ext.Addr.String()),
			Headers: map[string]string{
				"Via":            fmt.Sprintf("SIP/2.0/UDP %s:%d;branch=z9hG4bK%s;rport", localIP, addrPort(s.conn.LocalAddr()), hex.EncodeToString(randomBytes(6))),
				"From":           fmt.Sprintf(`"%s" <sip:%s@%s>;tag=%s`, caller, caller, s.cfg.Domain, hex.EncodeToString(randomBytes(4))),
				"To":             fmt.Sprintf("<sip:%s@%s>", ext.Username, s.cfg.Domain),
				"Call-ID":        callID,
				"CSeq":           "100 INVITE",
				"Contact":        fmt.Sprintf("<sip:%s@%s:%d>", s.cfg.Username, localIP, addrPort(s.conn.LocalAddr())),
				"Content-Type":   "application/sdp",
				"Content-Length": strconv.Itoa(len(sdpBody)),
			},
			Body: []byte(sdpBody),
		}
		_ = s.send(invite, ext.Addr)
	}

	// Ring timeout watchdog: if not answered within RingTimeout, cancel INVITE so SimCallHub AI can take over
	go func() {
		select {
		case <-time.After(time.Duration(s.cfg.RingTimeout) * time.Second):
			s.sessMu.Lock()
			sessRef, ok := s.sessions[callID]
			s.sessMu.Unlock()

			if ok && sessRef != nil && !sessRef.IsActive() {
				s.log.Info("SIP ring timeout exceeded, canceling extension invite for AI/voice fallback", "call_id", callID)
				for _, ext := range active {
					cancelMsg := &SIPMessage{
						IsResponse: false,
						Method:     "CANCEL",
						URI:        fmt.Sprintf("sip:%s@%s", ext.Username, ext.Addr.String()),
						Headers: map[string]string{
							"Via":     fmt.Sprintf("SIP/2.0/UDP %s:%d;branch=z9hG4bKcancel;rport", s.publicIP(ext.Addr.IP), addrPort(s.conn.LocalAddr())),
							"From":    fmt.Sprintf(`"%s" <sip:%s@%s>`, caller, caller, s.cfg.Domain),
							"To":      fmt.Sprintf("<sip:%s@%s>", ext.Username, s.cfg.Domain),
							"Call-ID": callID,
							"CSeq":    "100 CANCEL",
						},
					}
					_ = s.send(cancelMsg, ext.Addr)
				}
				sessRef.Close()
				s.sessMu.Lock()
				delete(s.sessions, callID)
				s.sessMu.Unlock()
			}
		case <-sess.ctx.Done():
			return
		}
	}()
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func addrPort(a net.Addr) int {
	if u, ok := a.(*net.UDPAddr); ok {
		return u.Port
	}
	return 5060
}
