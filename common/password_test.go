package common

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	enc, err := HashPassword("s3cret-pass")
	if err != nil {
		t.Fatal(err)
	}
	if !IsPasswordHash(enc) {
		t.Fatalf("encoded hash not recognized: %q", enc)
	}
	if VerifyPassword(enc, "s3cret-pass") != true {
		t.Fatal("correct password should verify")
	}
	if VerifyPassword(enc, "wrong") {
		t.Fatal("wrong password must not verify")
	}
	// Distinct salts → distinct encodings for the same password.
	enc2, _ := HashPassword("s3cret-pass")
	if enc == enc2 {
		t.Fatal("salts should differ between hashes")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	for _, bad := range []string{"", "plaintext", "pbkdf2-sha256$0$aa$bb", "md5$1$aa$bb", "pbkdf2-sha256$x$aa$bb"} {
		if VerifyPassword(bad, "x") {
			t.Fatalf("malformed hash must fail closed: %q", bad)
		}
	}
}

func TestHashPasswordEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("empty password must be rejected")
	}
}

func TestRandomPassword(t *testing.T) {
	a, b := RandomPassword(), RandomPassword()
	if a == "" || a == b {
		t.Fatalf("random passwords should be non-empty and unique: %q %q", a, b)
	}
}
