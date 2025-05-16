package tpl

import (
	"bufio"
	"io"
	"net/textproto"
	"os"
	"strings"
	"text/template"
)

type Template struct {
	hdr      textproto.MIMEHeader
	bodyTmpl *template.Template
	srcPath  string
}

func ParseFile(path string) (*Template, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tp := textproto.NewReader(bufio.NewReader(f))
	hdr := make(textproto.MIMEHeader)

	// 1) ヘッダの読み込み(空行に遭遇するまで)
	for {
		line, err := tp.ReadLine()
		if err != nil {
			if err == io.EOF {
				break // ファイルが減っだのみの場合
			}
		}
		if line == "" {
			break
		}
		// RFC2822: header-name ":" pace* header-value
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		if strings.EqualFold(k, "Sub") {
			k = "Subject"
		}
		hdr.Set(k, v)
	}

	// 2) 残りをbodyテンプレートとして読み込む
	bodyBytes, err := io.ReadAll(tp.R)
	if err != nil {
		return nil, err
	}

	// body部はGoのtext/templateとして解釈
	bodyTmpl, err := template.New(path).Parse(string(bodyBytes))
	if err != nil {
		return nil, err
	}

	return &Template{hdr: hdr, bodyTmpl: bodyTmpl, srcPath: path}, nil
}

func (t *Template) Header() textproto.MIMEHeader {
	return t.hdr
}

func (t *Template) Execute(w io.Writer, data any) error {
	return t.bodyTmpl.Execute(w, data)
}

func (t *Template) Subject() string {
	return t.hdr.Get("Subject")
}

func (t *Template) From() string { return t.hdr.Get("From") }
func (t *Template) To() string   { return t.hdr.Get("To") }
func (t *Template) Cc() string   { return t.hdr.Get("Cc") }
func (t *Template) Bcc() string  { return t.hdr.Get("Bcc") }
