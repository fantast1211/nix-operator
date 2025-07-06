package webcontroller

import (
	"context"
	"fmt"

	v1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/service"
	"go.xbrother.com/nix-operator/pkg/utils"
	"go.xbrother.com/nix-operator/pkg/validator"
	"google.golang.org/protobuf/types/known/anypb"
)

// SystemConfigServiceServer 实现 SystemConfigService 接口
type SystemConfigServiceServer struct {
	v1.UnimplementedSystemConfigServiceServer
	validators      map[string]validator.SpecValidator
	logger          *utils.Logger
	resourceService service.ResourceService
}

// NewSystemServiceServer 创建一个新的SystemServiceServer实例
func NewSystemServiceServer(validators map[string]validator.SpecValidator, logger *utils.Logger, resourceService service.ResourceService) *SystemConfigServiceServer {
	return &SystemConfigServiceServer{
		validators:      validators,
		logger:          logger,
		resourceService: resourceService,
	}
}

// ListResourceConfigs 列出资源配置
func (s *SystemConfigServiceServer) ListResourceConfigs(ctx context.Context, req *v1.ListResourceConfigsRequest) (*v1.ListResourceConfigsResponse, error) {
	s.logger.Debugf("controller", "Listing resource configs, kind: %s", req.Kind)

	// 调用服务层
	resources, err := s.resourceService.ListResources(ctx, req.Kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	s.logger.Debugf("controller", "Listed %d resource configs", len(resources))
	return &v1.ListResourceConfigsResponse{
		Resources: resources,
	}, nil
}

// GetResourceConfig 获取单个资源配置
func (s *SystemConfigServiceServer) GetResourceConfig(ctx context.Context, req *v1.GetResourceConfigRequest) (*v1.Resource, error) {
	s.logger.Debugf("controller", "Getting resource config: %s", req.Name)

	// 调用服务层
	resource, err := s.resourceService.GetResource(ctx, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	s.logger.Debugf("controller", "Retrieved resource config: %s", req.Name)
	return resource, nil
}

// UpdateResourceConfig 更新资源配置
func (s *SystemConfigServiceServer) UpdateResourceConfig(ctx context.Context, req *v1.UpdateResourceConfigRequest) (*v1.Resource, error) {
	s.logger.Debugf("controller", "Updating resource config: %s", req.Config.Metadata.Name)

	// 校验 spec
	if err := s.validateSpec(req.Config.Kind, req.Config.Spec); err != nil {
		return nil, fmt.Errorf("spec validation failed: %w", err)
	}

	// 调用服务层
	updatedResource, err := s.resourceService.UpdateResource(ctx, req.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to update resource: %w", err)
	}

	s.logger.Infof("controller", "Updated resource config: %s", req.Config.Metadata.Name)
	return updatedResource, nil
}

// validateSpec 校验 spec 内容
func (s *SystemConfigServiceServer) validateSpec(kind string, spec *anypb.Any) error {
	validator, exists := s.validators[kind]
	if !exists {
		return fmt.Errorf("no validator found for kind: %s", kind)
	}

	return validator.Validate(spec)
}
