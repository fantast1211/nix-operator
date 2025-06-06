package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
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

// -------------------- 工具方法 --------------------

// processConfigFile 处理单个配置文件并返回资源状态
func (c *Controller) processConfigFile(ctx context.Context, path string, kind string) (*ResourceWithStatus, error) {
	// 检查上下文是否已取消
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	// 加载配置文件
	cfg, err := loadConfigFile(path)
	if err != nil {
		log.Printf("Error loading config file %s: %v", path, err)
		return nil, err
	}

	// 如果指定了kind，则过滤
	if kind != "" && cfg.Kind != kind {
		return nil, nil
	}

	// 查找对应的处理器
	handler, exists := c.handlers[cfg.Kind]
	if !exists {
		// 没有处理器，只返回配置
		return &ResourceWithStatus{
			Config: cfg,
			Status: &config.ResourceStatus{
				Phase:   PhaseUnknown,
				Reason:  ReasonNoHandler,
				Message: fmt.Sprintf("No handler found for kind: %s", cfg.Kind),
			},
		}, nil
	}

	// 执行调谐以获取最新状态
	result, err := handler.Reconcile(ctx, cfg)
	if err != nil {
		return &ResourceWithStatus{
			Config: cfg,
			Status: &config.ResourceStatus{
				Phase:   PhaseError,
				Reason:  ReasonReconcileError,
				Message: fmt.Sprintf("Reconciliation error: %v", err),
			},
		}, nil
	}

	// 返回资源状态
	return &ResourceWithStatus{
		Config:          cfg,
		EffectiveConfig: result.Effective,
		Status:          result.Status,
	}, nil
}

// isJSONFile 检查文件是否为JSON文件
func isJSONFile(info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}

	ext := filepath.Ext(info.Name())
	return ext == ".json"
}

// checkSpecChanged 检查spec是否发生实质性变化
func checkSpecChanged(newSpec, oldSpec json.RawMessage) bool {
	if len(newSpec) == 0 && len(oldSpec) == 0 {
		return false
	}

	if len(newSpec) > 0 && len(oldSpec) > 0 {
		// 将两个JSON反序列化为map，然后使用reflect.DeepEqual比较
		var newSpecMap, oldSpecMap map[string]interface{}

		// 解析新旧spec
		newErr := json.Unmarshal(newSpec, &newSpecMap)
		oldErr := json.Unmarshal(oldSpec, &oldSpecMap)

		if newErr == nil && oldErr == nil {
			// 使用reflect.DeepEqual比较两个map是否相等
			return !reflect.DeepEqual(newSpecMap, oldSpecMap)
		}
		// 如果解析失败，保守起见认为发生了变化
		return true
	}

	// 一个有spec，另一个没有，视为变化
	return true
}

// updateMetadata 更新资源元数据
func updateMetadata(cfg, existingCfg *config.ResourceConfig) {
	// 检查spec是否发生实质性变化
	specChanged := checkSpecChanged(cfg.Spec, existingCfg.Spec)

	// ResourceVersion在任何变更时都自增
	version, err := strconv.Atoi(existingCfg.Metadata.ResourceVersion)
	if err == nil {
		cfg.Metadata.ResourceVersion = strconv.Itoa(version + 1)
	} else {
		cfg.Metadata.ResourceVersion = "1"
	}

	// Generation只在spec发生实质性变化时自增
	cfg.Metadata.Generation = existingCfg.Metadata.Generation
	if specChanged {
		cfg.Metadata.Generation++
	}
}

// initializeMetadata 初始化新资源的元数据
func initializeMetadata(cfg *config.ResourceConfig) {
	// 如果是新资源，初始化版本号和生成号
	cfg.Metadata.ResourceVersion = "1"
	cfg.Metadata.Generation = 1

	// 如果没有创建时间，设置当前时间
	if cfg.Metadata.CreationTime == "" {
		cfg.Metadata.CreationTime = time.Now().Format(time.RFC3339)
	}
}

// -------------------- 导出方法 --------------------

// ListResourceConfigs 获取所有资源配置
func (c *Controller) ListResourceConfigs(ctx context.Context, kind string) ([]*ResourceWithStatus, error) {
	var resources []*ResourceWithStatus

	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 扫描配置目录中的所有配置文件
	err := filepath.Walk(c.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过非JSON文件
		if !isJSONFile(info) {
			return nil
		}

		// 处理配置文件
		resource, err := c.processConfigFile(ctx, path, kind)
		if err != nil {
			return nil // 继续处理其他文件
		}

		// 如果返回nil，说明不匹配kind过滤条件
		if resource != nil {
			resources = append(resources, resource)
		}

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
		if err != nil {
			return err
		}

		// 跳过非JSON文件
		if !isJSONFile(info) {
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

		// 找到匹配的资源，处理它
		resource, err := c.processConfigFile(ctx, path, "")
		if err != nil {
			// 处理错误但继续搜索
			return nil
		}

		foundResource = resource
		return filepath.SkipAll // 找到后停止搜索
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

	// 读取现有配置文件以获取当前版本信息
	filePath := filepath.Join(c.configDir, fmt.Sprintf("%s.json", cfg.Metadata.Name))
	existingCfg, err := loadConfigFile(filePath)

	// 处理版本号和生成号
	if err == nil && existingCfg != nil {
		updateMetadata(cfg, existingCfg)
	} else {
		initializeMetadata(cfg)
	}

	// 保存配置文件
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
