package pigeon

import (
	"os"
	"strings"
	"testing"
)

func TestLoadAndString(t *testing.T) {
	yaml := `
from: alice@example.com
to: bob@example.com
smarthost: smtp.example.com:2525
auth_username: alice
auth_password: s3cr3t
attachments:
  - "/tmp/file1.txt"
headers:
  X-Test: test-header
`
	cfg, err := Load(yaml)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.From != "alice@example.com" {
		t.Errorf("From mismatch: %v", cfg.From)
	}
	if cfg.Smarthost.Host != "smtp.example.com" || cfg.Smarthost.Port != "2525" {
		t.Errorf("Smarthost parse error: %v", cfg.Smarthost)
	}
	if cfg.AuthUsername != "alice" {
		t.Errorf("AuthUsername mismatch")
	}
	if string(cfg.AuthPassword) != "s3cr3t" {
		t.Errorf("AuthPassword mismatch")
	}
	if len(cfg.Attachments) != 1 || cfg.Attachments[0] != "/tmp/file1.txt" {
		t.Errorf("Attachments parse error: %v", cfg.Attachments)
	}
	if v, ok := cfg.Headers["X-Test"]; !ok || v != "test-header" {
		t.Errorf("Headers parse error: %v", cfg.Headers)
	}
	// String() returns yaml with <secret>
	s := cfg.String()
	if !strings.Contains(s, "<secret>") {
		t.Errorf("String() did not redact secret: %s", s)
	}
}

func TestLoadFile(t *testing.T) {
	content := `
from: test@example.com
smarthost: mail:2525
`
	fname := "test_configfile.yaml"
	err := os.WriteFile(fname, []byte(content), 0600)
	if err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	defer os.Remove(fname)
	cfg, err := LoadFile(fname)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}
	if cfg.From != "test@example.com" || cfg.Smarthost.Host != "mail" || cfg.Smarthost.Port != "2525" {
		t.Errorf("LoadFile parse error: %+v", cfg)
	}
}
