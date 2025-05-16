// Package pigeon provides an email sending library with support for
// RFC2822 templates, YAML/JSON configuration, and attachments.
// See README.md for details and usage examples.
package pigeon

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotarpa/pigeon/tpl"
)

// Send builds and sends an email using the specified configuration and template data.
//
// If cfg.Attachments is non-empty, the message will be sent as multipart/mixed
// with attachments. Otherwise, a simple text/plain message is used.
// The template is loaded from cfg.TemplatePath and rendered using the provided data.
//
// The function returns (retry, err):
//   - retry=true means a temporary error (the caller may want to retry later)
//   - retry=false means a permanent error (invalid configuration, fatal SMTP error, etc.)
func Send(ctx context.Context, cfg EmailConfig, data any) (retry bool, err error) {
	if cfg.TemplatePath == "" {
		return false, errors.New("TemplatePath must be specified")
	}

	t, err := tpl.ParseFile(cfg.TemplatePath)
	if err != nil {
		return false, err
	}

	// Build the message headers.
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

	// Use the specified timezone if set; otherwise, default to UTC.
	var msgTime time.Time
	if cfg.Timezone != "" {
		loc, err := time.LoadLocation(cfg.Timezone)
		if err == nil {
			msgTime = time.Now().In(loc)
		} else {
			msgTime = time.Now().UTC()
		}
	} else {
		msgTime = time.Now().UTC()
	}
	hdr.Set("Date", msgTime.Format(time.RFC1123Z))

	// Add any custom headers from the configuration.
	for k, v := range cfg.Headers {
		if v == "" {
			continue
		}
		hdr.Set(k, v)
	}

	var msg bytes.Buffer

	// If there are no attachments, send as plain text.
	if len(cfg.Attachments) == 0 {
		hdr.Set("Context-Type", "text/plain; charset=UTF-8")
		hdr.Set("Content-Transfer-Encoding", "7bit")
		writeHeaders(&msg, hdr)
		msg.WriteString("\r\n")
		_ = t.Execute(&msg, data)
	} else {
		// Otherwise, construct a multipart/mixed message.
		mw := multipart.NewWriter(&msg)
		hdr.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", mw.Boundary()))
		writeHeaders(&msg, hdr)
		msg.WriteString("\r\n")

		// part 1: text body
		textHdr := textproto.MIMEHeader{
			"Content-Type":              {"text/plain; charset=UTF-8"},
			"Content-Transfer-Encoding": {"7bit"},
		}
		pw, _ := mw.CreatePart(textHdr)
		_ = t.Execute(pw, data)

		// Part 2+: attachments.
		for _, path := range cfg.Attachments {
			if err := addAttachementPart(mw, path); err != nil {
				return false, err
			}
		}
		mw.Close()
	}

	// Deliver the message via SMTP.
	hostPort := cfg.Smarthost.String()
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

	for _, rcpt := range recipients(hdr) {
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

// addAttachmentPart adds a file as a base64-encoded attachment part to the multipart message.
// It infers the content type from the file extension.
func addAttachementPart(mw *multipart.Writer, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fname := filepath.Base(path)
	ctype := mime.TypeByExtension(filepath.Ext(fname))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	hdr := textproto.MIMEHeader{
		"Content-Type":              {fmt.Sprintf("%s; name=\"%s\"", ctype, fname)},
		"Content-Transfer-Encoding": {"base64"},
		"Content-Disposition":       {fmt.Sprintf("attachment; filename=\"%s\"", fname)},
	}
	pw, _ := mw.CreatePart(hdr)
	encodeAndWrapBase64(pw, data)
	return nil
}

// encodeAndWrapBase64 writes base64-encoded data to w, breaking lines at 76 characters per RFC 2045.
func encodeAndWrapBase64(w io.Writer, b []byte) {
	enc := base64.StdEncoding
	const line = 76
	for len(b) > 0 {
		n := line / 4 * 3 // encode quantized
		if n > len(b) {
			n = len(b)
		}
		chunk := enc.EncodeToString(b[:n])
		io.WriteString(w, chunk)
		io.WriteString(w, "\r\n")
		b = b[n:]
	}
}

// chooseNonEmpty returns a if non-empty, else b.
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

// isASCII returns true if s contains only ASCII characters.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

// foldHeader writes a header with a soft line length limit of 78 characters (RFC 5322 recommended).
// This is not strictly required (limit is 998), but improves readability and interoperability.
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

// writeHeaders writes the MIME headers to the buffer, folding long lines at 78 characters.
func writeHeaders(buf *bytes.Buffer, h textproto.MIMEHeader) {
	for k, vv := range h {
		for _, v := range vv {
			foldHeader(buf, k, v)
		}
	}
}

// recipients extracts all recipient addresses (To, Cc, Bcc) from the headers.
func recipients(h textproto.MIMEHeader) []string {
	var out []string
	for _, f := range []string{"To", "Cc", "Bcc"} {
		for _, addr := range strings.Split(h.Get(f), ",") {
			if addr = strings.TrimSpace(addr); addr != "" {
				out = append(out, addr)
			}
		}
	}
	return out
}
