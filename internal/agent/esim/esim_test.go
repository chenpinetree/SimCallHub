package esim

import (
	"encoding/hex"
	"testing"
)

func TestBCD(t *testing.T) {
	iccid := "89860123456789012345"
	bcd, err := EncodeBCD(iccid)
	if err != nil {
		t.Fatalf("EncodeBCD error: %v", err)
	}

	decoded := DecodeBCD(bcd)
	if decoded != iccid {
		t.Fatalf("expected %s, got %s", iccid, decoded)
	}
}

func TestTLVParse(t *testing.T) {
	// Sample SGP.22 ProfileInfo TLV (Tag E3 containing 5A ICCID and 9F70 State)
	rawHex := "E3115A0A986810325476981032549F700101"
	data, _ := hex.DecodeString(rawHex)

	nodes, err := ParseTLV(data)
	if err != nil {
		t.Fatalf("ParseTLV error: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 root node, got %d", len(nodes))
	}

	root := nodes[0]
	if root.Tag != 0xE3 {
		t.Errorf("expected tag 0xE3, got 0x%X", root.Tag)
	}

	iccidNode := root.FindFirst(0x5A)
	if iccidNode == nil {
		t.Fatalf("did not find tag 5A")
	}
	iccid := DecodeBCD(iccidNode.Value)
	if iccid != "89860123456789012345" {
		t.Errorf("decoded ICCID mismatch: %s", iccid)
	}

	stateNode := root.FindFirst(0x9F70)
	if stateNode == nil || len(stateNode.Value) == 0 || stateNode.Value[0] != 0x01 {
		t.Errorf("state tag 9F70 mismatch")
	}
}

func TestXesimX7Payloads(t *testing.T) {
	// 1. Real BF3E (GetEuiccData EID) payload from Xesim X7 card
	eidPayload, _ := hex.DecodeString("BF3E125A1089086030202200000026000528521559")
	eid := extractEIDFromTLV(eidPayload)
	if eid != "89086030202200000026000528521559" {
		t.Fatalf("unexpected EID: %s", eid)
	}

	// 2. Real BF2D (GetProfilesInfo) payload from Xesim X7 card
	profPayload, _ := hex.DecodeString("BF2D35A033E3315A0A985802004136320792F54F10A0000005591010FFFFFFFF89000012009F7001019104436C75629204436C7562950102")
	profiles, err := ParseProfilesTLV(profPayload)
	if err != nil {
		t.Fatalf("ParseProfilesTLV error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	p := profiles[0]
	if p.ICCID != "8985200014632370295" || p.Provider != "Club" || p.Name != "Club" || p.State != "enabled" || p.Class != "operational" {
		t.Errorf("unexpected parsed profile: %+v", p)
	}

	// 3. Real BF22 (GetEUICCInfo2) payload from Xesim X7 card
	info2Payload, _ := hex.DecodeString("BF2281AA810302030182030202028303040200840D8101008204001047BA83023D338505077F3E1F808603090200870302030088020490A92C041481370F5125D0B1D408D4C3B232E6D25E795BEBFB0414C1D4AF8957F48CD51A47FEF73123A04011D3D69EAA2C041481370F5125D0B1D408D4C3B232E6D25E795BEBFB0414C1D4AF8957F48CD51A47FEF73123A04011D3D69E8B01009902064004030100000C0D45442D5A492D55502D30383238")
	var info EUICCInfo
	ParseEuiccInfo2TLV(info2Payload, &info)
	if info.Svn != "2.2.2" {
		t.Errorf("expected Svn 2.2.2, got %s", info.Svn)
	}
	if info.FirmwareVer != "4.2.0" {
		t.Errorf("expected FirmwareVer 4.2.0, got %s", info.FirmwareVer)
	}
	if info.FreeNVMBytes != 1066938 {
		t.Errorf("expected FreeNVMBytes 1066938, got %d", info.FreeNVMBytes)
	}
	if info.FreeRAMBytes != 15667 {
		t.Errorf("expected FreeRAMBytes 15667, got %d", info.FreeRAMBytes)
	}
	if info.SASNumber != "ED-ZI-UP-0828" {
		t.Errorf("expected SASNumber ED-ZI-UP-0828, got %s", info.SASNumber)
	}

	// 4. EnableProfile APDU structure with A0 [0] CHOICE wrapper around 5A ICCID
	bcd, err := EncodeBCD("8985200014632370295")
	if err != nil {
		t.Fatalf("EncodeBCD error: %v", err)
	}
	apdu := buildProfileStateAPDU("BF31", bcd)
	wantAPDU := "80E2910014BF3111A00C5A0A985802004136320792F581010100"
	if apdu != wantAPDU {
		t.Errorf("buildProfileStateAPDU = %s, want %s", apdu, wantAPDU)
	}

	// 5. Status word 910B (Proactive REFRESH pending) is treated as success
	if !IsSuccessSW(0x9000) || !IsSuccessSW(0x910B) || IsSuccessSW(0x6A82) {
		t.Errorf("IsSuccessSW check failed")
	}

	// 6. parseCCHOSession ignores ERROR and extracts valid session ID
	if id := parseCCHOSession([]string{"ERROR"}); id != -1 {
		t.Errorf("expected -1 for ERROR, got %d", id)
	}
	if id := parseCCHOSession([]string{"+CCHO: 1", "OK"}); id != 1 {
		t.Errorf("expected 1 for +CCHO: 1, got %d", id)
	}

	// 7. Real Xesim multi-profile BF2D payload with Club (enabled) and Kolet (disabled)
	multiPayload, _ := hex.DecodeString("BF2D81B1A081AEE3315A0A985802004136320792F54F10A0000005591010FFFFFFFF89000012009F7001019104436C75629204436C7562950102E3795A0A988410000230763561F64F10A0000005591010FFFFFFFF89000013009F70010090054B6F6C657491054B6F6C657492054B6F6C6574950102BF763CE23AE12CC1146750ADBF4600909A90704D9C4497BE5080F3E09DCA14636F6D2E6F64792E636F6E73756D65722E617070E30ADB080000000000000001")
	mProfiles, err := ParseProfilesTLV(multiPayload)
	if err != nil {
		t.Fatalf("ParseProfilesTLV multi error: %v", err)
	}
	if len(mProfiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(mProfiles))
	}
	if mProfiles[0].ICCID != "8985200014632370295" || mProfiles[0].State != "enabled" || mProfiles[0].Provider != "Club" {
		t.Errorf("unexpected profile 0: %+v", mProfiles[0])
	}
	if mProfiles[1].ICCID != "8948010020036753166" || mProfiles[1].State != "disabled" || mProfiles[1].Provider != "Kolet" || mProfiles[1].Nickname != "Kolet" {
		t.Errorf("unexpected profile 1: %+v", mProfiles[1])
	}
}
