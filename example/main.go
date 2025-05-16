package main

import (
	"context"
	"log"

	"github.com/dotarpa/pigeon"
)

func main() {
	// 設定ファイル読み込み
	cfg, err := pigeon.LoadFile("example/config.yaml")
	if err != nil {
		log.Fatalf("LoadFile error: %v", err)
	}
	// テンプレートに渡す値
	data := map[string]any{
		"Name":    "Pigeon User",
		"To":      cfg.To,
		"Subject": "Hello from Pigeon",
	}
	retry, err := pigeon.Send(context.Background(), *cfg, data)
	if err != nil {
		log.Fatalf("Send failed: %v (retry=%v)", err, retry)
	}
	log.Println("Mail sent!")
}
