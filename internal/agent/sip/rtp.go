package sip

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
)

// Standard RTP payload types for audio.
const (
	PayloadPCMU = 0 // G.711 u-law, 8000 Hz, mono
	PayloadPCMA = 8 // G.711 a-law, 8000 Hz, mono
)

// RTPHeader is the 12-byte fixed RTP header (RFC 3550).
type RTPHeader struct {
	Version        byte   // 2 bits (usually 2)
	Padding        bool   // 1 bit
	Extension      bool   // 1 bit
	CSRCCount      byte   // 4 bits
	Marker         bool   // 1 bit
	PayloadType    byte   // 7 bits
	SequenceNumber uint16 // 16 bits
	Timestamp      uint32 // 32 bits
	SSRC           uint32 // 32 bits
}

// RTPPacket represents an RTP packet with header and payload.
type RTPPacket struct {
	Header  RTPHeader
	Payload []byte
}

// Marshal serializes the RTP packet into raw bytes.
func (p *RTPPacket) Marshal() []byte {
	buf := make([]byte, 12+len(p.Payload))
	// Byte 0: V(2), P(1), X(1), CC(4)
	v := p.Header.Version
	if v == 0 {
		v = 2
	}
	b0 := (v << 6) & 0xC0
	if p.Header.Padding {
		b0 |= 0x20
	}
	if p.Header.Extension {
		b0 |= 0x10
	}
	b0 |= (p.Header.CSRCCount & 0x0F)
	buf[0] = b0

	// Byte 1: M(1), PT(7)
	b1 := p.Header.PayloadType & 0x7F
	if p.Header.Marker {
		b1 |= 0x80
	}
	buf[1] = b1

	binary.BigEndian.PutUint16(buf[2:4], p.Header.SequenceNumber)
	binary.BigEndian.PutUint32(buf[4:8], p.Header.Timestamp)
	binary.BigEndian.PutUint32(buf[8:12], p.Header.SSRC)
	copy(buf[12:], p.Payload)
	return buf
}

// ParseRTP decodes raw UDP bytes into an RTPPacket.
func ParseRTP(data []byte) (*RTPPacket, error) {
	if len(data) < 12 {
		return nil, errors.New("rtp packet too short")
	}
	v := (data[0] >> 6) & 0x03
	if v != 2 {
		return nil, fmt.Errorf("unsupported rtp version: %d", v)
	}
	p := (data[0] & 0x20) != 0
	x := (data[0] & 0x10) != 0
	cc := int(data[0] & 0x0F)
	m := (data[1] & 0x80) != 0
	pt := data[1] & 0x7F
	seq := binary.BigEndian.Uint16(data[2:4])
	ts := binary.BigEndian.Uint32(data[4:8])
	ssrc := binary.BigEndian.Uint32(data[8:12])

	offset := 12 + cc*4
	if len(data) < offset {
		return nil, errors.New("rtp packet cut off in csrc list")
	}
	if x {
		if len(data) < offset+4 {
			return nil, errors.New("rtp packet cut off in extension header")
		}
		extLen := int(binary.BigEndian.Uint16(data[offset+2:offset+4])) * 4
		offset += 4 + extLen
		if len(data) < offset {
			return nil, errors.New("rtp packet cut off in extension body")
		}
	}

	payloadLen := len(data) - offset
	if p && payloadLen > 0 {
		padLen := int(data[len(data)-1])
		if padLen > payloadLen {
			return nil, errors.New("invalid rtp padding length")
		}
		payloadLen -= padLen
	}

	payload := make([]byte, payloadLen)
	copy(payload, data[offset:offset+payloadLen])

	return &RTPPacket{
		Header: RTPHeader{
			Version:        v,
			Padding:        p,
			Extension:      x,
			CSRCCount:      byte(cc),
			Marker:         m,
			PayloadType:    pt,
			SequenceNumber: seq,
			Timestamp:      ts,
			SSRC:           ssrc,
		},
		Payload: payload,
	}, nil
}

// RTPStream manages a local UDP socket for bidirectional RTP streaming with a SIP client.
type RTPStream struct {
	conn       *net.UDPConn
	localPort  int
	remoteAddr *net.UDPAddr

	ssrc      uint32
	seq       uint32
	timestamp uint32

	mu     sync.Mutex
	closed bool
}

// NewRTPStream opens a UDP socket bound to the requested port or dynamic port.
func NewRTPStream(bindIP string, port int) (*RTPStream, error) {
	if bindIP == "" {
		bindIP = "0.0.0.0"
	}
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", bindIP, port))
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	actualPort := conn.LocalAddr().(*net.UDPAddr).Port

	return &RTPStream{
		conn:      conn,
		localPort: actualPort,
		ssrc:      rand.Uint32(),
	}, nil
}

// LocalPort returns the local UDP port bound for RTP.
func (s *RTPStream) LocalPort() int {
	return s.localPort
}

// SetRemoteAddr sets the remote client RTP IP and port.
func (s *RTPStream) SetRemoteAddr(addr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remoteAddr = addr
}

// RemoteAddr returns the current remote RTP address.
func (s *RTPStream) RemoteAddr() *net.UDPAddr {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.remoteAddr
}

// SendAudioFrame packets a 20 ms audio payload (e.g. 160 bytes G.711u) and sends it via UDP.
func (s *RTPStream) SendAudioFrame(payload []byte, payloadType byte) error {
	s.mu.Lock()
	dest := s.remoteAddr
	closed := s.closed
	s.mu.Unlock()

	if closed || dest == nil {
		return nil
	}

	seq := uint16(atomic.AddUint32(&s.seq, 1))
	// 20 ms at 8000 Hz = 160 samples increment per frame
	ts := atomic.AddUint32(&s.timestamp, 160)

	pkt := &RTPPacket{
		Header: RTPHeader{
			Version:        2,
			PayloadType:    payloadType,
			SequenceNumber: seq,
			Timestamp:      ts,
			SSRC:           s.ssrc,
		},
		Payload: payload,
	}

	bytes := pkt.Marshal()
	_, err := s.conn.WriteToUDP(bytes, dest)
	return err
}

// ReadPacket blocks until a packet arrives on the UDP socket.
func (s *RTPStream) ReadPacket() (*RTPPacket, *net.UDPAddr, error) {
	buf := make([]byte, 2048)
	n, from, err := s.conn.ReadFromUDP(buf)
	if err != nil {
		return nil, nil, err
	}

	// Auto-learn remote RTP destination from incoming packets (NAT traversal)
	s.mu.Lock()
	if s.remoteAddr == nil || (s.remoteAddr.IP.String() != from.IP.String() || s.remoteAddr.Port != from.Port) {
		s.remoteAddr = from
	}
	s.mu.Unlock()

	pkt, err := ParseRTP(buf[:n])
	if err != nil {
		return nil, from, err
	}
	return pkt, from, nil
}

// Close closes the RTP socket.
func (s *RTPStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.conn.Close()
}
