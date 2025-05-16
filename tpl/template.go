// Package tpl provides email template parsing and execution utilities
// for the pigeon email library. It supports RFC2822-style headers and
// Go's text/template syntax for message bodies.
package tpl

import (
	"bufio"
	"io"
	"net/textproto"
	"os"
	"strings"
	"text/template"
)

// Template represents a parsed email template, including headers
// and a text/template for the message body. The template file should
// use RFC2822-style headers followed by a blank line and a body,
// both supporting Go template variables.
type Template struct {
	hdr      textproto.MIMEHeader
	bodyTmpl *template.Template
	srcPath  string
}

// ParseFile parses an email template file in RFC2822-style format.
// The file must contain headers (key: value), a blank line, and then
// a body. Both headers and body may use Go template expressions.
// Returns a Template that can be executed with data to produce a
// complete message.
func ParseFile(path string) (*Template, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp := textproto.NewReader(bufio.NewReader(f))
	hdr := make(textproto.MIMEHeader)

	// 1) Read headers (until a blank line)
	for {
		line, err := tp.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
		}
		if line == "" {
			break
		}
		// RFC2822: header-name ":" space* header-value
		// Header line: key: value
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		// Allow "Sub:" as an alias for "Subject:"
		if strings.EqualFold(k, "Sub") {
			k = "Subject"
		}
		hdr.Set(k, v)
	}

	// 2) Read the remainder as the body template
	bodyBytes, err := io.ReadAll(tp.R)
	if err != nil {
		return nil, err
	}

	// Parse the body as a Go text/template
	bodyTmpl, err := template.New(path).Parse(string(bodyBytes))
	if err != nil {
		return nil, err
	}

	return &Template{hdr: hdr, bodyTmpl: bodyTmpl, srcPath: path}, nil
}

// Header returns the template's parsed MIME headers.
func (t *Template) Header() textproto.MIMEHeader {
	return t.hdr
}

// Execute renders the message body using the provided data,
// using Go text/template syntax.
func (t *Template) Execute(w io.Writer, data any) error {
	return t.bodyTmpl.Execute(w, data)
}

// Subject returns the "Subject" field from the template headers.
func (t *Template) Subject() string {
	return t.hdr.Get("Subject")
}

// From returns the "From" field from the template headers.
func (t *Template) From() string { return t.hdr.Get("From") }

// To returns the "To" field from the template headers.
func (t *Template) To() string { return t.hdr.Get("To") }

// Cc returns the "Cc" field from the template headers.
func (t *Template) Cc() string { return t.hdr.Get("Cc") }

// Bcc returns the "Bcc" field from the template headers.
func (t *Template) Bcc() string { return t.hdr.Get("Bcc") }
