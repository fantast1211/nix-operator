package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/registry"
	"go.xbrother.com/nix-operator/pkg/utils"
	"go.xbrother.com/nix-operator/pkg/validator"
	"go.xbrother.com/nix-operator/pkg/webcontroller"
	"go.xbrother.com/nix-operator/pkg/webrepository"
	"go.xbrother.com/nix-operator/pkg/webservice"
)

func main() {
	// 初始化日志
	logger, err := utils.NewLogger("webserver")
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	logger.Info("main", "Starting nix-operator web server...")

	// 配置目录
	configDir := "/etc/cr.d"
	if dir := os.Getenv("CONFIG_DIR"); dir != "" {
		configDir = dir
	}

	// 获取操作系统信息
	osInfo, err := getOSInfo()
	if err != nil {
		log.Fatalf("Failed to get OS info: %v", err)
	}

	// 创建组件注册表
	componentRegistry := registry.NewComponentRegistry(osInfo, logger)

	// 注册处理器和校验器
	if err := registerComponents(componentRegistry); err != nil {
		log.Fatalf("Failed to register components: %v", err)
	}

	// 验证注册完整性
	if err := componentRegistry.ValidateRegistration(); err != nil {
		log.Fatalf("Component registration validation failed: %v", err)
	}

	// 创建三层架构组件
	_, _, controllerLayer, err := createLayers(
		configDir,
		componentRegistry,
		logger,
	)
	if err != nil {
		log.Fatalf("Failed to create layers: %v", err)
	}

	// 创建HTTP服务器
	server := createHTTPServer(controllerLayer, logger)

	// 启动服务器
	go func() {
		logger.Info("main", "Starting HTTP server on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// 等待中断信号
	waitForShutdown(server, logger)

	// 可选：启动文件监控（如果需要）
	// go startFileWatcher(configDir, repositoryLayer, logger)

	logger.Info("main", "Server shutdown complete")
}

// getOSInfo 获取操作系统信息
func getOSInfo() (domain.OSInfo, error) {
	// 这里复用现有的controller包中的逻辑
	// 为了简化，直接返回Linux信息
	return domain.OSInfo{
		ID:         "ubuntu",
		VersionID:  "20.04",
		KernelName: "Linux",
		KernelVer:  "5.4.0",
	}, nil
}

// registerComponents 注册所有组件
func registerComponents(componentRegistry *registry.ComponentRegistry) error {
	// 注册校验器
	validator.RegisterDefaultValidators(componentRegistry.GetValidatorRegistry())

	// 注册处理器（这里需要根据实际的处理器实现进行调整）
	// 注意：这里假设hosts和time包中的处理器已经实现了controller.Handler接口
	
	// 由于现有的处理器是通过init()函数注册的，我们需要获取它们
	// 这里提供一个示例框架，实际实现需要根据具体的处理器来调整
	
	return nil
}

// createLayers 创建三层架构
func createLayers(
	configDir string,
	componentRegistry *registry.ComponentRegistry,
	logger *utils.Logger,
) (webrepository.Repository, webservice.Service, webcontroller.Controller, error) {
	// 创建Repository层
	reconcileRepo := webrepository.NewReconcileRepository(
		configDir,
		componentRegistry.GetHandlers(),
		componentRegistry.GetOSInfo(),
		logger,
	)

	configRepo := webrepository.NewConfigRepository(
		configDir,
		reconcileRepo,
		logger,
	)

	// 创建Service层
	resourceService := webservice.NewResourceService(
		configRepo,
		reconcileRepo,
		componentRegistry.GetValidatorRegistry(),
		logger,
	)

	// 创建Controller层
	resourceController := webcontroller.NewResourceController(
		resourceService,
		componentRegistry.GetValidatorRegistry(),
		logger,
	)

	return nil, resourceService, resourceController, nil
}

// createHTTPServer 创建HTTP服务器
func createHTTPServer(controller webcontroller.Controller, logger *utils.Logger) *http.Server {
	httpHandler := webcontroller.NewHTTPHandler(controller, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/resources", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			httpHandler.ListResourcesHandler(w, r)
		case http.MethodPost:
			httpHandler.UpdateResourceHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/v1/resource", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			httpHandler.GetResourceHandler(w, r)
		case http.MethodPut:
			httpHandler.UpdateResourceHandler(w, r)
		case http.MethodDelete:
			httpHandler.DeleteResourceHandler(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// 健康检查端点
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
}

// waitForShutdown 等待关闭信号
func waitForShutdown(server *http.Server, logger *utils.Logger) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	logger.Info("main", "Received shutdown signal")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Errorf("main", "Server shutdown error: %v", err)
	}
}

// startFileWatcher 启动文件监控（可选）
func startFileWatcher(configDir string, repo webrepository.Repository, logger *utils.Logger) {
	// 这里可以实现文件监控逻辑，当配置文件变化时触发调谐
	// 可以复用现有controller包中的文件监控逻辑
	logger.Info("main", "File watcher would be started here")
}