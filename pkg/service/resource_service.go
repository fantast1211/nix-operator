package service

import (
	"context"
	"fmt"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/repository"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// resourceService 资源服务实现
type resourceService struct {
	configRepo repository.ConfigRepository
	logger     *utils.Logger
}

// NewResourceService 创建资源服务实例
func NewResourceService(configRepo repository.ConfigRepository, logger *utils.Logger) ResourceService {
	return &resourceService{
		configRepo: configRepo,
		logger:     logger,
	}
}

// ListResources 列出指定类型的资源
func (s *resourceService) ListResources(ctx context.Context, kind string) ([]*systemv1.Resource, error) {
	s.logger.Debugf("service", "Listing resources of kind: %s", kind)

	// 从配置仓库获取配置列表
	resourcesWithStatus, err := s.configRepo.List(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	// 转换为 systemv1.ResourceConfig
	resources := make([]*systemv1.Resource, 0, len(resourcesWithStatus))
	for _, rs := range resourcesWithStatus {
		resource := &systemv1.Resource{
			Config:          rs.Config,
			EffectiveConfig: rs.EffectiveConfig,
			Status:          rs.Status,
		}

		resources = append(resources, resource)
	}

	s.logger.Debugf("service", "Found %d resources of kind: %s", len(resources), kind)
	return resources, nil
}

// GetResource 获取指定名称的资源
func (s *resourceService) GetResource(ctx context.Context, name string) (*systemv1.Resource, error) {
	s.logger.Debugf("service", "Getting resource: %s", name)

	// 从配置仓库获取配置
	rs, err := s.configRepo.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	// 检查 rs 是否为 nil
	if rs == nil {
		return nil, fmt.Errorf("resource not found: %s", name)
	}

	// 转换为 systemv1.ResourceConfig
	resource := &systemv1.Resource{
		Config:          rs.Config,
		EffectiveConfig: rs.EffectiveConfig,
		Status:          rs.Status,
	}

	s.logger.Debugf("service", "Retrieved resource: %s", name)
	return resource, nil
}

// UpdateResource 更新资源
func (s *resourceService) UpdateResource(ctx context.Context, resourceConfig *systemv1.ResourceConfig) (*systemv1.Resource, error) {
	s.logger.Debugf("service", "Updating resource: %s", resourceConfig.Metadata.Name)

	// 转换为 domain.ResourceConfig
	config := &systemv1.ResourceConfig{
		ApiVersion: resourceConfig.ApiVersion,
		Kind:       resourceConfig.Kind,
		Metadata:   resourceConfig.Metadata,
		Spec:       resourceConfig.Spec,
	}

	var rs systemv1.Resource

	// 保存配置到文件
	if err := s.configRepo.Save(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	// 执行调谐获取最新状态
	result, err := s.configRepo.Reconcile(ctx, config)
	rs.Config = config
	rs.EffectiveConfig = result.Effective
	if err != nil {
		s.logger.Warnf("service", "Reconcile failed for resource %s: %v", resourceConfig.Metadata.Name, err)
		// 即使调谐失败，也返回更新后的资源（状态可能为错误状态）
		rs.Status = &systemv1.ResourceStatus{
			Phase:   "Failed",
			Reason:  "ReconcileError",
			Message: err.Error(),
		}
	} else {
		rs.Status = result.Status
	}

	s.logger.Infof("service", "Updated resource: %s, status: %s", resourceConfig.Metadata.Name, rs.Status.Phase)
	return &rs, nil
}
