# Pigeon – Simple, Template-Based Email Library for Go

**Pigeon** is a lightweight Go library for sending emails with text templates, YAML/JSON configuration, and file attachments.
It uses only the Go standard library and is designed for flexible, production-quality email notifications and reports.

---

## Features

- Pure Go (no external dependencies, except YAML parser)
- Dynamic email headers and body with [text/template](https://pkg.go.dev/text/template)
- Load configuration from YAML/JSON files
- Support for multiple To/Cc/Bcc addresses
- UTF-8 subject lines (RFC 2047 encoding)
- Multipart/mixed email with file attachments
- Optional custom headers
- Comprehensive tests and example included

---

## Installation

```sh
go get github.com/dotarpa/pigeon
```

---

## Usage Example

### 1. Prepare a Template File

Example: `mail.tmpl`

```
From: {{ .From }}
To: {{ .To }}
Sub: {{ .Subject }}

Hello, {{ .Name }}!

{{- if .Items }}
You have {{ len .Items }} new items:
{{- range .Items }}
  - {{ . }}
{{- end }}
{{ else }}
No new items.
{{ end }}

-- End of message --
```

**Important Notes:**
- The template file must follow RFC2822 format: headers, then a blank line, then the message body
- The blank line between headers and body is **required**
- Both headers and body support Go template syntax (`{{ .Variable }}`)

### Header Priority

When both template file and configuration file specify the same header field, the priority order is:

1. **Template file headers** (highest priority)
2. **Configuration file values** (fallback)

Examples:

**Case 1: Template overrides config**
```
# mail.tmpl
From: template@example.com
To: user@example.com
Sub: Hello

Message body
```

```yaml
# config.yaml
from: config@example.com  # This will be ignored
to: config-to@example.com # This will be ignored
```
Result: Uses `template@example.com` and `user@example.com`

**Case 2: Config provides fallback**
```
# mail.tmpl
Sub: Hello

Message body
```

```yaml
# config.yaml
from: config@example.com
to: config-to@example.com
```
Result: Uses `config@example.com` and `config-to@example.com` from config

**Case 3: Template variables**
```
# mail.tmpl
From: {{ .From }}
To: {{ .To }}
Sub: {{ .Subject }}

Hello {{ .Name }}!
```
Result: Template variables are expanded using the data passed to `pigeon.Send()`

---

### 2. Prepare a YAML Configuration File

Example: `config.yaml`

```yaml
from: sender@example.com
to: receiver@example.com
smarthost: smtp.example.com:25
template_path: mail.tmpl
attachments:
  - ./sample.txt
headers:
  X-App: pigeon
timezone: Asia/Tokyo
```

---

### 3. Write Go Code to Send the Email

Example: `main.go`

```go
package main

import (
	"context"
	"log"

	"github.com/dotarpa/pigeon"
)

func main() {
	cfg, err := pigeon.LoadFile("config.yaml")
	if err != nil {
		log.Fatal(err)
	}
	data := map[string]any{
		"Name":    "Alice",
		"To":      cfg.To,
		"From":    cfg.From,
		"Subject": "Welcome!",
		"Items":   []string{"Server rebooted", "Disk space low"},
	}
	retry, err := pigeon.Send(context.Background(), *cfg, data)
	if err != nil {
		log.Fatalf("Send failed: %v (retry=%v)", err, retry)
	}
	log.Println("Mail sent successfully")
}
```

### 4. SendRaw

```go
package main

import (
	"context"
	"log"
	"strings"

	"github.com/dotarpa/pigeon"
)

func main() {
	rawMail := `From: alice@example.com
To: bob@example.com
Subject: =?UTF-8?B?44GT44KT44Gr44Gh44Gv?=

Hello Bob,
This is a test mail with UTF-8 subject!
`
	err := pigeon.SendRaw(context.Background(), strings.NewReader(rawMail), "localhost:25")
	if err != nil {
		log.Fatalf("SendRaw failed: %v", err)
	}
	log.Println("Mail sent")
}
```

---

## Testing

Run all unit and integration tests:

```sh
go test -v ./...
```

---

## Directory Structure

```
pigeon/
  config.go       # EmailConfig and configuration loading
  email.go        # Send function and MIME/multipart logic
  tpl/            # Email template parsing
  example/        # Usage example (main.go, config.yaml, mail.tmpl)
  testdata/       # (optional) test fixtures
```

---

## License

Apache License 2.0

---

## Links

- [pkg.go.dev documentation](https://pkg.go.dev/github.com/dotarpa/pigeon)
- [text/template documentation](https://pkg.go.dev/text/template)

---

## Limitations / Not Yet Implemented

Pigeon currently does **not** support:

- **HTML email**: Only plain text (`text/plain`) messages are supported. Embedding HTML in the template will not create a proper HTML email or `multipart/alternative` message.
- **SMTP authentication**: No support for SMTP username/password authentication. Only open or IP-authorized relays can be used.
- **TLS connections**: No `STARTTLS` or implicit SSL support; SMTP is unencrypted only.
- **Post-template validation**: There is no strict validation of headers, recipients, or content after template execution. Malformed output may cause the send to fail at the SMTP server.
