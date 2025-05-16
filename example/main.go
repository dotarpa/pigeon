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
