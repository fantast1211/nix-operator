package webservice

import (
	"context"
	"fmt"

	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/interfaces"
	"go.xbrother.com/nix-operator/pkg/utils"
	"go.xbrother.com/nix-operator/pkg/validator"
)

// resourceService 资源服务实现
type resourceService struct {
	configRepo        interfaces.ConfigRepository
	reconcileRepo     interfaces.ReconcileRepository
	validatorRegistry validator.ValidatorRegistry
	logger            *utils.Logger
}

// NewResourceService 创建资源服务
func NewResourceService(
	configRepo interfaces.ConfigRepository,
	reconcileRepo interfaces.ReconcileRepository,
	validatorRegistry validator.ValidatorRegistry,
	logger *utils.Logger,
) interfaces.ResourceService {
	return &resourceService{
		configRepo:        configRepo,
		reconcileRepo:     reconcileRepo,
		validatorRegistry: validatorRegistry,
		logger:            logger,
	}
}

// ListResources 列出资源
func (s *resourceService) ListResources(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error) {
	s.logger.Debugf("resource_service", "Listing resources of kind: %s", kind)

	// 调用配置仓储获取资源列表（内部会触发调谐）
	resources, err := s.configRepo.List(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %v", err)
	}

	s.logger.Debugf("resource_service", "Found %d resources of kind: %s", len(resources), kind)
	return resources, nil
}

// GetResource 获取单个资源
func (s *resourceService) GetResource(ctx context.Context, name string) (*domain.ResourceWithStatus, error) {
	s.logger.Debugf("resource_service", "Getting resource: %s", name)

	// 调用配置仓储获取资源（内部会触发调谐）
	resource, err := s.configRepo.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource %s: %v", name, err)
	}

	s.logger.Debugf("resource_service", "Retrieved resource: %s (kind: %s)", name, resource.Resource.Kind)
	return resource, nil
}

// UpdateResource 更新资源
func (s *resourceService) UpdateResource(ctx context.Context, resource *domain.Resource) (*domain.Resource, error) {
	s.logger.Debugf("resource_service", "Updating resource: %s (kind: %s)", resource.Metadata.Name, resource.Kind)

	// 1. 类型校验
	validator := s.validatorRegistry.GetValidator(resource.Kind)
	if validator == nil {
		return nil, fmt.Errorf("no validator found for kind: %s", resource.Kind)
	}

	if err := validator.Validate(resource.Spec); err != nil {
		return nil, fmt.Errorf("validation failed: %v", err)
	}

	// 2. 转换为配置对象
	config := &domain.ResourceConfig{
		APIVersion: resource.APIVersion,
		Kind:       resource.Kind,
		Metadata:   resource.Metadata,
		Spec:       resource.Spec,
	}

	// 3. 保存配置（不立即调谐，依赖文件监控）
	if err := s.configRepo.Save(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to save resource: %v", err)
	}

	s.logger.Infof("resource_service", "Resource updated successfully: %s", resource.Metadata.Name)
	return resource, nil
}

// DeleteResource 删除资源
func (s *resourceService) DeleteResource(ctx context.Context, name string) error {
	s.logger.Debugf("resource_service", "Deleting resource: %s", name)

	// 调用配置仓储删除资源
	if err := s.configRepo.Delete(ctx, name); err != nil {
		return fmt.Errorf("failed to delete resource %s: %v", name, err)
	}

	s.logger.Infof("resource_service", "Resource deleted successfully: %s", name)
	return nil
}