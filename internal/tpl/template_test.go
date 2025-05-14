package tpl

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mailtmpl-*.tmpl")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	return f.Name()
}

func TestParseFile_SubAlias(t *testing.T) {
	tmpl := `From: alice@example.com
To: bob@example.com
Sub: テストメール

こんにちは、{{ .Name }} さん!`

	path := writeTempFile(t, tmpl)
	tpl, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	if got := tpl.Subject(); got != "テストメール" {
		t.Errorf("Subject = %q, want %q", got, "テストメール")
	}
	if got := tpl.From(); got != "alice@example.com" {
		t.Errorf("From = %q, want %q", got, "alice@example.com")
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, map[string]string{"Name": "Bob"}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	body := strings.TrimSpace(buf.String())
	wantBody := "こんにちは、Bob さん!"
	if body != wantBody {
		t.Errorf("body = %q, want %q", body, wantBody)
	}
}

func TestParseFile_SubjectHeader(t *testing.T) {
	tmpl := `From: carol@example.com
To:    dave@example.com
Subject: 正式ヘッダ

Body line`

	path := writeTempFile(t, tmpl)
	tpl, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if got := tpl.Subject(); got != "正式ヘッダ" {
		t.Errorf("Subject = %q, want %q", got, "正式ヘッダ")
	}
}

func TestParseFile_MultipleTo(t *testing.T) {
	tmpl := `From: eva@example.com
To: user1@example.com, user2@example.com
Sub: multi to test

body`

	path := writeTempFile(t, tmpl)
	tpl, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	want := "user1@example.com, user2@example.com"
	if got := tpl.To(); got != want {
		t.Errorf("To = %q, want %q", got, want)
	}
}
