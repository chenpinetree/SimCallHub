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

// Standard GSMA SGP.22 eSIM AID for Issuer Security Domain Root (ISD-R).
const (
	ISDRAID       = "A0000005591010FFFFFFFF8900000100"
	ISDRAIDTest   = "A0000005591010FFFFFFFF8900050100"
	DefaultTimeout = 5 * time.Second
)

// Channel manages an open 3GPP logical channel to an eUICC chip.
type Channel struct {
	SessionID int
	m         *modem.Modem
}

// OpenISDRChannel opens a logical channel to the eSIM ISD-R applet using AT+CCHO.
func OpenISDRChannel(ctx context.Context, m *modem.Modem) (*Channel, error) {
	// Try standard ISD-R AID first
	lines, err := m.Raw(ctx, fmt.Sprintf(`AT+CCHO="%s"`, ISDRAID), DefaultTimeout)
	if err != nil || len(lines) == 0 {
		// Try alternative AID
		lines, err = m.Raw(ctx, fmt.Sprintf(`AT+CCHO="%s"`, ISDRAIDTest), DefaultTimeout)
		if err != nil {
			return nil, fmt.Errorf("open logical channel (AT+CCHO): %w", err)
		}
	}

	sessionID := -1
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "+CCHO:") {
			val := strings.TrimSpace(line[6:])
			if id, err := strconv.Atoi(val); err == nil && id >= 0 {
				sessionID = id
				break
			}
		}
	}

	if sessionID < 0 {
		return nil, errors.New("eSIM not detected or AT+CCHO did not return a valid session ID")
	}

	return &Channel{
		SessionID: sessionID,
		m:         m,
	}, nil
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

// Transmit transmits a raw APDU command through the logical channel and returns the response bytes.
func (c *Channel) Transmit(ctx context.Context, apduHex string) ([]byte, uint16, error) {
	apduHex = strings.TrimSpace(apduHex)
	apduLen := len(apduHex) / 2
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

	// Handle 61xx (GET RESPONSE): more data available from card
	if sw1 == 0x61 {
		le := sw2
		getRespAPDU := fmt.Sprintf("00C00000%02X", le)
		morePayload, moreSW, err := c.Transmit(ctx, getRespAPDU)
		if err == nil {
			payload = append(payload, morePayload...)
			sw = moreSW
		}
	}

	return payload, sw, nil
}
