package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"liapi/common"
)

func boolPtr(b bool) *bool { return &b }

// A fresh config must NOT generate any admin credentials: no password hash,
// login disabled until the user opts in.
func TestNoAutoPassword(t *testing.T) {
	c := validConfig()
	c.AdminPasswordHash = ""
	c.AdminPassword = ""
	c.LoginEnabled = nil
	if err := c.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if c.AdminPasswordHash != "" {
		t.Fatalf("expected no auto-generated hash, got %q", c.AdminPasswordHash)
	}
	if c.AdminPassword != "" {
		t.Fatalf("plaintext password must stay empty, got %q", c.AdminPassword)
	}
	if c.LoginAllowed() {
		t.Fatal("login must be disabled with no password set")
	}
}

// login_enabled=true without a password is rejected.
func TestLoginEnabledRequiresPassword(t *testing.T) {
	c := validConfig()
	c.AdminPasswordHash = ""
	c.LoginEnabled = boolPtr(true)
	if err := c.Validate(); err == nil {
		t.Fatal("expected error: login_enabled=true needs a password")
	}
}

// A plaintext admin_password is hashed and cleared in place.
func TestAdminPasswordHashed(t *testing.T) {
	c := validConfig()
	c.AdminPasswordHash = ""
	c.AdminPassword = "s3cret-pw"
	c.LoginEnabled = boolPtr(true)
	if err := c.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if c.AdminPassword != "" {
		t.Fatal("plaintext must be cleared after hashing")
	}
	if !common.IsPasswordHash(c.AdminPasswordHash) {
		t.Fatalf("expected pbkdf2 hash, got %q", c.AdminPasswordHash)
	}
	if !common.VerifyPassword(c.AdminPasswordHash, "s3cret-pw") {
		t.Fatal("hash does not verify against original password")
	}
	if !c.LoginAllowed() {
		t.Fatal("login should be allowed")
	}
}

// An explicit login_enabled=false refuses password login even when a hash
// exists (static admin_token keeps working — tested at the auth layer).
func TestLoginDisabledExplicit(t *testing.T) {
	c := validConfig()
	c.AdminPasswordHash = ""
	c.AdminPassword = "pw-123456"
	if err := c.Validate(); err != nil { // hash now set
		t.Fatalf("validate: %v", err)
	}
	c.LoginEnabled = boolPtr(false)
	if c.LoginAllowed() {
		t.Fatal("login must be disabled when explicitly switched off")
	}
}

func TestListenAddr(t *testing.T) {
	cases := []struct {
		addr string
		port int
		want string
	}{
		{"", 0, "0.0.0.0:8787"},      // legacy defaults
		{":8787", 0, "0.0.0.0:8787"}, // legacy addr with port
		{"127.0.0.1", 9000, "127.0.0.1:9000"},
		{"0.0.0.0", 0, "0.0.0.0:8787"},
		{"127.0.0.1:1234", 0, "127.0.0.1:1234"}, // legacy addr wins when port=0
		{"127.0.0.1:1234", 5555, "127.0.0.1:5555"},
		{"::1", 8787, "[::1]:8787"},
	}
	for _, tc := range cases {
		c := &Config{Addr: tc.addr, Port: tc.port}
		if got := c.ListenAddr(); got != tc.want {
			t.Errorf("addr=%q port=%d: got %q want %q", tc.addr, tc.port, got, tc.want)
		}
	}
}

func TestListenValidation(t *testing.T) {
	c := validConfig()
	c.Port = 70000
	if err := c.Validate(); err == nil {
		t.Fatal("port > 65535 must be rejected")
	}
	c = validConfig()
	c.Addr = "http://0.0.0.0"
	if err := c.Validate(); err == nil {
		t.Fatal("addr with scheme must be rejected")
	}
}

// Load on a missing file creates the parent dir (0700) and a 0600 config
// without any generated password, and persists addr/port.
func TestLoadCreatesDefaultConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "liapi")
	path := filepath.Join(dir, "config.json")
	c, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config mode %v, want 0600", fi.Mode().Perm())
	}
	if c.AdminPasswordHash != "" || c.AdminPassword != "" {
		t.Fatalf("no credentials expected, got hash=%q pw=%q", c.AdminPasswordHash, c.AdminPassword)
	}
	if c.Addr != DefaultHost || c.Port != DefaultPort {
		t.Fatalf("persisted listen defaults = %q/%d, want %s/%d", c.Addr, c.Port, DefaultHost, DefaultPort)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "admin_password\"") {
		t.Fatal("config file must not contain an admin_password field when unset")
	}
}

// Load with a hand-written plaintext admin_password hashes it and rewrites
// the file immediately — the plaintext must not survive on disk.
func TestLoadHashesPlaintextFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// Bootstrap a file through Load once (random token etc.)...
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	// ...then hand-edit: plaintext password, no hash.
	raw, _ := os.ReadFile(path)
	c := map[string]any{}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	delete(c, "admin_password_hash")
	c["admin_password"] = "hand-edited-99"
	delete(c, "login_enabled")
	out, _ := json.Marshal(c)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !common.VerifyPassword(cfg.AdminPasswordHash, "hand-edited-99") {
		t.Fatal("hash does not verify against the hand-edited password")
	}
	onDisk, _ := os.ReadFile(path)
	if strings.Contains(string(onDisk), "hand-edited-99") {
		t.Fatal("plaintext persisted on disk after Load")
	}
	if !strings.Contains(string(onDisk), "pbkdf2-sha256$") {
		t.Fatal("rewritten file lacks the hash")
	}
}

// DataDir honours LIAPI_HOME so tests never touch the real home directory.
func TestDataDirEnv(t *testing.T) {
	t.Setenv("LIAPI_HOME", filepath.Join(t.TempDir(), "home"))
	if got := DataDir(); !strings.HasSuffix(got, "home") {
		t.Fatalf("DataDir=%s, expected LIAPI_HOME override", got)
	}
	if got := DefaultConfigPath(); filepath.Base(got) != "config.json" {
		t.Fatalf("DefaultConfigPath=%s", got)
	}
}
