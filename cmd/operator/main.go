package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/utils"

	// 注册所有处理器
	_ "go.xbrother.com/nix-operator/pkg/handlers/hosts"
	_ "go.xbrother.com/nix-operator/pkg/handlers/time"
)

// initializeLogger 初始化日志系统并设置优雅关闭
func initializeLogger() (*utils.Logger, error) {
	// 初始化日志系统
	if err := utils.InitLogger("xtopus"); err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %v", err)
	}

	// 设置信号处理，确保优雅关闭
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		utils.Info("main", "Received shutdown signal, closing logger...")
		utils.CloseLogger()
		os.Exit(0)
	}()

	return utils.GlobalLogger, nil
}

func main() {
	configDir := flag.String("config-dir", "etc/cr.d", "Path to configuration directory")
	flag.Parse()

	// 初始化日志系统
	logger, err := initializeLogger()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer utils.CloseLogger()

	logger.Info("main", "Initializing xtopus operator...")

	controller, err := controller.NewController(*configDir, logger)
	if err != nil {
		logger.Fatal("main", "Failed to create controller: "+err.Error())
	}

	logger.Infof("main", "Starting xtopus operator with config directory: %s", *configDir)
	if err := controller.Run(); err != nil {
		logger.Fatal("main", "Error running controller: "+err.Error())
	}
}
