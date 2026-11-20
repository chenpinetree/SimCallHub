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
	ISDPAID  string `json:"isdpAid,omitempty"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Nickname string `json:"nickname"`
	State    string `json:"state"` // "enabled" | "disabled"
	Class    string `json:"class"` // "operational" | "test" | "provisioning"
}

// EUICCInfo reports the eUICC chip identity, hardware specifications, storage capacity, and installed profiles.
type EUICCInfo struct {
	Supported     bool      `json:"supported"`
	EID           string    `json:"eid"`
	AID           string    `json:"aid,omitempty"`
	CardBrand     string    `json:"cardBrand,omitempty"`
	Svn           string    `json:"svn,omitempty"`
	FirmwareVer   string    `json:"firmwareVer,omitempty"`
	JavaCardVer   string    `json:"javaCardVer,omitempty"`
	SASNumber     string    `json:"sasNumber,omitempty"`
	FreeNVMBytes  int       `json:"freeNvmBytes,omitempty"`
	FreeRAMBytes  int       `json:"freeRamBytes,omitempty"`
	InstalledApps int       `json:"installedApps,omitempty"`
	Profiles      []Profile `json:"profiles"`
}

// Manager orchestrates SGP.22 eSIM profile operations over the 3GPP AT interface.
type Manager struct{}

// NewManager creates a new eSIM manager.
func NewManager() *Manager {
	return &Manager{}
}

// detectCardBrand identifies the eSIM hardware vendor from the matched ISD-R AID.
func detectCardBrand(aid string) string {
	switch strings.ToUpper(aid) {
	case ISDRAIDXesimX7, ISDRAIDXesimAlt:
		return "Xesim eUICC (X7 / X2 系列)"
	case ISDRAIDStandard:
		return "GSMA SGP.22 标准 eUICC"
	case ISDRAIDTest:
		return "SGP.22 测试 eUICC"
	default:
		return "可插拔 eUICC / eSIM 芯片"
	}
}

// GetInfo checks if the current SIM is an eUICC and retrieves its EID, EUICCInfo2 metadata, and profile list.
func (mgr *Manager) GetInfo(ctx context.Context, m *modem.Modem) (*EUICCInfo, error) {
	ch, err := OpenISDRChannel(ctx, m)
	if err != nil {
		return &EUICCInfo{Supported: false}, nil
	}
	defer ch.Close(ctx)

	eid, err := mgr.queryEID(ctx, ch)
	if err != nil || eid == "" {
		eid = "Unknown / Hidden"
	}

	profiles, err := mgr.queryProfiles(ctx, ch)
	if err != nil {
		profiles = []Profile{}
	}

	info := &EUICCInfo{
		Supported: true,
		EID:       eid,
		AID:       ch.AID,
		CardBrand: detectCardBrand(ch.AID),
		Profiles:  profiles,
	}

	// Enrich with SGP.22 ES10b.GetEUICCInfo2 (firmware version, SGP.22 SVN, free storage space, SAS number)
	mgr.populateEuiccInfo2(ctx, ch, info)

	return info, nil
}

// queryEID retrieves the 32-digit EID from the eUICC chip (SGP.22 ES10c.GetEuiccData tag BF3E -> tag 5A).
func (mgr *Manager) queryEID(ctx context.Context, ch *Channel) (string, error) {
	// Primary SGP.22 ES10c.GetEuiccDataRequest (BF3E with TagList 5C 01 5A) as used in Xesim LPA
	payload, sw, err := ch.Transmit(ctx, "80E2910006BF3E035C015A00")
	if err == nil && IsSuccessSW(sw) && len(payload) > 0 {
		if eid := extractEIDFromTLV(payload); eid != "" {
			return eid, nil
		}
	}

	// Fallback: try without trailing Le byte
	payload, sw, err = ch.Transmit(ctx, "80E2910006BF3E035C015A")
	if err == nil && IsSuccessSW(sw) && len(payload) > 0 {
		if eid := extractEIDFromTLV(payload); eid != "" {
			return eid, nil
		}
	}

	return "", fmt.Errorf("unable to retrieve EID (SW=%04X)", sw)
}

func extractEIDFromTLV(payload []byte) string {
	nodes, err := ParseTLV(payload)
	if err != nil {
		return ""
	}
	for _, n := range nodes {
		if eidNode := n.FindFirst(0x5A); eidNode != nil && len(eidNode.Value) > 0 {
			return strings.ToUpper(hex.EncodeToString(eidNode.Value))
		}
	}
	return ""
}

// populateEuiccInfo2 queries ES10b.GetEUICCInfo2 (tag BF22) and decodes version & free memory metrics.
func (mgr *Manager) populateEuiccInfo2(ctx context.Context, ch *Channel, info *EUICCInfo) {
	payload, sw, err := ch.Transmit(ctx, "80E2910003BF2200")
	if err != nil || !IsSuccessSW(sw) || len(payload) == 0 {
		return
	}
	ParseEuiccInfo2TLV(payload, info)
}

// ParseEuiccInfo2TLV parses a raw BF22 EUICCInfo2 BER-TLV payload into info.
func ParseEuiccInfo2TLV(payload []byte, info *EUICCInfo) {
	nodes, err := ParseTLV(payload)
	if err != nil || len(nodes) == 0 {
		return
	}

	var root *TLV
	for _, n := range nodes {
		if n.Tag == 0xBF22 {
			root = n
			break
		}
	}
	if root == nil {
		root = nodes[0]
	}

	for _, child := range root.Children {
		switch child.Tag {
		case 0x82: // svn (VersionType, 3 bytes e.g. 02 02 02 -> "2.2.2")
			if v := formatVersionBytes(child.Value); v != "" {
				info.Svn = v
			}
		case 0x83: // euiccFirmwareVer (VersionType, 3 bytes e.g. 04 02 00 -> "4.2.0")
			if v := formatVersionBytes(child.Value); v != "" {
				info.FirmwareVer = v
			}
		case 0x84: // extCardResource (ETSI TS 102 226 ExtendedCardResourcesInformation)
			parseExtCardResource(child.Value, info)
		case 0x86: // javacardVersion
			if v := formatVersionBytes(child.Value); v != "" {
				info.JavaCardVer = v
			}
		case 0x0C: // sasAcreditationNumber (UTF8String)
			info.SASNumber = strings.TrimSpace(string(child.Value))
		}
	}
}

func formatVersionBytes(b []byte) string {
	if len(b) == 3 {
		return fmt.Sprintf("%d.%d.%d", b[0], b[1], b[2])
	}
	return ""
}

// parseExtCardResource decodes tag 84 inside EUICCInfo2:
// 81: installedApplication, 82: freeNonVolatileMemory, 83: freeVolatileMemory.
func parseExtCardResource(data []byte, info *EUICCInfo) {
	subNodes, err := ParseTLV(data)
	if err != nil {
		return
	}
	for _, sn := range subNodes {
		switch sn.Tag {
		case 0x81:
			info.InstalledApps = decodeBigEndianInt(sn.Value)
		case 0x82:
			info.FreeNVMBytes = decodeBigEndianInt(sn.Value)
		case 0x83:
			info.FreeRAMBytes = decodeBigEndianInt(sn.Value)
		}
	}
}

func decodeBigEndianInt(b []byte) int {
	val := 0
	for _, v := range b {
		val = (val << 8) | int(v)
	}
	return val
}

// queryProfiles retrieves the list of installed profiles (ES10c.GetProfilesInfo: BF2D).
func (mgr *Manager) queryProfiles(ctx context.Context, ch *Channel) ([]Profile, error) {
	// Try standard empty ProfileInfoListRequest first (returns all tags on Xesim X7 and standard eUICCs)
	payload, sw, err := ch.Transmit(ctx, "80E2910003BF2D00")
	if err != nil || !IsSuccessSW(sw) || len(payload) == 0 {
		// Fallback to explicit TagList (5C 07 5A 9F70 91 92 90 95) as in Xesim LPA
		payload, sw, err = ch.Transmit(ctx, "80E291000CBF2D095C075A9F709192909500")
		if err != nil {
			return nil, err
		}
		if !IsSuccessSW(sw) {
			return nil, fmt.Errorf("GetProfilesInfo returned SW=%04X", sw)
		}
	}

	return ParseProfilesTLV(payload)
}

// ParseProfilesTLV decodes an ES10c.GetProfilesInfo (BF2D) BER-TLV response into Profile items.
func ParseProfilesTLV(payload []byte) ([]Profile, error) {
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

		// ISD-P AID: tag 4F
		if aidNode := pNode.FindFirst(0x4F); aidNode != nil && len(aidNode.Value) > 0 {
			p.ISDPAID = HexBytes(aidNode.Value)
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

// buildProfileStateAPDU constructs the SGP.22 ES10c EnableProfileRequest (BF31) or DisableProfileRequest (BF32).
// Per SGP.22 and Xesim LPA (La2/a), profileIdentifier is a CHOICE [0] (tag A0) wrapping iccid (tag 5A):
//
//	BF31/BF32 <len>
//	  A0 <len>
//	    5A 0A <iccid_bcd>
//	  81 01 01 (refreshFlag: true)
func buildProfileStateAPDU(tagHex string, bcd []byte) string {
	iccidTLV := fmt.Sprintf("5A%02X%s", len(bcd), HexBytes(bcd))
	profileIDTLV := fmt.Sprintf("A0%02X%s", len(iccidTLV)/2, iccidTLV)
	inner := profileIDTLV + "810101"
	tlv := fmt.Sprintf("%s%02X%s", tagHex, len(inner)/2, inner)
	return fmt.Sprintf("80E29100%02X%s00", len(tlv)/2, tlv)
}

// checkOperationResult verifies the ASN.1 integer result tag (0x80) inside Enable/Disable/Delete/SetNickname responses.
// Result code 0x00 indicates OK; non-zero indicates an eUICC error code.
func checkOperationResult(opName string, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	nodes, err := ParseTLV(payload)
	if err != nil {
		return nil
	}
	for _, root := range nodes {
		if resNode := root.FindFirst(0x80); resNode != nil && len(resNode.Value) > 0 {
			code := int(resNode.Value[0])
			if code != 0 {
				return fmt.Errorf("%s rejected by eUICC (result code %d)", opName, code)
			}
			return nil
		}
	}
	return nil
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

	apdu := buildProfileStateAPDU("BF31", bcd)
	payload, sw, err := ch.Transmit(ctx, apdu)
	if err != nil {
		return err
	}
	if !IsSuccessSW(sw) {
		return fmt.Errorf("EnableProfile failed with SW=%04X", sw)
	}
	if err := checkOperationResult("EnableProfile", payload); err != nil {
		return err
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

	apdu := buildProfileStateAPDU("BF32", bcd)
	payload, sw, err := ch.Transmit(ctx, apdu)
	if err != nil {
		return err
	}
	if !IsSuccessSW(sw) {
		return fmt.Errorf("DisableProfile failed with SW=%04X", sw)
	}
	if err := checkOperationResult("DisableProfile", payload); err != nil {
		return err
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

	// SGP.22 DeleteProfileRequest (BF33) is a direct CHOICE { isdpAid [4F], iccid [5A] }
	inner := fmt.Sprintf("5A%02X%s", len(bcd), HexBytes(bcd))
	tlv := fmt.Sprintf("BF33%02X%s", len(inner)/2, inner)
	apdu := fmt.Sprintf("80E29100%02X%s00", len(tlv)/2, tlv)

	payload, sw, err := ch.Transmit(ctx, apdu)
	if err != nil {
		return err
	}
	if !IsSuccessSW(sw) {
		return fmt.Errorf("DeleteProfile failed with SW=%04X", sw)
	}
	return checkOperationResult("DeleteProfile", payload)
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

	payload, sw, err := ch.TransmitStoreData(ctx, tlv)
	if err != nil {
		return err
	}
	if !IsSuccessSW(sw) {
		return fmt.Errorf("SetNickname failed with SW=%04X", sw)
	}
	return checkOperationResult("SetNickname", payload)
}
