package webrepository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/interfaces"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// reconcileRepository 调谐仓储实现
type reconcileRepository struct {
	configDir string
	handlers  map[string]controller.Handler // 复用现有的Handler
	osInfo    domain.OSInfo
	logger    *utils.Logger
}

// NewReconcileRepository 创建调谐仓储
func NewReconcileRepository(configDir string, handlers map[string]controller.Handler, osInfo domain.OSInfo, logger *utils.Logger) interfaces.ReconcileRepository {
	return &reconcileRepository{
		configDir: configDir,
		handlers:  handlers,
		osInfo:    osInfo,
		logger:    logger,
	}
}

// Reconcile 执行调谐获取实时状态（供查询操作使用）
func (r *reconcileRepository) Reconcile(ctx context.Context, cfg *domain.ResourceConfig) (*domain.ReconcileResult, error) {
	r.logger.Debugf("reconcile_repository", "Reconciling config: %s (kind: %s)", cfg.Metadata.Name, cfg.Kind)

	// 查找对应的处理器
	handler, exists := r.handlers[cfg.Kind]
	if !exists {
		return nil, fmt.Errorf("no handler found for kind: %s", cfg.Kind)
	}

	// 转换为现有controller的类型
	legacyConfig := &config.ResourceConfig{
		APIVersion: cfg.APIVersion,
		Kind:       cfg.Kind,
		Metadata: config.Metadata{
			Name:            cfg.Metadata.Name,
			ResourceVersion: cfg.Metadata.ResourceVersion,
			Generation:      cfg.Metadata.Generation,
			Labels:          cfg.Metadata.Labels,
			Annotations:     cfg.Metadata.Annotations,
		},
		Spec: cfg.Spec,
	}

	// 执行调谐
	result, err := handler.Reconcile(ctx, legacyConfig)
	if err != nil {
		return nil, fmt.Errorf("reconciliation failed: %v", err)
	}

	// 转换结果
	domainResult := &domain.ReconcileResult{
		Effective: &domain.ResourceConfig{
			APIVersion: result.Effective.APIVersion,
			Kind:       result.Effective.Kind,
			Metadata: domain.Metadata{
				Name:            result.Effective.Metadata.Name,
				ResourceVersion: result.Effective.Metadata.ResourceVersion,
				Generation:      result.Effective.Metadata.Generation,
				Labels:          result.Effective.Metadata.Labels,
				Annotations:     result.Effective.Metadata.Annotations,
			},
			Spec: result.Effective.Spec,
		},
		Status: &domain.ResourceStatus{
			Phase:   result.Status.Phase,
			Reason:  result.Status.Reason,
			Message: result.Status.Message,
		},
	}

	r.logger.Debugf("reconcile_repository", "Reconciliation completed for %s: %s", cfg.Metadata.Name, result.Status.Phase)
	return domainResult, nil
}

// ReconcileAll 批量调谐（供文件监控使用）
func (r *reconcileRepository) ReconcileAll(ctx context.Context) error {
	r.logger.Debug("reconcile_repository", "Starting reconciliation of all configs")

	// 扫描配置目录中的所有配置文件
	err := filepath.Walk(r.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录和非JSON文件
		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".json" {
			return nil
		}

		// 加载并处理配置文件
		cfg, err := r.loadConfigFile(path)
		if err != nil {
			r.logger.Errorf("reconcile_repository", "Error loading config file %s: %v", path, err)
			return nil // 继续处理其他文件
		}

		r.logger.Debugf("reconcile_repository", "Processing config file: %s (kind: %s)", path, cfg.Kind)

		// 执行调谐
		_, err = r.Reconcile(ctx, cfg)
		if err != nil {
			r.logger.Errorf("reconcile_repository", "Reconciliation error for %s: %v", path, err)
			return nil // 继续处理其他文件
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking config directory: %v", err)
	}

	r.logger.Info("reconcile_repository", "Reconciliation of all configs completed")
	return nil
}

// ListResourceConfigs 列出所有配置并执行调谐
func (r *reconcileRepository) ListResourceConfigs(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error) {
	r.logger.Debugf("reconcile_repository", "Listing resource configs of kind: %s", kind)

	var resources []*domain.ResourceWithStatus

	// 扫描配置目录
	err := filepath.Walk(r.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录和非JSON文件
		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".json" {
			return nil
		}

		// 加载配置文件
		cfg, err := r.loadConfigFile(path)
		if err != nil {
			r.logger.Errorf("reconcile_repository", "Error loading config file %s: %v", path, err)
			return nil // 继续处理其他文件
		}

		// 过滤指定类型
		if kind != "" && cfg.Kind != kind {
			return nil
		}

		// 执行调谐获取状态
		result, err := r.Reconcile(ctx, cfg)
		if err != nil {
			r.logger.Errorf("reconcile_repository", "Reconciliation error for %s: %v", path, err)
			// 即使调谐失败，也返回配置信息，但状态为错误
			resources = append(resources, &domain.ResourceWithStatus{
				Resource: &domain.Resource{
					APIVersion: cfg.APIVersion,
					Kind:       cfg.Kind,
					Metadata:   cfg.Metadata,
					Spec:       cfg.Spec,
				},
				Status: &domain.ResourceStatus{
					Phase:   "Error",
					Reason:  "ReconciliationFailed",
					Message: err.Error(),
				},
			})
			return nil
		}

		resources = append(resources, &domain.ResourceWithStatus{
			Resource: &domain.Resource{
				APIVersion: cfg.APIVersion,
				Kind:       cfg.Kind,
				Metadata:   cfg.Metadata,
				Spec:       cfg.Spec,
			},
			Status: result.Status,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking config directory: %v", err)
	}

	r.logger.Debugf("reconcile_repository", "Found %d resources of kind: %s", len(resources), kind)
	return resources, nil
}

// loadConfigFile 加载配置文件
func (r *reconcileRepository) loadConfigFile(path string) (*domain.ResourceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg domain.ResourceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}