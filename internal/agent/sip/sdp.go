package sip

import (
	"fmt"
	"strconv"
	"strings"
)

// SDPInfo holds the parsed connection details from an SDP body.
type SDPInfo struct {
	IP          string
	Port        int
	PayloadType byte
}

// GenerateSDP generates a standard SDP offer/answer for G.711 audio.
func GenerateSDP(localIP string, rtpPort int) string {
	if localIP == "" || localIP == "0.0.0.0" {
		localIP = "127.0.0.1"
	}
	return fmt.Sprintf("v=0\r\n"+
		"o=- %d %d IN IP4 %s\r\n"+
		"s=SimCallHub\r\n"+
		"c=IN IP4 %s\r\n"+
		"t=0 0\r\n"+
		"m=audio %d RTP/AVP 0 8\r\n"+
		"a=rtpmap:0 PCMU/8000\r\n"+
		"a=rtpmap:8 PCMA/8000\r\n"+
		"a=sendrecv\r\n",
		rtpPort, rtpPort, localIP, localIP, rtpPort)
}

// ParseSDP extracts the remote IP and RTP port from an SDP text.
func ParseSDP(sdp string) (*SDPInfo, error) {
	info := &SDPInfo{
		PayloadType: PayloadPCMU, // default PCMU
	}

	lines := strings.Split(sdp, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "c=IN IP4 ") {
			info.IP = strings.TrimSpace(line[9:])
		} else if strings.HasPrefix(line, "m=audio ") {
			parts := strings.Split(line, " ")
			if len(parts) >= 2 {
				port, err := strconv.Atoi(parts[1])
				if err == nil {
					info.Port = port
				}
			}
			if len(parts) >= 4 {
				// Check first preferred codec
				if parts[3] == "8" {
					info.PayloadType = PayloadPCMA
				} else {
					info.PayloadType = PayloadPCMU
				}
			}
		}
	}

	if info.IP == "" || info.Port == 0 {
		return nil, fmt.Errorf("could not extract valid IP and port from SDP")
	}
	return info, nil
}
