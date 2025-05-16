package main

import (
	"context"
	"log"

	"github.com/dotarpa/pigeon"
)

func main() {
	cfg, err := pigeon.LoadFile("example/config.yaml")
	if err != nil {
		log.Fatalf("LoadFile error: %v", err)
	}
	data := map[string]any{
		"From":    cfg.From,
		"To":      cfg.To,
		"Subject": "Daily Report",
		"Name":    "Pigeon User",
		"Items":   []string{"Server rebooted", "Disk space low", "New user registered"},
	}
	retry, err := pigeon.Send(context.Background(), *cfg, data)
	if err != nil {
		log.Fatalf("Send failed: %v (retry=%v)", err, retry)
	}
	log.Println("Mail sent!")
}
