package service

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	v1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	grpcPort = 9090
	httpPort = 8080
)

// 启动gRPC服务器
func StartGRPCServer(controller *controller.Controller) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()

	// 注册HardwareConfigService
	v1.RegisterHardwareConfigServiceServer(grpcServer, &HardwareConfigServiceServer{controller: controller})

	// 注册SystemService
	v1.RegisterSystemServiceServer(grpcServer, NewSystemServiceServer(controller))

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

	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// 注册HardwareConfigService的HTTP处理程序
	if err := v1.RegisterHardwareConfigServiceHandlerFromEndpoint(ctx, mux, fmt.Sprintf("localhost:%d", grpcPort), opts); err != nil {
		return fmt.Errorf("failed to register hardware config gateway: %v", err)
	}

	// 注册SystemService的HTTP处理程序
	if err := v1.RegisterSystemServiceHandlerFromEndpoint(ctx, mux, fmt.Sprintf("localhost:%d", grpcPort), opts); err != nil {
		return fmt.Errorf("failed to register system service gateway: %v", err)
	}

	log.Printf("Starting HTTP server on port %d...", httpPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", httpPort), mux)
}
