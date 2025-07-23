package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/repository"
	"go.xbrother.com/nix-operator/pkg/schema"
	"go.xbrother.com/nix-operator/pkg/service"
	"go.xbrother.com/nix-operator/pkg/utils"
	"go.xbrother.com/nix-operator/pkg/validator"
	"go.xbrother.com/nix-operator/pkg/webcontroller"

	// 注册所有处理器
	_ "go.xbrother.com/nix-operator/pkg/handlers/hosts"
	_ "go.xbrother.com/nix-operator/pkg/handlers/time"

	_ "go.xbrother.com/nix-operator/pkg/handlers/bond"
	_ "go.xbrother.com/nix-operator/pkg/handlers/network"
)

const (
	grpcPort = 19456
	httpPort = 18456
)

// 启动gRPC服务器
func StartGRPCServer(ctx context.Context, validators map[string]validator.SpecValidator, logger *utils.Logger, resourceService service.ResourceService, schemaProvider schema.Provider, cancel context.CancelFunc) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	// 注册ConfigService
	systemv1.RegisterSystemConfigServiceServer(grpcServer, webcontroller.NewSystemServiceServer(validators, logger, resourceService, schemaProvider))

	go func() {
		logger.Infof("main", "Starting gRPC server on port %d...", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Errorf("main", "Failed to serve gRPC: %v", err)
			cancel() // 触发上下文取消，通知主程序优雅关闭
		}
	}()

	return grpcServer, lis, nil
}

// 启动HTTP服务器（gRPC-Gateway）
func StartHTTPServer(ctx context.Context, logger *utils.Logger, cancel context.CancelFunc) {
	// 配置支持protobuf Any类型的marshaler
	mux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
			Marshaler: &runtime.JSONPb{
				MarshalOptions: protojson.MarshalOptions{
					EmitUnpopulated: true,
				},
				UnmarshalOptions: protojson.UnmarshalOptions{
					DiscardUnknown: true,
				},
			},
		}),
	)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// 注册SystemService的HTTP处理程序
	if err := systemv1.RegisterSystemConfigServiceHandlerFromEndpoint(ctx, mux, fmt.Sprintf("localhost:%d", grpcPort), opts); err != nil {
		logger.Errorf("main", "Failed to register system service gateway: %v", err)
		cancel() // 触发上下文取消，通知主程序优雅关闭
		return
	}

	logger.Infof("main", "Starting HTTP server on port %d...", httpPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", httpPort), mux); err != nil {
		logger.Errorf("main", "HTTP server failed: %v", err)
		cancel() // 触发上下文取消，通知主程序优雅关闭
	}
}

// initializeLogger 初始化日志系统
func initializeLogger() (*utils.Logger, error) {
	// 初始化日志系统
	if err := utils.InitLogger("xtopus"); err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %v", err)
	}

	return utils.GlobalLogger, nil
}

// Application 应用程序结构
type Application struct {
	ResourceController webcontroller.SystemConfigServiceServer
	ResourceService    service.ResourceService
	ConfigRepository   repository.ConfigRepository
}

// initializeApplication 初始化三层架构应用
func initializeApplication(configDir string, logger *utils.Logger, ctx context.Context, statusRepo repository.StatusRepository, cancel context.CancelFunc) (*grpc.Server, error) {

	// 初始化校验器映射
	validators := initializeValidators()

	// 初始化 Repository 层
	configRepo := repository.NewConfigRepository(configDir, logger)

	// 初始化 Service 层
	resourceService := service.NewResourceService(configRepo, statusRepo, logger)

	// 初始化 Schema Provider
	schemaProvider := schema.NewProvider()

	// 启动gRPC服务器
	grpcServer, _, err := StartGRPCServer(ctx, validators, logger, resourceService, schemaProvider, cancel)
	if err != nil {
		return nil, fmt.Errorf("failed to start gRPC server: %v", err)
	}

	// 启动HTTP服务器
	go StartHTTPServer(ctx, logger, cancel)

	return grpcServer, nil
}

// initializeValidators 初始化校验器映射
// 使用自动扫描注册系统，支持自动发现和注册所有 *ConfigurationSpec 类型
func initializeValidators() map[string]validator.SpecValidator {
	// 创建自动校验器注册表
	registry := validator.NewAutoValidatorRegistry()

	// 这里可以注册特殊的校验器覆盖（如果某些类型需要特殊校验逻辑）
	// 例如：registry.RegisterOverride("SpecialConfiguration", customValidator)

	// 自动扫描并注册所有配置类型的校验器
	if err := registry.AutoScan(); err != nil {
		// 如果自动扫描失败，记录错误但不中断程序
		// 可以考虑降级到手动注册或者返回错误
		panic(fmt.Sprintf("Failed to auto-scan validators: %v", err))
	}

	// 返回注册的校验器映射
	return registry.GetValidators()
}

func main() {
	configDir := flag.String("config-dir", "etc/cr.d", "Path to configuration directory")
	statusDir := flag.String("status-dir", "etc/status", "Path to status directory")
	nodeDir := flag.String("node-dir", "etc/nodes", "Path to node configuration directory")
	enableAPI := flag.Bool("enable-api", true, "Enable HTTP/gRPC API")
	flag.Parse()

	// 初始化日志系统
	logger, err := initializeLogger()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer utils.CloseLogger()

	// 创建上下文，用于优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 统一的信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("main", "Received shutdown signal, initiating graceful shutdown...")
		cancel() // 触发上下文取消，通知所有组件优雅关闭
	}()

	logger.Info("main", "Initializing xtopus operator with three-tier architecture...")

	// 生成节点配置文件
	if err := utils.GenerateNodeConfigFile(*nodeDir); err != nil {
		logger.Errorf("main", "Failed to generate node configuration file: %v", err)
		// 不中断程序，继续运行
	}

	// 初始化基于文件的 StatusRepository
	statusRepo := repository.NewFileStatusRepository(*statusDir, logger)

	// 启动传统的文件监控控制器（保持现有功能）
	controller, err := controller.NewController(*configDir, *nodeDir, statusRepo, logger)
	if err != nil {
		logger.Fatal("main", "Failed to create controller: "+err.Error())
	}

	// 启动控制器
	go func() {
		if err := controller.Run(); err != nil {
			logger.Errorf("main", "Error running controller: %v", err)
			cancel() // 控制器出错时触发优雅关闭
		}
	}()

	var grpcServer *grpc.Server
	if *enableAPI {
		// 初始化三层架构
		grpcServer, err = initializeApplication(*configDir, logger, ctx, statusRepo, cancel)
		if err != nil {
			logger.Fatal("main", "Failed to initialize application: "+err.Error())
		}
	}

	logger.Infof("main", "Starting xtopus operator with config directory: %s", *configDir)

	// 等待上下文取消（可能来自信号或组件错误）
	<-ctx.Done()
	logger.Info("main", "Shutting down...")

	// 优雅关闭gRPC服务器
	if grpcServer != nil {
		logger.Info("main", "Stopping gRPC server...")
		grpcServer.GracefulStop()
	}

	logger.Info("main", "Shutdown complete")
}
