package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	v1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	grpcPort = 9090
	httpPort = 8080
)

// HardwareConfigServiceServer 实现 HardwareConfigService 接口
type HardwareConfigServiceServer struct {
	v1.UnimplementedHardwareConfigServiceServer
	controller *controller.Controller
}

// ListResourceConfigs 获取所有资源配置
func (s *HardwareConfigServiceServer) ListResourceConfigs(ctx context.Context, req *v1.ListResourceConfigsRequest) (*v1.ListResourceConfigsResponse, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 从控制器获取资源配置
	resources, err := s.controller.ListResourceConfigs(ctx, req.Kind)
	if err != nil {
		return nil, err
	}

	// 转换为API响应格式
	response := &v1.ListResourceConfigsResponse{
		Resources: make([]*v1.Resource, 0, len(resources)),
	}

	for _, res := range resources {
		apiResource, err := convertToAPIResource(res)
		if err != nil {
			continue
		}
		response.Resources = append(response.Resources, apiResource)
	}

	return response, nil
}

// GetResourceConfig 获取资源配置
func (s *HardwareConfigServiceServer) GetResourceConfig(ctx context.Context, req *v1.GetResourceConfigRequest) (*v1.Resource, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 从控制器获取资源配置
	res, err := s.controller.GetResourceConfig(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	// 转换为API响应格式
	return convertToAPIResource(res)
}

// UpdateResourceConfig 更新资源配置
func (s *HardwareConfigServiceServer) UpdateResourceConfig(ctx context.Context, req *v1.UpdateResourceConfigRequest) (*v1.Resource, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 验证请求
	if req.Resource == nil || req.Resource.Config == nil {
		return nil, fmt.Errorf("invalid request: resource or config is nil")
	}

	// 确保名称匹配
	if req.Resource.Config.Metadata == nil {
		req.Resource.Config.Metadata = &v1.Metadata{Name: req.Name}
	} else if req.Resource.Config.Metadata.Name != req.Name {
		return nil, fmt.Errorf("name in path (%s) does not match name in body (%s)", req.Name, req.Resource.Config.Metadata.Name)
	}

	// 转换为内部配置格式
	cfg, err := convertFromAPIResource(req.Resource)
	if err != nil {
		return nil, err
	}

	// 更新资源配置
	updatedRes, err := s.controller.UpdateResourceConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// 转换为API响应格式
	return convertToAPIResource(updatedRes)
}

// 启动gRPC服务器
func StartGRPCServer(controller *controller.Controller) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	v1.RegisterHardwareConfigServiceServer(grpcServer, &HardwareConfigServiceServer{controller: controller})

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

	if err := v1.RegisterHardwareConfigServiceHandlerFromEndpoint(ctx, mux, fmt.Sprintf("localhost:%d", grpcPort), opts); err != nil {
		return fmt.Errorf("failed to register gateway: %v", err)
	}

	log.Printf("Starting HTTP server on port %d...", httpPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", httpPort), mux)
}

// 转换内部资源配置到API资源
func convertToAPIResource(res *controller.ResourceWithStatus) (*v1.Resource, error) {
	if res == nil {
		return nil, fmt.Errorf("resource is nil")
	}

	// 转换配置
	cfg := &v1.ResourceConfig{
		ApiVersion: res.Config.APIVersion,
		Kind:       res.Config.Kind,
		Metadata: &v1.Metadata{
			Name:            res.Config.Metadata.Name,
			ResourceVersion: res.Config.Metadata.ResourceVersion,
			Generation:      int32(res.Config.Metadata.Generation),
			CreationTime:    res.Config.Metadata.CreationTime,
			DeletionTime:    res.Config.Metadata.DeletionTime,
			Labels:          res.Config.Metadata.Labels,
			Annotations:     res.Config.Metadata.Annotations,
		},
	}

	// 转换spec为字符串
	specBytes, err := json.Marshal(res.Config.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %v", err)
	}
	cfg.Spec = string(specBytes)

	// 构造API资源
	apiResource := &v1.Resource{
		Config: cfg,
	}

	// 如果有有效配置，也转换它
	if res.EffectiveConfig != nil {
		effectiveCfg := &v1.ResourceConfig{
			ApiVersion: res.EffectiveConfig.APIVersion,
			Kind:       res.EffectiveConfig.Kind,
			Metadata: &v1.Metadata{
				Name:            res.EffectiveConfig.Metadata.Name,
				ResourceVersion: res.EffectiveConfig.Metadata.ResourceVersion,
				Generation:      int32(res.EffectiveConfig.Metadata.Generation),
				CreationTime:    res.EffectiveConfig.Metadata.CreationTime,
				DeletionTime:    res.EffectiveConfig.Metadata.DeletionTime,
				Labels:          res.EffectiveConfig.Metadata.Labels,
				Annotations:     res.EffectiveConfig.Metadata.Annotations,
			},
		}

		// 转换spec为字符串
		effectiveSpecBytes, err := json.Marshal(res.EffectiveConfig.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal effective spec: %v", err)
		}
		effectiveCfg.Spec = string(effectiveSpecBytes)

		apiResource.EffectiveConfig = effectiveCfg
	}

	// 转换状态
	if res.Status != nil {
		// 解析时间字符串为时间戳
		var lastReconcileTime *timestamppb.Timestamp
		if res.Status.LastReconcileTime != "" {
			t, err := time.Parse(time.RFC3339, res.Status.LastReconcileTime)
			if err == nil {
				lastReconcileTime = timestamppb.New(t)
			}
		}

		apiResource.Status = &v1.ResourceStatus{
			Phase:             res.Status.Phase,
			Reason:            res.Status.Reason,
			Message:           res.Status.Message,
			LastReconcileTime: lastReconcileTime,
		}
	}

	return apiResource, nil
}

// 转换API资源到内部资源配置
func convertFromAPIResource(apiResource *v1.Resource) (*config.ResourceConfig, error) {
	if apiResource == nil || apiResource.Config == nil {
		return nil, fmt.Errorf("resource or config is nil")
	}

	// 转换基本字段
	cfg := &config.ResourceConfig{
		APIVersion: apiResource.Config.ApiVersion,
		Kind:       apiResource.Config.Kind,
	}

	// 转换元数据
	if apiResource.Config.Metadata != nil {
		cfg.Metadata = config.Metadata{
			Name:            apiResource.Config.Metadata.Name,
			ResourceVersion: apiResource.Config.Metadata.ResourceVersion,
			Generation:      int(apiResource.Config.Metadata.Generation),
			CreationTime:    apiResource.Config.Metadata.CreationTime,
			DeletionTime:    apiResource.Config.Metadata.DeletionTime,
			Labels:          apiResource.Config.Metadata.Labels,
			Annotations:     apiResource.Config.Metadata.Annotations,
		}
	}

	// 解析spec字符串为JSON
	if apiResource.Config.Spec != "" {
		var specMap map[string]interface{}
		if err := json.Unmarshal([]byte(apiResource.Config.Spec), &specMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec: %v", err)
		}

		// 转换为JSON原始消息
		specBytes, err := json.Marshal(specMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal spec map: %v", err)
		}
		cfg.Spec = specBytes
	}

	return cfg, nil
}
