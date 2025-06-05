package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// ResourceWithStatus 包含资源配置和状态
type ResourceWithStatus struct {
	Config          *config.ResourceConfig
	EffectiveConfig *config.ResourceConfig
	Status          *config.ResourceStatus
}

// 资源状态常量
const (
	PhaseReady   = "Ready"
	PhaseError   = "Error"
	PhaseUnknown = "Unknown"

	ReasonNoHandler      = "NoHandler"
	ReasonReconcileError = "ReconcileError"
)

// ListResourceConfigs 获取所有资源配置
func (c *Controller) ListResourceConfigs(ctx context.Context, kind string) ([]*ResourceWithStatus, error) {
	var resources []*ResourceWithStatus

	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 扫描配置目录中的所有配置文件
	err := filepath.Walk(c.configDir, func(path string, info os.FileInfo, err error) error {
		// 检查上下文是否已取消
		if ctxErr := ctx.Err(); ctxErr != nil {
			return err
		}

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
		cfg, err := loadConfigFile(path)
		if err != nil {
			log.Printf("Error loading config file %s: %v", path, err)
			return nil
		}

		// 如果指定了kind，则过滤
		if kind != "" && cfg.Kind != kind {
			return nil
		}

		// 查找对应的处理器
		handler, exists := c.handlers[cfg.Kind]
		if !exists {
			// 没有处理器，只返回配置
			resources = append(resources, &ResourceWithStatus{
				Config: cfg,
				Status: &config.ResourceStatus{
					Phase:   PhaseUnknown,
					Reason:  ReasonNoHandler,
					Message: fmt.Sprintf("No handler found for kind: %s", cfg.Kind),
				},
			})
			return nil
		}

		// 执行调谐以获取最新状态
		result, err := handler.Reconcile(ctx, cfg)
		if err != nil {
			resources = append(resources, &ResourceWithStatus{
				Config: cfg,
				Status: &config.ResourceStatus{
					Phase:   PhaseError,
					Reason:  ReasonReconcileError,
					Message: fmt.Sprintf("Reconciliation error: %v", err),
				},
			})
			return nil
		}

		// 添加到结果列表
		resources = append(resources, &ResourceWithStatus{
			Config:          cfg,
			EffectiveConfig: result.Effective,
			Status:          result.Status,
		})

		return nil
	})

	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		log.Printf("Error walking config directory: %v", err)
		return nil, fmt.Errorf("failed to list resources: %v", err)
	}

	return resources, nil
}

// GetResourceConfig 获取指定名称的资源配置
func (c *Controller) GetResourceConfig(ctx context.Context, name string) (*ResourceWithStatus, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 扫描配置目录中的所有配置文件
	var foundResource *ResourceWithStatus

	err := filepath.Walk(c.configDir, func(path string, info os.FileInfo, err error) error {
		// 检查上下文是否已取消
		if ctxErr := ctx.Err(); ctxErr != nil {
			return err
		}

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
		cfg, err := loadConfigFile(path)
		if err != nil {
			log.Printf("Error loading config file %s: %v", path, err)
			return nil
		}

		// 检查名称是否匹配
		if cfg.Metadata.Name != name {
			return nil
		}

		// 找到匹配的资源
		// 查找对应的处理器
		handler, exists := c.handlers[cfg.Kind]
		if !exists {
			// 没有处理器，只返回配置
			foundResource = &ResourceWithStatus{
				Config: cfg,
				Status: &config.ResourceStatus{
					Phase:   PhaseUnknown,
					Reason:  ReasonNoHandler,
					Message: fmt.Sprintf("No handler found for kind: %s", cfg.Kind),
				},
			}
			return filepath.SkipAll
		}

		// 执行调谐以获取最新状态
		result, err := handler.Reconcile(ctx, cfg)
		if err != nil {
			foundResource = &ResourceWithStatus{
				Config: cfg,
				Status: &config.ResourceStatus{
					Phase:   PhaseError,
					Reason:  ReasonReconcileError,
					Message: fmt.Sprintf("Reconciliation error: %v", err),
				},
			}
		} else {
			foundResource = &ResourceWithStatus{
				Config:          cfg,
				EffectiveConfig: result.Effective,
				Status:          result.Status,
			}
		}

		return filepath.SkipAll
	})

	if err != nil && err != filepath.SkipAll && err != context.Canceled && err != context.DeadlineExceeded {
		return nil, fmt.Errorf("error searching for resource: %v", err)
	}

	if foundResource == nil {
		return nil, fmt.Errorf("resource not found: %s", name)
	}

	return foundResource, nil
}

// UpdateResourceConfig 更新资源配置
func (c *Controller) UpdateResourceConfig(ctx context.Context, cfg *config.ResourceConfig) (*ResourceWithStatus, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 验证配置
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	if cfg.Metadata.Name == "" {
		return nil, fmt.Errorf("resource name is required")
	}

	// 查找对应的处理器
	handler, exists := c.handlers[cfg.Kind]
	if !exists {
		return nil, fmt.Errorf("no handler found for kind: %s", cfg.Kind)
	}

	// 生成或更新资源版本
	if cfg.Metadata.ResourceVersion == "" {
		cfg.Metadata.ResourceVersion = "1"
	} else {
		// 尝试解析并递增资源版本
		version, err := strconv.Atoi(cfg.Metadata.ResourceVersion)
		if err == nil {
			cfg.Metadata.ResourceVersion = strconv.Itoa(version + 1)
		}
	}

	// 更新生成号
	cfg.Metadata.Generation++

	// 如果没有创建时间，设置当前时间
	if cfg.Metadata.CreationTime == "" {
		cfg.Metadata.CreationTime = time.Now().Format(time.RFC3339)
	}

	// 保存配置文件
	filePath := filepath.Join(c.configDir, fmt.Sprintf("%s.json", cfg.Metadata.Name))
	cfgData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %v", err)
	}

	if writeErr := utils.AtomicWriteFile(cfgData, filePath, 0644); writeErr != nil {
		return nil, fmt.Errorf("failed to write config file: %v", writeErr)
	}

	// 执行调谐
	result, err := handler.Reconcile(ctx, cfg)
	if err != nil {
		return &ResourceWithStatus{
			Config: cfg,
			Status: &config.ResourceStatus{
				Phase:             PhaseError,
				Reason:            ReasonReconcileError,
				Message:           fmt.Sprintf("Reconciliation error: %v", err),
				LastReconcileTime: time.Now().Format(time.RFC3339),
			},
		}, nil
	}

	// 如果状态中没有最后调谐时间，添加当前时间
	if result.Status != nil && result.Status.LastReconcileTime == "" {
		result.Status.LastReconcileTime = time.Now().Format(time.RFC3339)
	}

	return &ResourceWithStatus{
		Config:          cfg,
		EffectiveConfig: result.Effective,
		Status:          result.Status,
	}, nil
}
