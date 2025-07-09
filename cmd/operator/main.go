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
	v1 "go.xbrother.com/nix-operator/api/system/v1"
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
)

const (
	grpcPort = 9090
	httpPort = 8080
)

// 启动gRPC服务器
func StartGRPCServer(validators map[string]validator.SpecValidator, logger *utils.Logger, resourceService service.ResourceService, schemaProvider schema.Provider) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	// 注册HardwareConfigService
	v1.RegisterSystemConfigServiceServer(grpcServer, webcontroller.NewSystemServiceServer(validators, logger, resourceService, schemaProvider))

	go func() {
		log.Printf("Starting gRPC server on port %d...", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()

	return grpcServer, lis, nil
}

// 启动HTTP服务器（gRPC-Gateway）
func StartHTTPServer(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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
	if err := v1.RegisterSystemConfigServiceHandlerFromEndpoint(ctx, mux, fmt.Sprintf("localhost:%d", grpcPort), opts); err != nil {
		return fmt.Errorf("failed to register system service gateway: %v", err)
	}

	log.Printf("Starting HTTP server on port %d...", httpPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", httpPort), mux)
}

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

// Application 应用程序结构
type Application struct {
	ResourceController webcontroller.SystemConfigServiceServer
	ResourceService    service.ResourceService
	ConfigRepository   repository.ConfigRepository
}

// initializeApplication 初始化三层架构应用
func initializeApplication(configDir string, logger *utils.Logger, ctx context.Context, controller *controller.Controller) (*grpc.Server, error) {

	// 初始化校验器映射
	validators := initializeValidators()

	// 初始化 Repository 层
	configRepo := repository.NewConfigRepository(configDir, logger)

	// 初始化 Service 层
	resourceService := service.NewResourceService(configRepo, controller, logger)

	// 初始化 Schema Provider
	schemaProvider := schema.NewProvider()

	// 启动gRPC服务器
	grpcServer, _, err := StartGRPCServer(validators, logger, resourceService, schemaProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to start gRPC server: %v", err)
	}

	// 启动HTTP服务器
	go func() {
		if err := StartHTTPServer(ctx); err != nil {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	return grpcServer, nil
}

// initializeValidators 初始化校验器映射
func initializeValidators() map[string]validator.SpecValidator {
	validators := make(map[string]validator.SpecValidator)

	// 使用 proto 定义创建校验器
	validators["HostsConfiguration"] = validator.NewProtoValidator(
		"HostsConfiguration",
		&systemv1.HostsConfigurationSpec{},
	)

	// 添加 TimeConfiguration 校验器
	validators["TimeConfiguration"] = validator.NewProtoValidator(
		"TimeConfiguration",
		&systemv1.TimeConfigurationSpec{},
	)

	return validators
}

func main() {
	configDir := flag.String("config-dir", "etc/cr.d", "Path to configuration directory")
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

	logger.Info("main", "Initializing xtopus operator with three-tier architecture...")

	// 启动传统的文件监控控制器（保持现有功能）
	controller, err := controller.NewController(*configDir, logger)
	if err != nil {
		logger.Fatal("main", "Failed to create controller: "+err.Error())
	}

	// 启动控制器
	go func() {
		if err := controller.Run(); err != nil {
			logger.Fatal("main", "Error running controller: "+err.Error())
		}
	}()

	var grpcServer *grpc.Server
	if *enableAPI {
		// 初始化三层架构
		grpcServer, err = initializeApplication(*configDir, logger, ctx, controller)
		if err != nil {
			logger.Fatal("main", "Failed to initialize application: "+err.Error())
		}
		// 确保在程序退出时优雅关闭gRPC服务器
		defer func() {
			if grpcServer != nil {
				grpcServer.GracefulStop()
			}
		}()
	}

	logger.Infof("main", "Starting xtopus operator with config directory: %s", *configDir)

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down...")
}
