package sip

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/chenpinetree/SimCallHub/internal/agent/service"
)

// CallDirection indicates who initiated the call.
type CallDirection int

const (
	DirectionInbound  CallDirection = iota // Incoming cellular call -> ringing SIP clients
	DirectionOutbound                      // SIP client dialed -> outgoing cellular call
)

// CallSession coordinates one active SIP-to-Cellular phone call.
type CallSession struct {
	CallID     string
	Direction  CallDirection
	CallerNum  string
	CalleeNum  string
	FromHeader string
	ToHeader   string
	ViaHeader  string
	CSeq       string

	ClientAddr *net.UDPAddr
	RTPStream  *RTPStream

	ModemSvc    *service.Service
	AudioStream *service.AudioStream

	Codec byte // PayloadPCMU (0) or PayloadPCMA (8)

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	active bool
	closed bool
	log    *slog.Logger
}

// NewCallSession creates a new call session.
func NewCallSession(callID string, dir CallDirection, modem *service.Service, log *slog.Logger) *CallSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &CallSession{
		CallID:    callID,
		Direction: dir,
		ModemSvc:  modem,
		ctx:       ctx,
		cancel:    cancel,
		log:       log,
		Codec:     PayloadPCMU,
	}
}

// StartAudioBridge runs the bidirectional PCM <-> RTP bridge between EC20 and the SIP client.
func (s *CallSession) StartAudioBridge(rtp *RTPStream, remoteRTP *net.UDPAddr) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.RTPStream = rtp
	s.RTPStream.SetRemoteAddr(remoteRTP)
	s.active = true
	s.mu.Unlock()

	// Attach to modem's call audio
	if s.ModemSvc != nil {
		s.AudioStream = s.ModemSvc.AttachAudio()
	}

	s.log.Info("audio bridge established", "call_id", s.CallID, "remote_rtp", remoteRTP.String(), "codec", s.Codec)

	// Goroutine 1: Modem -> SIP (Downlink)
	go s.downlinkLoop()

	// Goroutine 2: SIP -> Modem (Uplink)
	go s.uplinkLoop()
}

// downlinkLoop receives 20 ms 8kHz 16-bit linear PCM frames from EC20 and sends them as RTP to SIP.
func (s *CallSession) downlinkLoop() {
	if s.AudioStream == nil || s.RTPStream == nil {
		return
	}
	frames := s.AudioStream.Frames()
	for {
		select {
		case <-s.ctx.Done():
			return
		case pcm, ok := <-frames:
			if !ok {
				return
			}
			var g711Payload []byte
			if s.Codec == PayloadPCMA {
				g711Payload = LinearToAlaw(pcm)
			} else {
				g711Payload = LinearToUlaw(pcm)
			}

			if err := s.RTPStream.SendAudioFrame(g711Payload, s.Codec); err != nil {
				// Non-fatal dropped packet
			}
		}
	}
}

// uplinkLoop receives RTP packets from the SIP client, decodes G.711 to 16-bit linear PCM, and feeds the modem.
func (s *CallSession) uplinkLoop() {
	if s.AudioStream == nil || s.RTPStream == nil {
		return
	}
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			pkt, _, err := s.RTPStream.ReadPacket()
			if err != nil {
				if s.ctx.Err() != nil {
					return
				}
				time.Sleep(5 * time.Millisecond)
				continue
			}

			if len(pkt.Payload) == 0 {
				continue
			}

			var pcm16 []byte
			if pkt.Header.PayloadType == PayloadPCMA || s.Codec == PayloadPCMA {
				pcm16 = AlawToLinear(pkt.Payload)
			} else {
				pcm16 = UlawToLinear(pkt.Payload)
			}

			if err := s.AudioStream.Write(pcm16); err != nil && err != io.EOF {
				// Dropped frame when queue full
			}
		}
	}
}

// Close terminates the call session and releases all audio and network resources.
func (s *CallSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.active = false
	s.cancel()
	s.mu.Unlock()

	if s.AudioStream != nil {
		s.AudioStream.Close()
	}
	if s.RTPStream != nil {
		s.RTPStream.Close()
	}

	s.log.Info("call session closed", "call_id", s.CallID)
}

// IsActive returns whether the call is currently connected and streaming.
func (s *CallSession) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}
