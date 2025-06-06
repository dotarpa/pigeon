// Package pigeon provides an email sending library with support for
// RFC2822 templates, YAML/JSON configuration, and attachments.
// See README.md for details and usage examples.
package pigeon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/dotarpa/pigeon/tpl"
)

const (
	maxLineLength    = 78
	maxContentLength = 76
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

	if cfg.Smarthost.Host == "" && cfg.Smarthost.Port == "" {
		return false, errors.New("smarthost must be specified")
	}

	t, err := tpl.ParseFile(cfg.TemplatePath)
	if err != nil {
		return false, err
	}

	// Build the message headers.
	hdr := make(textproto.MIMEHeader)

	// Render template fields with data
	var fromBuf, toBuf, ccBuf, bccBuf, subjBuf bytes.Buffer

	fromTemplate := chooseNonEmpty(t.From(), cfg.From)
	if fromTemplate == "" {
		return false, errors.New("missing From address")
	}

	// Parse and execute From field as template
	fromTpl, err := template.New("from").Parse(fromTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to parse From template: %w", err)
	}
	if err := fromTpl.Execute(&fromBuf, data); err != nil {
		return false, fmt.Errorf("failed to execute From template: %w", err)
	}
	from := fromBuf.String()

	hdr.Set("From", from)

	toTemplate := chooseNonEmpty(t.To(), cfg.To)
	if toTemplate == "" {
		return false, errors.New("missing To address")
	}

	// Parse and execute To field as template
	toTpl, err := template.New("to").Parse(toTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to parse To template: %w", err)
	}
	if err := toTpl.Execute(&toBuf, data); err != nil {
		return false, fmt.Errorf("failed to execute To template: %w", err)
	}
	to := toBuf.String()
	hdr.Set("To", to)

	// Handle Cc if present
	if ccTemplate := chooseNonEmpty(t.Cc(), cfg.Cc); ccTemplate != "" {
		ccTpl, err := template.New("cc").Parse(ccTemplate)
		if err != nil {
			return false, fmt.Errorf("failed to parse Cc template: %w", err)
		}
		if err := ccTpl.Execute(&ccBuf, data); err != nil {
			return false, fmt.Errorf("failed to execute Cc template: %w", err)
		}
		if cc := ccBuf.String(); cc != "" {
			hdr.Set("Cc", cc)
		}
	}

	// Handle Bcc if present
	if bccTemplate := chooseNonEmpty(t.Bcc(), cfg.Bcc); bccTemplate != "" {
		bccTpl, err := template.New("bcc").Parse(bccTemplate)
		if err != nil {
			return false, fmt.Errorf("failed to parse Bcc template: %w", err)
		}
		if err := bccTpl.Execute(&bccBuf, data); err != nil {
			return false, fmt.Errorf("failed to execute Bcc template: %w", err)
		}
		if bcc := bccBuf.String(); bcc != "" {
			hdr.Set("Bcc", bcc)
		}
	}

	// Subject is always taken from template(because config has no subject field for now).
	subjTemplate := t.Subject()
	if subjTemplate == "" {
		subjTemplate = "(no subject)"
	}

	// Parse and execute Subject field as template
	subjTpl, err := template.New("subject").Parse(subjTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to parse Subject template: %w", err)
	}
	if err := subjTpl.Execute(&subjBuf, data); err != nil {
		return false, fmt.Errorf("failed to execute Subject template: %w", err)
	}
	subj := subjBuf.String()
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
		var bodyBuf bytes.Buffer
		t.Execute(&bodyBuf, data)

		if isASCII(bodyBuf.String()) && !hasLongLines(bodyBuf.String()) {
			hdr.Set("Content-Type", "text/plain; charset=UTF-8")
			hdr.Set("Content-Transfer-Encoding", "7bit")
		} else {
			hdr.Set("Content-Type", "text/plain; charset=UTF-8")
			hdr.Set("Content-Transfer-Encoding", "quoted-printable")
		}

		writeHeaders(&msg, hdr)
		msg.WriteString("\r\n")
		writeTextPart(&msg, t, data)
	} else {
		// Otherwise, construct a multipart/mixed message.
		mw := multipart.NewWriter(&msg)
		// Set a shorter boundary to avoid line wrapping issues
		boundary := fmt.Sprintf("pigeon_%d", time.Now().Unix())
		mw.SetBoundary(boundary)
		hdr.Set("Content-Type", fmt.Sprintf("multipart/mixed; boundary=%s", boundary))
		writeHeaders(&msg, hdr)
		msg.WriteString("\r\n")

		// part 1: text body
		var bodyBuf bytes.Buffer
		t.Execute(&bodyBuf, data)

		textHdr := textproto.MIMEHeader{}
		if isASCII(bodyBuf.String()) && !hasLongLines(bodyBuf.String()) {
			textHdr.Set("Content-Type", "text/plain; charset=UTF-8")
			textHdr.Set("Content-Transfer-Encoding", "7bit")
		} else {
			textHdr.Set("Content-Type", "text/plain; charset=UTF-8")
			textHdr.Set("Content-Transfer-Encoding", "quoted-printable")
		}

		pw, _ := mw.CreatePart(textHdr)
		writeTextPart(pw, t, data)

		// Part 2+: attachments.
		for _, path := range cfg.Attachments {
			if err := addAttachmentPart(mw, path); err != nil {
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
	defer func() {
		if quitErr := c.Quit(); quitErr != nil {
			// Log but don't override the main error
		}
	}()

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
	return false, nil
}

// addAttachmentPart adds a file as a base64-encoded attachment part to the multipart message.
// It infers the content type from the file extension.
func addAttachmentPart(mw *multipart.Writer, path string) error {
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

// encodingUTF8Subject returns an RFC 2047 encoded UTF-8 subject using quoted-printable
func encodingUTF8Subject(s string) string {
	if isASCII(s) {
		return s
	}

	var buf bytes.Buffer
	qpWriter := quotedprintable.NewWriter(&buf)
	qpWriter.Write([]byte(s))
	qpWriter.Close()

	// Remove any CRLF that quoted-printable might add and replace = with encoded form
	encoded := strings.ReplaceAll(buf.String(), "\r\n", "")
	encoded = strings.ReplaceAll(encoded, "=", "=3D")
	return fmt.Sprintf("=?UTF-8?Q?%s?=", encoded)
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

// writeHeaders writes the MIME headers to the buffer with simple line folding.
func writeHeaders(buf *bytes.Buffer, h textproto.MIMEHeader) {
	for k, vv := range h {
		for _, v := range vv {
			line := k + ": " + v
			if len(line) <= maxLineLength {
				buf.WriteString(line + "\r\n")
			} else {
				// Simple folding at reasonable break points
				buf.WriteString(k + ": ")
				remaining := v
				lineLen := len(k) + 2

				for len(remaining) > 0 {
					available := maxLineLength - lineLen
					if available <= 0 {
						buf.WriteString("\r\n ")
						lineLen = 1
						available = maxLineLength - 1
					}

					if len(remaining) <= available {
						buf.WriteString(remaining + "\r\n")
						break
					}

					// Find a good break point
					breakPoint := available
					for i := available - 1; i > available/2 && i < len(remaining); i-- {
						if remaining[i] == ' ' || remaining[i] == ';' {
							breakPoint = i + 1
							break
						}
					}

					buf.WriteString(remaining[:breakPoint] + "\r\n ")
					remaining = remaining[breakPoint:]
					lineLen = 1
				}
			}
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

// SendRaw sends the raw RFC2822 message (headers+body) via SMTP to smtpAddr.
// From, To, Cc, Bcc headers are extracted and used for MAIL/RCPT commands.
// The message is streamed as-is via DATA.
func SendRaw(ctx context.Context, raw io.Reader, smtpAddr string) error {
	tp := textproto.NewReader(bufio.NewReader(raw))
	headers, err := tp.ReadMIMEHeader()
	if err != nil {
		return fmt.Errorf("failed to parse header: %w", err)
	}

	from := headers.Get("From")
	if from == "" {
		return errors.New("missing from address")
	}

	toAll := parseAddressList(headers.Get("To"))
	toAll = append(toAll, parseAddressList(headers.Get("Cc"))...)
	toAll = append(toAll, parseAddressList(headers.Get("Bcc"))...)

	if len(toAll) == 0 {
		return errors.New("no recipients found in To/Cc/Bcc")
	}

	host := smtpAddr
	if i := strings.Index(smtpAddr, ":"); i > 0 {
		host = smtpAddr[:i]
	}
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", smtpAddr)
	if err != nil {
		return fmt.Errorf("failed to dial smtp: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp.NewClient: %w", err)
	}
	defer client.Quit()

	addrFrom, err := extractAddr(from)
	if err != nil {
		return fmt.Errorf("parse From: %w", err)
	}
	if err := client.Mail(addrFrom); err != nil {
		return fmt.Errorf("MAIL FROM failed: %w", err)
	}

	uniq := map[string]struct{}{}
	for _, rcpt := range toAll {
		addrRcpt, err := extractAddr(rcpt)
		if err != nil {
			continue
		}
		if _, ok := uniq[addrRcpt]; ok {
			continue
		}
		if err := client.Rcpt(addrRcpt); err != nil {
			return fmt.Errorf("RCPT TO failed for %s: %w", addrRcpt, err)
		}
		uniq[addrRcpt] = struct{}{}
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}

	if seeker, ok := raw.(io.Seeker); ok {
		// seek to start if possible
		seeker.Seek(0, io.SeekStart)
	}
	if _, err := io.Copy(wc, tp.R); err != nil {
		return fmt.Errorf("sending mail data failed: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("DATA close: %w", err)
	}
	client.Quit()
	return nil
}

func parseAddressList(list string) []string {
	if list == "" {
		return nil
	}

	addrList, err := mail.ParseAddressList(list)
	if err != nil {
		// fallback: try to split by comma if parse fails
		spl := regexp.MustCompile(`\s*,\s*`).Split(list, -1)
		var out []string
		for _, s := range spl {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	out := make([]string, 0, len(addrList))
	for _, a := range addrList {
		out = append(out, a.Address)
	}

	return out
}

// extractAddr extracts only the email address part (no name/comment).
func extractAddr(addr string) (string, error) {
	a, err := mail.ParseAddress(addr)
	if err == nil {
		return a.Address, nil
	}
	// fallback: try <address> pattern or return as is if looks like an email
	re := regexp.MustCompile(`<([^>]+)>`)
	m := re.FindStringSubmatch(addr)
	if len(m) == 2 {
		return m[1], nil
	}

	addr = strings.TrimSpace(addr)
	if strings.Contains(addr, "@") {
		return addr, nil
	}

	return "", errors.New("invalid address format")
}

// writeTextPart writes the text body with quoted-printable encoding when needed
func writeTextPart(w io.Writer, t *tpl.Template, data any) error {
	var bodyBuf bytes.Buffer
	if err := t.Execute(&bodyBuf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	body := bodyBuf.String()

	// Always use quoted-printable for non-ASCII content or long lines
	if !isASCII(body) || hasLongLines(body) {
		qpWriter := quotedprintable.NewWriter(w)
		defer qpWriter.Close()
		if _, err := qpWriter.Write([]byte(body)); err != nil {
			return fmt.Errorf("failed to write quoted-printable: %w", err)
		}
		return nil
	}

	if _, err := w.Write([]byte(body)); err != nil {
		return fmt.Errorf("failed to write body: %w", err)
	}
	return nil
}

// hasLongLines checks if any line in the text exceeds 76 characters
func hasLongLines(text string) bool {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if len(line) > maxContentLength {
			return true
		}
	}
	return false
}
