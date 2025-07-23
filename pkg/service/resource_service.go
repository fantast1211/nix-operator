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
	statusRepo repository.StatusRepository
	logger     *utils.Logger
}

// NewResourceService 创建资源服务实例
func NewResourceService(configRepo repository.ConfigRepository, statusRepo repository.StatusRepository, logger *utils.Logger) ResourceService {
	return &resourceService{
		configRepo: configRepo,
		statusRepo: statusRepo,
		logger:     logger,
	}
}

// ListResources 列出指定类型的资源
func (s *resourceService) ListResources(ctx context.Context, kind string) ([]*systemv1.ResourceConfig, error) {
	s.logger.Debugf("service", "Listing resources of kind: %s", kind)

	// 从配置仓库获取配置列表
	configs, err := s.configRepo.ListConfigs(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	// 从状态仓库获取该类型的所有状态
	statuses, err := s.statusRepo.ListStatuses(ctx, kind)
	if err != nil {
		s.logger.Warnf("service", "Failed to list statuses for kind %s: %v", kind, err)
		statuses = make(map[string]*systemv1.ResourceStatus) // 使用空状态映射
	}

	// 构建资源列表
	resources := make([]*systemv1.ResourceConfig, 0, len(configs))
	for _, config := range configs {
		// 复制配置以避免修改原始数据
		resourceConfig := &systemv1.ResourceConfig{
			ApiVersion: config.ApiVersion,
			Kind:       config.Kind,
			Metadata:   config.Metadata,
			Spec:       config.Spec,
		}

		// 从缓存中获取状态
		if status, exists := statuses[config.Metadata.Name]; exists {
			resourceConfig.Status = status
		} else {
			// 如果没有缓存状态，设置为未知状态
			resourceConfig.Status = &systemv1.ResourceStatus{
				Phase:   "Unknown",
				Reason:  "Initial",
				Message: "Status not yet available, reconciliation may be in progress",
			}
		}

		resources = append(resources, resourceConfig)
	}

	s.logger.Debugf("service", "Found %d resources of kind: %s", len(resources), kind)
	return resources, nil
}

// GetResource 获取指定名称的资源
func (s *resourceService) GetResource(ctx context.Context, name string) (*systemv1.ResourceConfig, error) {
	s.logger.Debugf("service", "Getting resource: %s", name)

	// 从配置仓库获取配置
	config, err := s.configRepo.GetConfig(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	// 构建资源配置
	resourceConfig := &systemv1.ResourceConfig{
		ApiVersion: config.ApiVersion,
		Kind:       config.Kind,
		Metadata:   config.Metadata,
		Spec:       config.Spec,
	}

	// 从状态仓库获取状态
	resourceStatus, err := s.statusRepo.GetStatus(ctx, name)
	if err != nil {
		s.logger.Debugf("service", "No cached status found for resource %s: %v", name, err)
		// 如果没有缓存状态，设置为未知状态
		resourceConfig.Status = &systemv1.ResourceStatus{
			Phase:   "Unknown",
			Reason:  "Initial",
			Message: "Status not yet available, reconciliation may be in progress",
		}
	} else {
		resourceConfig.Status = resourceStatus
	}

	s.logger.Debugf("service", "Retrieved resource: %s", name)
	return resourceConfig, nil
}

// UpdateResource 更新资源
func (s *resourceService) UpdateResource(ctx context.Context, resourceConfig *systemv1.ResourceConfig) (*systemv1.ResourceConfig, error) {
	s.logger.Debugf("service", "Updating resource: %s", resourceConfig.Metadata.Name)

	// 保存配置到文件
	if err := s.configRepo.SaveConfig(ctx, resourceConfig); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	// 构建返回的资源配置
	resultConfig := &systemv1.ResourceConfig{
		ApiVersion: resourceConfig.ApiVersion,
		Kind:       resourceConfig.Kind,
		Metadata:   resourceConfig.Metadata,
		Spec:       resourceConfig.Spec,
	}

	// 从状态仓库获取当前状态
	resourceStatus, err := s.statusRepo.GetStatus(ctx, resourceConfig.Metadata.Name)
	if err != nil {
		s.logger.Debugf("service", "No cached status found for resource %s: %v", resourceConfig.Metadata.Name, err)
		// 配置已更新，但reconciliation尚未完成
		resultConfig.Status = &systemv1.ResourceStatus{
			Phase:   "Unknown",
			Reason:  "ConfigurationUpdated",
			Message: "Configuration updated, reconciliation will be triggered automatically",
		}
	} else {
		resultConfig.Status = resourceStatus
	}

	s.logger.Infof("service", "Updated resource: %s, current status: %s", resourceConfig.Metadata.Name, resultConfig.Status.Phase)
	return resultConfig, nil
}
