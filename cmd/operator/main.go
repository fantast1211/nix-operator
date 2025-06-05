package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/service"

	// 注册所有处理器
	_ "go.xbrother.com/nix-operator/pkg/handlers/hosts"
	// _ "go.xbrother.com/nix-operator/pkg/handlers/network"
	// _ "go.xbrother.com/nix-operator/pkg/handlers/serial"
	// _ "go.xbrother.com/nix-operator/pkg/handlers/time"
	// _ "go.xbrother.com/nix-operator/pkg/handlers/udev"
)

func main() {
	configDir := flag.String("config-dir", "etc/cr.d", "Path to configuration directory")
	enableAPI := flag.Bool("enable-api", true, "Enable HTTP/gRPC API")
	flag.Parse()

	controller, err := controller.NewController(*configDir)
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}

	log.Printf("Starting nix-operator with config directory: %s", *configDir)

	// 创建上下文，用于优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动API服务器
	if *enableAPI {
		// 启动gRPC服务器
		grpcServer, _, err := service.StartGRPCServer(controller)
		if err != nil {
			log.Fatalf("Failed to start gRPC server: %v", err)
		}
		defer grpcServer.GracefulStop()

		// 启动HTTP服务器
		go func() {
			if err := service.StartHTTPServer(ctx); err != nil {
				log.Fatalf("Failed to start HTTP server: %v", err)
			}
		}()
	}

	// 启动控制器
	go func() {
		if err := controller.Run(); err != nil {
			log.Fatalf("Error running controller: %v", err)
		}
	}()

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")
}
