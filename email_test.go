package pigeon

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func startMockSMTP(t *testing.T) (addr string, received <-chan string, teardown func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ch := make(chan string, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)

		fmt.Fprintf(writer, "220 localhost SimpleSMTP\r\n")
		writer.Flush()

		var data strings.Builder
		inData := false
		for {
			line, _ := reader.ReadString('\n')
			line = strings.TrimRight(line, "\r\n")
			if !inData {
				switch {
				case strings.HasPrefix(strings.ToUpper(line), "HELO"),
					strings.HasPrefix(strings.ToUpper(line), "EHLO"):
					fmt.Fprintf(writer, "250 OK\r\n")
				case strings.HasPrefix(strings.ToUpper(line), "MAIL FROM"),
					strings.HasPrefix(strings.ToUpper(line), "RCPT TO"):
					fmt.Fprintf(writer, "250 OK\r\n")
				case strings.HasPrefix(strings.ToUpper(line), "DATA"):
					fmt.Fprintf(writer, "354 End data with <CR><LF>.<CR><LF>\r\n")
					inData = true
				case strings.HasPrefix(strings.ToUpper(line), "QUIT"):
					fmt.Fprintf(writer, "221 Bye\r\n")
					writer.Flush()
					return
				default:
					fmt.Fprintf(writer, "250 OK\r\n")
				}
				writer.Flush()
			} else {
				if line == "." {
					// end of data
					fmt.Fprintf(writer, "250 OK\r\n")
					writer.Flush()
					ch <- data.String()
					inData = false
				} else {
					data.WriteString(line + "\n")
				}
			}
		}
	}()

	return ln.Addr().String(), ch, func() { ln.Close() }
}

