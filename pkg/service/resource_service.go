package service

import (
	"context"
	"fmt"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/repository"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// resourceService 资源服务实现
type resourceService struct {
	configRepo repository.ConfigRepository
	controller *controller.Controller
	logger     *utils.Logger
}

// NewResourceService 创建资源服务实例
func NewResourceService(configRepo repository.ConfigRepository, controller *controller.Controller, logger *utils.Logger) ResourceService {
	return &resourceService{
		configRepo: configRepo,
		controller: controller,
		logger:     logger,
	}
}

// ListResources 列出指定类型的资源
func (s *resourceService) ListResources(ctx context.Context, kind string) ([]*systemv1.Resource, error) {
	s.logger.Debugf("service", "Listing resources of kind: %s", kind)

	// 从配置仓库获取配置列表
	configs, err := s.configRepo.ListConfigs(ctx, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	// 为每个配置执行调谐并构建资源
	resources := make([]*systemv1.Resource, 0, len(configs))
	for _, config := range configs {
		resource, err := s.buildResourceWithStatus(ctx, config)
		if err != nil {
			s.logger.Warnf("service", "Failed to build resource for config %s: %v", config.Metadata.Name, err)
			// 创建一个错误状态的资源
			resource = &systemv1.Resource{
				Config: config,
				Status: &systemv1.ResourceStatus{
					Phase:   "Failed",
					Reason:  "ReconcileError",
					Message: err.Error(),
				},
			}
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
	config, err := s.configRepo.GetConfig(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	// 构建带状态的资源
	resource, err := s.buildResourceWithStatus(ctx, config)
	if err != nil {
		s.logger.Warnf("service", "Failed to build resource for config %s: %v", config.Metadata.Name, err)
		// 返回一个错误状态的资源
		resource = &systemv1.Resource{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   "Failed",
				Reason:  "ReconcileError",
				Message: err.Error(),
			},
		}
	}

	s.logger.Debugf("service", "Retrieved resource: %s", name)
	return resource, nil
}

// UpdateResource 更新资源
func (s *resourceService) UpdateResource(ctx context.Context, resourceConfig *systemv1.ResourceConfig) (*systemv1.Resource, error) {
	s.logger.Debugf("service", "Updating resource: %s", resourceConfig.Metadata.Name)

	// 保存配置到文件
	if err := s.configRepo.SaveConfig(ctx, resourceConfig); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	// 构建带状态的资源
	resource, err := s.buildResourceWithStatus(ctx, resourceConfig)
	if err != nil {
		s.logger.Warnf("service", "Failed to build resource for config %s: %v", resourceConfig.Metadata.Name, err)
		// 返回一个错误状态的资源
		resource = &systemv1.Resource{
			Config: resourceConfig,
			Status: &systemv1.ResourceStatus{
				Phase:   "Failed",
				Reason:  "ReconcileError",
				Message: err.Error(),
			},
		}
	}

	s.logger.Infof("service", "Updated resource: %s, status: %s", resourceConfig.Metadata.Name, resource.Status.Phase)
	return resource, nil
}

// buildResourceWithStatus 构建带状态的资源
func (s *resourceService) buildResourceWithStatus(ctx context.Context, config *systemv1.ResourceConfig) (*systemv1.Resource, error) {
	// 获取对应kind的handler
	handler := s.controller.GetHandler(config.Kind)
	if handler == nil {
		return nil, fmt.Errorf("no handler found for kind: %s", config.Kind)
	}

	// 执行调谐获取最新状态
	results, err := handler.Reconcile(ctx, []*systemv1.ResourceConfig{config})
	if err != nil {
		reconcileResult, _ := status.ReconcileError(config, status.ReasonReconcileError, fmt.Errorf("Reconciliation error: %v", err))
		return &systemv1.Resource{
			Config: config,
			Status: reconcileResult.Status,
		}, nil
	}

	// 找到对应配置的结果
	var result *domain.ReconcileResult
	for _, r := range results {
		if r != nil && r.Effective != nil && r.Effective.Metadata != nil && r.Effective.Metadata.Name == config.Metadata.Name {
			result = r
			break
		}
	}

	// 如果没有找到对应的结果，使用第一个结果或创建默认结果
	if result == nil && len(results) > 0 {
		result = results[0]
	}
	if result == nil {
		reconcileResult, _ := status.ReconcileError(config, status.ReasonReconcileError, fmt.Errorf("No reconcile result found"))
		return &systemv1.Resource{
			Config: config,
			Status: reconcileResult.Status,
		}, nil
	}

	// 返回资源状态
	return &systemv1.Resource{
		Config:          config,
		EffectiveConfig: result.Effective,
		Status:          result.Status,
	}, nil
}
