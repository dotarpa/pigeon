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
