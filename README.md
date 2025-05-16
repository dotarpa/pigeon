# pigeon - Simple Go Email Library

**Pigeon** は Go 言語向けの、テンプレート・YAML・添付ファイル対応メール送信ライブラリです。  
標準パッケージのみで実装されており、テキストテンプレート・添付ファイル付きメール・設定の動的ロードに対応します。

## Usage

### メールテンプレートファイル例

**ヘッダと本文の間には必ず空行が必要です。**

- mail.tmpl(例)

```
From: sender@example.com
To: {{ .To }}
Sub: {{ .Subject }}

Hello, {{ .Name }}!
```

### configファイル例

- config.yaml

```yaml
from: sender@example.com
to: receiver@example.com
smarthost: smtp.example.com:25
template_path: mail.tmpl
attachments:
  - ./sample.txt
headers:
  X-App: pigeon
```

### example code

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
		"Subject": "Welcome!",
	}
	retry, err := pigeon.Send(context.Background(), *cfg, data)
	if err != nil {
		log.Fatalf("Send failed: %v (retry=%v)", err, retry)
	}
	log.Println("Mail sent successfully")
}
```
