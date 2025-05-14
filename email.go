package pigeon

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/dotarpa/pigeon/internal/tpl"
)

// Send renders the template located at cfg.TemplatePath using data and
// sends the resulting message through the configured SMTP smarthost.
//
// The retrun value `retry` is true when the caller *may* retry the operation
// (e.g. transiet network failure) and false for parmanent errors
// (e.g. template parsing failure, 5xx SMTP response for invalid sender).
func Send(ctx context.Context, cfg EmailConfig, data any) (retry bool, err error) {
	if cfg.TemplatePath == "" {
		return false, errors.New("TemplatePath must be specified")
	}

	t, err := tpl.ParseFile(cfg.TemplatePath)
	if err != nil {
		return false, err
	}

	// ------------------------------------------------------------------
	// Build headers(template values have priority, config values are fallback)
	// ------------------------------------------------------------------
	hdr := make(textproto.MIMEHeader)

	from := chooseNonEmpty(t.From(), cfg.From)
	if from == "" {
		return false, errors.New("missing From address")
	}
	hdr.Set("From", from)

	to := chooseNonEmpty(t.To(), cfg.To)
	if to == "" {
		return false, errors.New("missing To address")
	}
	hdr.Set("To", to)

	if cc := chooseNonEmpty(t.Cc(), cfg.Cc); cc != "" {
		hdr.Set("Cc", cc)
	}
	if bcc := chooseNonEmpty(t.Bcc(), cfg.Bcc); bcc != "" {
		hdr.Set("Bcc", bcc)
	}

	// Subject is always taken from template(because config has no subject field for now).
	subj := t.Subject()
	if subj == "" {
		subj = "(no subject)"
	}
	hdr.Set("Subject", encodingUTF8Subject(subj))

	// Required headers.
	hdr.Set("MIME-Version", "1.0")
	hdr.Set("Content-Type", "text/plain; charset=UTF-8")
	hdr.Set("Content-Transfer-Encoding", "7bit")
	hdr.Set("Data", time.Now().UTC().Format(time.RFC1123Z))

	// Additional user-supplied headers from config
	for k, v := range cfg.Headers {
		if v == "" {
			continue
		}
		hdr.Set(k, v)
	}

	// ------------------------------------------------------------------
	// Render body
	// ------------------------------------------------------------------
	var msg bytes.Buffer
	for k, vv := range hdr {
		for _, v := range vv {
			foldHeader(&msg, k, v)
		}
	}
	msg.WriteString("\r\n") // blank line between headers and body

	if err := t.Execute(&msg, data); err != nil {
		return false, err // permanent error(template exec
	}

	// ------------------------------------------------------------------
	// Deliver via SMTP.
	// ------------------------------------------------------------------
	hostPort := cfg.Smarthost
	if hostPort == "" {
		hostPort = "localhost:25"
	}
	d := &net.Dialer{}
	if deadline, ok := ctx.Deadline(); ok {
		d.Deadline = deadline
	}
	conn, err := d.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return true, err // network failure - retry allowed
	}
	defer conn.Close()

	host := hostPort
	if idx := strings.LastIndex(hostPort, ":"); idx != -1 {
		host = hostPort[:idx]
	}

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return true, err
	}
	defer c.Quit()

	if cfg.Hello != "" {
		_ = c.Hello(cfg.Hello)
	}

	if err := c.Mail(from); err != nil {
		return false, err
	}

	for _, rcpt := range getherRecipients(hdr) {
		if err := c.Rcpt(rcpt); err != nil {
			return false, err // recipient rejected - permanent
		}
	}

	wc, err := c.Data()
	if err != nil {
		return true, err
	}
	if _, err := msg.WriteTo(wc); err != nil {
		return true, err
	}
	if err := wc.Close(); err != nil {
		return true, err
	}
	if err := c.Quit(); err != nil {
		return true, err
	}
	return false, nil

}

func chooseNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// encodingUTF8Subject returns an RFC 2047 encoded UTF-8 subject.
// Currently it uses the simple Base64 B encoding.
func encodingUTF8Subject(s string) string {
	// Encode only if non-ASCII is found; otherwise return as-is.
	if isASCII(s) {
		return s
	}
	b := base64.StdEncoding.EncodeToString([]byte(s))
	return fmt.Sprintf("=?UTF-8?B?%s?=", b)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

// foldHeader writes a single header line obeying the 78-char limitation.
// It uses a native space fold suitable for ASCII headers.
func foldHeader(buf *bytes.Buffer, k, v string) {
	const max = 78
	line := k + ": " + v
	if len(line) <= max {
		buf.WriteString(line + "\r\n")
		return
	}

	// naive folding: break at max-3 and indent one space
	for len(line) > max {
		buf.WriteString(line[:max-1] + "\r\n")
		line = line[max-1:]
	}
	buf.WriteString(line + "\r\n")
}

func getherRecipients(h textproto.MIMEHeader) []string {
	var list []string
	appendAddr := func(field string) {
		if v := h.Get(field); v != "" {
			for _, addr := range strings.Split(v, ",") {
				addr = strings.TrimSpace(addr)
				if addr != "" {
					list = append(list, addr)
				}
			}
		}
	}
	appendAddr("To")
	appendAddr("Cc")
	appendAddr("Bcc")
	return list
}
