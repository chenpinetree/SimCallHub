package esim

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chenpinetree/SimCallHub/internal/agent/modem"
)

// Standard GSMA SGP.22 and vendor-specific (Xesim X7/X2, 5ber, estk) eSIM ISD-R AIDs.
const (
	ISDRAIDStandard = "A0000005591010FFFFFFFF8900000100"
	ISDRAIDXesimX7  = "A0000005591010FFFFFFFF8900000177"
	ISDRAIDXesimAlt = "A0000005591010FFFFFFFF8900050500"
	ISDRAIDTest     = "A0000005591010FFFFFFFF8900050100"
	ISDRAIDPrefix   = "A0000005591010"
	XesimSCIDAID    = "A000000533C000FF860000000427"
	DefaultTimeout  = 5 * time.Second
)

// CandidateISDRAIDs lists the ISD-R AIDs in priority order.
var CandidateISDRAIDs = []string{
	ISDRAIDStandard,
	ISDRAIDXesimX7,
	ISDRAIDXesimAlt,
	ISDRAIDTest,
	ISDRAIDPrefix,
}

// Channel manages an open 3GPP logical channel to an eUICC chip.
type Channel struct {
	SessionID int
	AID       string
	m         *modem.Modem
}

// IsSuccessSW reports whether an ISO 7816-4 / 3GPP status word indicates success.
// In addition to 9000, eUICC cards (such as Xesim X7) return 91xx (e.g. 910B) when
// a USAT proactive REFRESH command is queued after profile enable/disable.
func IsSuccessSW(sw uint16) bool {
	hi := sw >> 8
	return sw == 0x9000 || hi == 0x90 || hi == 0x91
}

// parseCCHOSession extracts the positive logical channel session ID from AT+CCHO output lines.
func parseCCHOSession(lines []string) int {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CCHO:") {
			val := strings.TrimSpace(line[6:])
			// Handle formats like "+CCHO: 1" or "+CCHO: 1,..."
			if idx := strings.IndexByte(val, ','); idx >= 0 {
				val = strings.TrimSpace(val[:idx])
			}
			if id, err := strconv.Atoi(val); err == nil && id >= 0 {
				return id
			}
		}
	}
	return -1
}

// OpenISDRChannel opens a logical channel to the eSIM ISD-R applet using AT+CCHO.
// Note: m.Raw returns lines=["ERROR"] with err=nil when an AT command returns ERROR,
// so each candidate AID must be validated by checking for a parsed +CCHO session ID.
func OpenISDRChannel(ctx context.Context, m *modem.Modem) (*Channel, error) {
	var lastErr error
	for _, aid := range CandidateISDRAIDs {
		lines, err := m.Raw(ctx, fmt.Sprintf(`AT+CCHO="%s"`, aid), DefaultTimeout)
		if err != nil {
			lastErr = err
			continue
		}
		if sessionID := parseCCHOSession(lines); sessionID >= 0 {
			return &Channel{
				SessionID: sessionID,
				AID:       aid,
				m:         m,
			}, nil
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("open logical channel (AT+CCHO): %w", lastErr)
	}
	return nil, errors.New("eSIM not detected or AT+CCHO did not return a valid session ID")
}

// Close closes the logical channel using AT+CCHC.
func (c *Channel) Close(ctx context.Context) error {
	if c.SessionID < 0 {
		return nil
	}
	_, err := c.m.Raw(ctx, fmt.Sprintf("AT+CCHC=%d", c.SessionID), DefaultTimeout)
	c.SessionID = -1
	return err
}

// TransmitStoreData wraps an SGP.22 ES10x command payload in STORE DATA (80 E2) APDUs,
// automatically splitting payloads larger than 120 bytes into chained chunks
// (80 E2 11 <seq> <len> ... followed by 80 E2 91 <seq> <len> ...00) as in Xesim LPA.
func (c *Channel) TransmitStoreData(ctx context.Context, tlvHex string) ([]byte, uint16, error) {
	tlvHex = strings.TrimSpace(tlvHex)
	const maxChunkHex = 240 // 120 bytes per APDU chunk
	if len(tlvHex) <= maxChunkHex {
		apdu := fmt.Sprintf("80E29100%02X%s00", len(tlvHex)/2, tlvHex)
		return c.Transmit(ctx, apdu)
	}

	seq := 0
	for offset := 0; offset < len(tlvHex); offset += maxChunkHex {
		end := offset + maxChunkHex
		last := end >= len(tlvHex)
		if last {
			end = len(tlvHex)
		}
		chunk := tlvHex[offset:end]
		p1 := "11"
		suffix := ""
		if last {
			p1 = "91"
			suffix = "00"
		}
		apdu := fmt.Sprintf("80E2%s%02X%02X%s%s", p1, seq, len(chunk)/2, chunk, suffix)
		payload, sw, err := c.Transmit(ctx, apdu)
		if err != nil {
			return nil, sw, err
		}
		if last || !IsSuccessSW(sw) {
			return payload, sw, nil
		}
		seq++
	}
	return nil, 0, errors.New("empty store data payload")
}

// Transmit transmits a raw APDU command through the logical channel and returns the response bytes.
func (c *Channel) Transmit(ctx context.Context, apduHex string) ([]byte, uint16, error) {
	apduHex = strings.TrimSpace(apduHex)
	// In 3GPP TS 27.007 and Quectel modems (+CGLA=<sessionid>,<length>,<command>),
	// <length> is an integer specifying the character count of <command>, not byte count.
	apduLen := len(apduHex)
	cmd := fmt.Sprintf(`AT+CGLA=%d,%d,"%s"`, c.SessionID, apduLen, apduHex)

	lines, err := c.m.Raw(ctx, cmd, DefaultTimeout)
	if err != nil {
		return nil, 0, fmt.Errorf("CGLA command error: %w", err)
	}

	var respHex string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CGLA:") {
			parts := strings.SplitN(line[6:], ",", 2)
			if len(parts) == 2 {
				respHex = strings.Trim(strings.TrimSpace(parts[1]), `"`)
				break
			}
		}
	}

	if respHex == "" {
		return nil, 0, errors.New("empty response from CGLA")
	}

	respBytes, err := hex.DecodeString(respHex)
	if err != nil || len(respBytes) < 2 {
		return nil, 0, fmt.Errorf("invalid CGLA hex response: %s", respHex)
	}

	sw1 := respBytes[len(respBytes)-2]
	sw2 := respBytes[len(respBytes)-1]
	sw := (uint16(sw1) << 8) | uint16(sw2)
	payload := respBytes[:len(respBytes)-2]

	// Handle 6Cxx (Wrong length Le): retry command with exact expected length sw2
	if sw1 == 0x6C && len(apduHex) >= 10 {
		correctedAPDU := apduHex[:len(apduHex)-2] + fmt.Sprintf("%02X", sw2)
		return c.Transmit(ctx, correctedAPDU)
	}

	// Handle 61xx (GET RESPONSE): more data available from card (e.g. 61B5 -> 00C00000B5)
	for sw1 == 0x61 {
		le := sw2
		getRespAPDU := fmt.Sprintf("00C00000%02X", le)
		morePayload, moreSW, err := c.Transmit(ctx, getRespAPDU)
		if err != nil {
			return payload, sw, err
		}
		payload = append(payload, morePayload...)
		sw = moreSW
		sw1 = byte(sw >> 8)
		sw2 = byte(sw & 0xFF)
	}

	return payload, sw, nil
}
