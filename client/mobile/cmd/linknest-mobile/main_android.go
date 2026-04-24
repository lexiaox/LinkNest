//go:build android

package main

import (
	"log"

	"linknest/client/mobile/internal/ui"
)

func main() {
	if err := ui.Launch(); err != nil {
		log.Fatalf("launch mobile app: %v", err)
	}
}
