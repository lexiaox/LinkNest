//go:build windows

package main

import (
	"log"

	"linknest/client/desktop/internal/ui"
	clientconfig "linknest/client/internal/config"
)

func main() {
	root, err := clientconfig.RootDir()
	if err != nil {
		log.Fatalf("resolve client root: %v", err)
	}

	if err := ui.Launch(root); err != nil {
		log.Fatalf("launch desktop app: %v", err)
	}
}
