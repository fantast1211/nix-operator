package main

import (
	"flag"
	"log"

	"go.xbrother.com/nix-operator/pkg/controller"

	// 注册所有处理器
	_ "go.xbrother.com/nix-operator/pkg/handlers/hosts"
	// _ "go.xbrother.com/nix-operator/pkg/handlers/network"
	// _ "go.xbrother.com/nix-operator/pkg/handlers/serial"
	// _ "go.xbrother.com/nix-operator/pkg/handlers/time"
	// _ "go.xbrother.com/nix-operator/pkg/handlers/udev"
)

func main() {
	configDir := flag.String("config-dir", "etc/cr.d", "Path to configuration directory")
	flag.Parse()

	controller, err := controller.NewController(*configDir)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	log.Printf("Starting nix-operator with config directory: %s", *configDir)
	if err := controller.Run(); err != nil {
		log.Fatalf("Error running controller: %v", err)
	}
}
