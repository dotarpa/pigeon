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

func TestParseFile_TemplateVariables(t *testing.T) {
	tmpl := `From: {{ .From }}
To: {{ .To }}
Sub: {{ .Subject }}

Hello {{ .Name }},
This is a test message.`

	path := writeTempFile(t, tmpl)
	tpl, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	// Test that template variables are preserved in headers
	if got := tpl.From(); got != "{{ .From }}" {
		t.Errorf("From = %q, want %q", got, "{{ .From }}")
	}
	if got := tpl.To(); got != "{{ .To }}" {
		t.Errorf("To = %q, want %q", got, "{{ .To }}")
	}
	if got := tpl.Subject(); got != "{{ .Subject }}" {
		t.Errorf("Subject = %q, want %q", got, "{{ .Subject }}")
	}

	// Test template execution with data
	data := map[string]string{
		"From":    "sender@example.com",
		"To":      "receiver@example.com",
		"Subject": "Test Message",
		"Name":    "Alice",
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	body := strings.TrimSpace(buf.String())
	wantBody := "Hello Alice,\nThis is a test message."
	if body != wantBody {
		t.Errorf("body = %q, want %q", body, wantBody)
	}
}

func TestParseFile_MixedTemplateAndStatic(t *testing.T) {
	tmpl := `From: {{ .From }}
To: static@example.com
Cc: {{ .Cc }}
Sub: Mixed template test

Message for {{ .Recipient }}`

	path := writeTempFile(t, tmpl)
	tpl, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	// Test mixed template and static values
	if got := tpl.From(); got != "{{ .From }}" {
		t.Errorf("From = %q, want %q", got, "{{ .From }}")
	}
	if got := tpl.To(); got != "static@example.com" {
		t.Errorf("To = %q, want %q", got, "static@example.com")
	}
	if got := tpl.Cc(); got != "{{ .Cc }}" {
		t.Errorf("Cc = %q, want %q", got, "{{ .Cc }}")
	}

	data := map[string]string{
		"From":      "admin@example.com",
		"Cc":        "manager@example.com",
		"Recipient": "John",
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	result := buf.String()
	t.Logf("Execute result: %q", result)

	body := strings.TrimSpace(buf.String())
	wantBody := "Message for John"
	if body != wantBody {
		t.Errorf("body = %q, want %q", body, wantBody)
	}
}

func TestParseFile_HeaderTemplateExecution(t *testing.T) {
	tmpl := `From: {{ .From }}
To: {{ .To }}
Sub: {{ .Subject }}

Hello {{ .Name }},
This is a test message.`

	path := writeTempFile(t, tmpl)
	tpl, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	data := map[string]string{
		"From":    "sender@example.com",
		"To":      "receiver@example.com",
		"Subject": "Test Message",
		"Name":    "Alice",
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	result := buf.String()
	t.Logf("Execute result: %q", result)

	// tpl.Execute returns only the body, so check body template variables only
	if !strings.Contains(result, "Hello Alice,") {
		t.Errorf("Expected body to be replaced, got: %s", result)
	}
	if !strings.Contains(result, "This is a test message.") {
		t.Errorf("Expected body to contain message, got: %s", result)
	}

	// Header parts are not processed by tpl.Execute and need to be retrieved individually
	// This is handled separately in the email.go Send function
}