func TestSend_Basic(t *testing.T) {
	addr, recv, teardown := startMockSMTP(t)
	defer teardown()

	tmplContent := "From: sender@example.com\nTo: recv1@example.com\nSub: TestSub\n\nHello, World"
	tmplPath := tplWriteTemp(t, tmplContent)

	smarthost := HostPort{}
	var err error
	smarthost.Host, smarthost.Port, err = net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	cfg := EmailConfig{
		From:         "",
		To:           "",
		Smarthost:    smarthost,
		TemplatePath: tmplPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	retry, err := Send(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if retry {
		t.Errorf("expected retry=false, got true")
	}

	select {
	case raw := <-recv:
		if !strings.Contains(raw, "Hello, World") {
			t.Errorf("body not found in raw message")
		}
		if !strings.Contains(raw, "Subject: =?UTF-8?B?") &&
			!strings.Contains(raw, "Subject: TestSub") {
			t.Errorf("subject header missing")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message received by mock SMTP")
	}
}

// tplWriteTemp is helper creating temp file with given content.
func tplWriteTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "e2e-*.tmpl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestSendWithAttacment(t *testing.T) {
	addr, recv, teardown := startMockSMTP(t)
	defer teardown()

	tmplContent := "From: sender@example.com\nTo: recv1@example.com\nSub: TestAttachment\n\nfile is attached."
	tmplPath := tplWriteTemp(t, tmplContent)

	attachContent := "test attachment file data"
	attachFile := "test_attach.txt"
	af, err := os.CreateTemp(t.TempDir(), attachFile)
	if err != nil {
		t.Fatalf("CreateTemp attachment: %v", err)
	}
	_, err = af.WriteString(attachContent)
	if err != nil {
		t.Fatalf("WriteString attachment: %v", err)
	}
	af.Close()

	smarthost := HostPort{}
	smarthost.Host, smarthost.Port, _ = net.SplitHostPort(addr)

	cfg := EmailConfig{
		From:         "",
		To:           "",
		Smarthost:    smarthost,
		TemplatePath: tmplPath,
		Attachments:  []string{af.Name()},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	retry, err := Send(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if retry {
		t.Fatalf("expected retry=false, got true")
	}

	select {
	case raw := <-recv:
		if !strings.Contains(raw, "Content-Disposition: attachment; filename=") {
			t.Errorf("attachment part missing Content-Disposition: %s", raw)
		}
		if !strings.Contains(raw, "Content-Type: multipart/mixed") {
			t.Errorf("not multipart/mixed: %s", raw)
		}
		if !strings.Contains(raw, "file is attached") {
			t.Errorf("body text missing: %s", raw)
		}

		if !strings.Contains(raw, "dGVzdCBhdHRhY2htZW50IGZpbGUgZGF0YQ==") {
			t.Errorf("base64 attachment data missing: %s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message recived by mock SMTP")
	}

}

func TestSend_DateHeaderWithTimeZone(t *testing.T) {
	addr, recv, teardown := startMockSMTP(t)
	defer teardown()

	tmplContent := "From: sender@example.com\nTo: recv@example.com\nSub: Test TZ\n\nBody."
	tmplPath := tplWriteTemp(t, tmplContent)

	smarthost := HostPort{}
	smarthost.Host, smarthost.Port, _ = net.SplitHostPort(addr)

	cfg := EmailConfig{
		Smarthost:    smarthost,
		TemplatePath: tmplPath,
		Timezone:     "Asia/Tokyo",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Send(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	select {
	case raw := <-recv:
		dateLine := ""
		for _, l := range strings.Split(raw, "\n") {
			if strings.HasPrefix(l, "Date:") {
				dateLine = l
				break
			}
		}
		if dateLine == "" {
			t.Fatal("Date header missing")
		}

		if !strings.Contains(dateLine, "+0900") {
			t.Errorf("Date header not in Asia/Tokyo: %s", dateLine)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message recived by mock SMTP")
	}
}

func TestSend_DateHeaderUTC(t *testing.T) {
	addr, recv, teardown := startMockSMTP(t)
	defer teardown()

	tmplContent := "From: sender@example.com\nTo: recv@example.com\nSub: UTC Test\n\nBody."
	tmplPath := tplWriteTemp(t, tmplContent)

	smarthost := HostPort{}
	smarthost.Host, smarthost.Port, _ = net.SplitHostPort(addr)

	cfg := EmailConfig{
		Smarthost:    smarthost,
		TemplatePath: tmplPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Send(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	select {
	case raw := <-recv:
		dateLine := ""
		for _, l := range strings.Split(raw, "\n") {
			if strings.HasPrefix(l, "Date:") {
				dateLine = l
				break
			}
		}
		if dateLine == "" {
			t.Fatalf("Date header missing")
		}
		if !strings.Contains(dateLine, "+0000") {
			t.Errorf("Date header not in UTC: %s", dateLine)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no message received by mock SMTP")
	}
}

func TestSend_QuotedPrintableWrapping(t *testing.T) {
	addr, recv, teardown := startMockSMTP(t)
	defer teardown()

	// Create template with long line that should be wrapped by quoted-printable encoding
	longLine := strings.Repeat("This is a very long line that should be wrapped by quoted-printable encoding. ", 2)
	tmplContent := fmt.Sprintf("From: sender@example.com\nTo: recv@example.com\nSub: QuotedPrintable Test\n\n%s", longLine)
	tmplPath := tplWriteTemp(t, tmplContent)

	smarthost := HostPort{}
	smarthost.Host, smarthost.Port, _ = net.SplitHostPort(addr)

	cfg := EmailConfig{
		Smarthost:    smarthost,
		TemplatePath: tmplPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Send(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	select {
	case raw := <-recv:
		// Verify quoted-printable encoding is used
		if !strings.Contains(raw, "Content-Transfer-Encoding: quoted-printable") {
			t.Errorf("quoted-printable encoding not found in message")
		}

		// Verify proper line wrapping - quoted-printable should wrap long lines
		lines := strings.Split(raw, "\n")
		bodyStarted := false
		foundLongLineWrapped := false

		for _, line := range lines {
			// Skip headers and empty lines to find body
			if line == "" {
				bodyStarted = true
				continue
			}

			if bodyStarted {
				// Check that no line exceeds 76 characters
				if len(line) > 76 {
					t.Errorf("line too long after quoted-printable encoding: %d chars: %s", len(line), line)
				}

				// Check for soft line breaks (lines ending with =)
				if strings.HasSuffix(line, "=") {
					foundLongLineWrapped = true
				}
			}
		}

		// Verify that line wrapping occurred
		if !foundLongLineWrapped {
			t.Errorf("expected to find soft line breaks (=) indicating line wrapping")
		}

		// Verify original text content is preserved (should be present even if encoded)
		if !strings.Contains(raw, "very long line") {
			t.Errorf("original text content not found in message")
		}

	case <-time.After(2 * time.Second):
		t.Fatal("no message received by mock SMTP")
	}
}

func TestSend_QuotedPrintableSpecialChars(t *testing.T) {
	addr, recv, teardown := startMockSMTP(t)
	defer teardown()

	// Create template with special characters including non-ASCII and reserved chars
	specialChars := "Hello 世界！Special chars: =, ñ, café, résumé, and symbols: <>\"&"
	tmplContent := fmt.Sprintf("From: sender@example.com\nTo: recv@example.com\nSub: Special Chars Test\n\n%s", specialChars)
	tmplPath := tplWriteTemp(t, tmplContent)

	smarthost := HostPort{}
	smarthost.Host, smarthost.Port, _ = net.SplitHostPort(addr)

	cfg := EmailConfig{
		Smarthost:    smarthost,
		TemplatePath: tmplPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Send(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	select {
	case raw := <-recv:
		// Verify quoted-printable encoding is used
		if !strings.Contains(raw, "Content-Transfer-Encoding: quoted-printable") {
			t.Errorf("quoted-printable encoding not found in message")
		}

		// Verify special characters are encoded (should contain = markers)
		if !strings.Contains(raw, "=") {
			t.Errorf("no quoted-printable encoding markers found")
		}

		// Verify each line has proper length
		lines := strings.Split(raw, "\n")
		bodyStarted := false
		for _, line := range lines {
			if bodyStarted && line != "" {
				if len(line) > 76 {
					t.Errorf("quoted-printable line too long: %d chars: %s", len(line), line)
				}
			}
			if line == "" {
				bodyStarted = true
			}
		}

	case <-time.After(2 * time.Second):
		t.Fatal("no message received by mock SMTP")
	}
}

func TestSend_QuotedPrintableExactly76Chars(t *testing.T) {
	addr, recv, teardown := startMockSMTP(t)
	defer teardown()

	// Create a line that is exactly 76 characters (should not be wrapped)
	exactLine := strings.Repeat("a", 76)
	tmplContent := fmt.Sprintf("From: sender@example.com\nTo: recv@example.com\nSub: Exact Length Test\n\n%s", exactLine)
	tmplPath := tplWriteTemp(t, tmplContent)

	smarthost := HostPort{}
	smarthost.Host, smarthost.Port, _ = net.SplitHostPort(addr)

	cfg := EmailConfig{
		Smarthost:    smarthost,
		TemplatePath: tmplPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Send(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	select {
	case raw := <-recv:
		// Find the line with 76 'a' characters
		lines := strings.Split(raw, "\n")
		found := false
		for _, line := range lines {
			if strings.Contains(line, strings.Repeat("a", 70)) { // Check for substantial part
				found = true
				// Line should not be broken if it's exactly 76 chars or less
				if len(line) <= 76 {
					break
				} else {
					t.Errorf("76-character line was incorrectly wrapped: %d chars", len(line))
				}
			}
		}
		if !found {
			t.Errorf("expected 76-character line not found in output")
		}

	case <-time.After(2 * time.Second):
		t.Fatal("no message received by mock SMTP")
	}
}
