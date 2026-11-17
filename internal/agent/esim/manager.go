package esim

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/chenpinetree/SimCallHub/internal/agent/modem"
)

// Profile represents an installed eSIM/eUICC profile.
type Profile struct {
	ICCID    string `json:"iccid"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Nickname string `json:"nickname"`
	State    string `json:"state"` // "enabled" | "disabled"
	Class    string `json:"class"` // "operational" | "test" | "provisioning"
}

// EUICCInfo reports the eUICC chip identity and installed profiles.
type EUICCInfo struct {
	Supported bool      `json:"supported"`
	EID       string    `json:"eid"`
	Profiles  []Profile `json:"profiles"`
}

// Manager orchestrates SGP.22 eSIM profile operations over the 3GPP AT interface.
type Manager struct{}

// NewManager creates a new eSIM manager.
func NewManager() *Manager {
	return &Manager{}
}

// GetInfo checks if the current SIM is an eUICC and retrieves its EID and profile list.
func (mgr *Manager) GetInfo(ctx context.Context, m *modem.Modem) (*EUICCInfo, error) {
	ch, err := OpenISDRChannel(ctx, m)
	if err != nil {
		return &EUICCInfo{Supported: false}, nil
	}
	defer ch.Close(ctx)

	eid, err := mgr.queryEID(ctx, ch)
	if err != nil || eid == "" {
		// Fallback check: could be eUICC without EID readable via GetEUICCInfo1
		eid = "Unknown / Hidden"
	}

	profiles, err := mgr.queryProfiles(ctx, ch)
	if err != nil {
		profiles = []Profile{}
	}

	return &EUICCInfo{
		Supported: true,
		EID:       eid,
		Profiles:  profiles,
	}, nil
}

// queryEID retrieves the 32-digit EID from the eUICC chip (SGP.22 tag 5A).
func (mgr *Manager) queryEID(ctx context.Context, ch *Channel) (string, error) {
	// ES10c.GetEUICCInfo1 APDU: 80 E2 91 00 03 BF 2E 00
	payload, sw, err := ch.Transmit(ctx, "80E2910003BF2E00")
	if err != nil || (sw != 0x9000 && (sw>>8) != 0x90 && (sw>>8) != 0x61) {
		// Alternative: GetEUICCChallenge APDU: 80 E2 91 00 03 BF 2E 00
		payload, sw, err = ch.Transmit(ctx, "80E2910006BF3E035C015A00")
	}

	if len(payload) == 0 {
		return "", fmt.Errorf("empty EID payload, SW=%04X", sw)
	}

	nodes, err := ParseTLV(payload)
	if err != nil {
		return "", err
	}

	for _, n := range nodes {
		if eidNode := n.FindFirst(0x5A); eidNode != nil {
			return strings.ToUpper(hex.EncodeToString(eidNode.Value)), nil
		}
	}
	return "", nil
}

// queryProfiles retrieves the list of installed profiles (ES10c.GetProfilesInfo: 80 E2 91 00 03 BF 2D 00).
func (mgr *Manager) queryProfiles(ctx context.Context, ch *Channel) ([]Profile, error) {
	payload, sw, err := ch.Transmit(ctx, "80E2910003BF2D00")
	if err != nil {
		return nil, err
	}
	if sw != 0x9000 && (sw>>8) != 0x90 {
		return nil, fmt.Errorf("GetProfilesInfo returned SW=%04X", sw)
	}

	nodes, err := ParseTLV(payload)
	if err != nil {
		return nil, err
	}

	var list []Profile
	var profileNodes []*TLV

	for _, root := range nodes {
		profileNodes = append(profileNodes, root.FindAll(0xE3)...)
	}

	for _, pNode := range profileNodes {
		var p Profile
		p.State = "disabled"
		p.Class = "operational"

		// ICCID: tag 5A
		if iccidNode := pNode.FindFirst(0x5A); iccidNode != nil {
			p.ICCID = DecodeBCD(iccidNode.Value)
		}

		// State: tag 9F70 (01 = Enabled, 00 = Disabled)
		if stateNode := pNode.FindFirst(0x9F70); stateNode != nil && len(stateNode.Value) > 0 {
			if stateNode.Value[0] == 0x01 {
				p.State = "enabled"
			}
		}

		// Nickname: tag 90
		if nickNode := pNode.FindFirst(0x90); nickNode != nil {
			p.Nickname = string(nickNode.Value)
		}

		// Service Provider: tag 91
		if provNode := pNode.FindFirst(0x91); provNode != nil {
			p.Provider = string(provNode.Value)
		}

		// Profile Name: tag 92
		if nameNode := pNode.FindFirst(0x92); nameNode != nil {
			p.Name = string(nameNode.Value)
		}

		// Class: tag 95
		if classNode := pNode.FindFirst(0x95); classNode != nil && len(classNode.Value) > 0 {
			switch classNode.Value[0] {
			case 0x00:
				p.Class = "test"
			case 0x01:
				p.Class = "provisioning"
			case 0x02:
				p.Class = "operational"
			}
		}

		if p.ICCID != "" {
			list = append(list, p)
		}
	}

	return list, nil
}

// EnableProfile activates an eSIM profile and triggers card refresh.
func (mgr *Manager) EnableProfile(ctx context.Context, m *modem.Modem, iccid string) error {
	ch, err := OpenISDRChannel(ctx, m)
	if err != nil {
		return err
	}
	defer ch.Close(ctx)

	bcd, err := EncodeBCD(iccid)
	if err != nil {
		return fmt.Errorf("invalid ICCID %s: %w", iccid, err)
	}

	// SGP.22 ES10c.EnableProfileRequest:
	// BF 31 <len>
	//   5A <len> <iccid_bcd>
	//   81 01 01 (enableRefresh: true)
	inner := fmt.Sprintf("5A%02X%s810101", len(bcd), HexBytes(bcd))
	tlv := fmt.Sprintf("BF31%02X%s", len(inner)/2, inner)
	apdu := fmt.Sprintf("80E29100%02X%s", len(tlv)/2, tlv)

	_, sw, err := ch.Transmit(ctx, apdu)
	if err != nil {
		return err
	}
	if sw != 0x9000 && (sw>>8) != 0x90 {
		return fmt.Errorf("EnableProfile failed with SW=%04X", sw)
	}

	// Trigger module soft restart to reload SIM profile
	_ = ch.Close(ctx)
	_, _ = m.Raw(ctx, "AT+CFUN=1,1", 5*time.Second)

	return nil
}

// DisableProfile deactivates the specified profile.
func (mgr *Manager) DisableProfile(ctx context.Context, m *modem.Modem, iccid string) error {
	ch, err := OpenISDRChannel(ctx, m)
	if err != nil {
		return err
	}
	defer ch.Close(ctx)

	bcd, err := EncodeBCD(iccid)
	if err != nil {
		return fmt.Errorf("invalid ICCID %s: %w", iccid, err)
	}

	inner := fmt.Sprintf("5A%02X%s810101", len(bcd), HexBytes(bcd))
	tlv := fmt.Sprintf("BF32%02X%s", len(inner)/2, inner)
	apdu := fmt.Sprintf("80E29100%02X%s", len(tlv)/2, tlv)

	_, sw, err := ch.Transmit(ctx, apdu)
	if err != nil {
		return err
	}
	if sw != 0x9000 && (sw>>8) != 0x90 {
		return fmt.Errorf("DisableProfile failed with SW=%04X", sw)
	}

	_ = ch.Close(ctx)
	_, _ = m.Raw(ctx, "AT+CFUN=1,1", 5*time.Second)
	return nil
}

// DeleteProfile permanently removes an installed profile from the eUICC.
func (mgr *Manager) DeleteProfile(ctx context.Context, m *modem.Modem, iccid string) error {
	ch, err := OpenISDRChannel(ctx, m)
	if err != nil {
		return err
	}
	defer ch.Close(ctx)

	bcd, err := EncodeBCD(iccid)
	if err != nil {
		return fmt.Errorf("invalid ICCID %s: %w", iccid, err)
	}

	inner := fmt.Sprintf("5A%02X%s", len(bcd), HexBytes(bcd))
	tlv := fmt.Sprintf("BF33%02X%s", len(inner)/2, inner)
	apdu := fmt.Sprintf("80E29100%02X%s", len(tlv)/2, tlv)

	_, sw, err := ch.Transmit(ctx, apdu)
	if err != nil {
		return err
	}
	if sw != 0x9000 && (sw>>8) != 0x90 {
		return fmt.Errorf("DeleteProfile failed with SW=%04X", sw)
	}
	return nil
}

// SetNickname updates the user-friendly nickname of an installed profile.
func (mgr *Manager) SetNickname(ctx context.Context, m *modem.Modem, iccid, nickname string) error {
	ch, err := OpenISDRChannel(ctx, m)
	if err != nil {
		return err
	}
	defer ch.Close(ctx)

	bcd, err := EncodeBCD(iccid)
	if err != nil {
		return fmt.Errorf("invalid ICCID %s: %w", iccid, err)
	}

	nickBytes := []byte(nickname)
	inner := fmt.Sprintf("5A%02X%s90%02X%s", len(bcd), HexBytes(bcd), len(nickBytes), HexBytes(nickBytes))
	tlv := fmt.Sprintf("BF29%02X%s", len(inner)/2, inner)
	apdu := fmt.Sprintf("80E29100%02X%s", len(tlv)/2, tlv)

	_, sw, err := ch.Transmit(ctx, apdu)
	if err != nil {
		return err
	}
	if sw != 0x9000 && (sw>>8) != 0x90 {
		return fmt.Errorf("SetNickname failed with SW=%04X", sw)
	}
	return nil
}
