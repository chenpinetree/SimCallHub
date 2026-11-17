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
