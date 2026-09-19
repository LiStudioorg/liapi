package common

import "testing"

func TestTokenEqual(t *testing.T) {
	if !TokenEqual("abc", "abc") {
		t.Fatal("equal tokens should match")
	}
	if TokenEqual("abc", "abd") {
		t.Fatal("different tokens should not match")
	}
	if TokenEqual("", "abc") {
		t.Fatal("empty vs non-empty should not match")
	}
}

func TestMaskToken(t *testing.T) {
	if got := MaskToken("sk-abcdefghijklm"); got != "sk-a...jklm" {
		t.Fatalf("unexpected mask: %q", got)
	}
	if got := MaskToken("short"); got != "***" {
		t.Fatalf("short token should be fully masked: %q", got)
	}
}

func TestRandomToken(t *testing.T) {
	a := RandomToken("sk-")
	b := RandomToken("sk-")
	if a == b {
		t.Fatal("random tokens should differ")
	}
	if len(a) < 10 {
		t.Fatalf("token too short: %q", a)
	}
}
