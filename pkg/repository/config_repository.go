package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
	"google.golang.org/protobuf/encoding/protojson"
)

// configRepository 配置文件操作实现
type configRepository struct {
	configDir  string
	logger     *utils.Logger
	controller *controller.Controller // 修复：正确的字段名和类型
}

// NewConfigRepository 创建配置仓库实例
func NewConfigRepository(configDir string, logger *utils.Logger, controller *controller.Controller) ConfigRepository {
	return &configRepository{
		configDir:  configDir,
		logger:     logger,
		controller: controller,
	}
}

// Reconcile 实现调谐方法
func (r *configRepository) Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*domain.ReconcileResult, error) {
	r.logger.Debugf("repository", "Reconciling config: %s (kind: %s)", config.Metadata.Name, config.Kind)

	// 查找对应的处理器
	handler, exists := r.controller.Handlers[config.Kind]
	if !exists {
		r.logger.Warnf("repository", "No handler found for kind: %s", config.Kind)
		return status.ReconcileError(config, status.ReasonNoHandler, fmt.Errorf("No handler found for kind: %s", config.Kind))
	}

	// 执行调谐
	controllerResult, err := handler.Reconcile(ctx, config)
	if err != nil {
		r.logger.Errorf("repository", "Reconciliation error for %s: %v", config.Metadata.Name, err)
		return status.ReconcileError(config, status.ReasonReconcileError, err)
	}

	// 转换结果
	result, _ := status.ReconcileUnknown(config, "NoStatus", "No status returned from handler")

	// 如果有有效配置，使用返回的配置
	if controllerResult.Effective != nil {
		result.Effective = controllerResult.Effective
	}

	// 转换状态
	if controllerResult.Status != nil {
		result.Status = controllerResult.Status
	}

	r.logger.Infof("repository", "Reconciliation completed for %s: %s", config.Metadata.Name, result.Status.Phase)
	return result, nil
}

// List 列出指定类型的配置
func (r *configRepository) List(ctx context.Context, kind string) ([]*systemv1.Resource, error) {
	r.logger.Debugf("repository", "Listing configs of kind: %s", kind)

	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var resources []*systemv1.Resource

	// 扫描配置目录
	err := filepath.Walk(r.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过非JSON文件
		if !utils.IsJSONFile(info) {
			return nil
		}

		// 加载配置文件
		cfg, err := utils.LoadConfigFile(path)
		if err != nil {
			r.logger.Warnf("repository", "Error loading config file %s: %v", path, err)
			return nil // 继续处理其他文件
		}

		// 处理配置文件，获取完整的资源状态
		resource, err := r.processConfigFile(ctx, cfg, kind)
		if err != nil {
			// 处理错误但继续处理其他文件
			r.logger.Warnf("repository", "Error processing config file %s: %v", path, err)
			return nil
		}

		// 如果资源为nil（被kind过滤掉），跳过
		if resource == nil {
			return nil
		}

		resources = append(resources, resource)
		return nil
	})

	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		return nil, fmt.Errorf("failed to walk config directory: %w", err)
	}

	r.logger.Debugf("repository", "Found %d configs of kind: %s", len(resources), kind)
	return resources, nil
}

// Get 获取指定名称的配置
func (r *configRepository) Get(ctx context.Context, name string) (*systemv1.Resource, error) {
	r.logger.Debugf("repository", "Getting config: %s", name)

	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 扫描配置目录中的所有配置文件
	var foundResource *systemv1.Resource

	err := filepath.Walk(r.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过非JSON文件
		if !utils.IsJSONFile(info) {
			return nil
		}

		// 加载配置文件
		cfg, err := utils.LoadConfigFile(path)
		if err != nil {
			r.logger.Warnf("repository", "Error loading config file %s: %v", path, err)
			return nil // 继续搜索其他文件
		}

		// 检查名称是否匹配
		if cfg.Metadata.Name != name {
			return nil
		}

		// 找到匹配的资源，处理它
		resource, err := r.processConfigFile(ctx, cfg, "")
		if err != nil {
			// 处理错误但继续搜索
			r.logger.Warnf("repository", "Error processing config file %s: %v", path, err)
			return nil
		}

		foundResource = resource
		return filepath.SkipAll // 找到后停止搜索
	})

	if err != nil && err != filepath.SkipAll && err != context.Canceled && err != context.DeadlineExceeded {
		return nil, fmt.Errorf("error searching for resource: %v", err)
	}

	if foundResource == nil {
		r.logger.Warnf("repository", "Config not found: %s", name)
		return nil, fmt.Errorf("config not found: %s", name)
	}

	// // 以 JSON 格式打印 rws 的内容
	// if rwsJSON, err := json.MarshalIndent(foundResource, "", "  "); err == nil {
	// 	r.logger.Debugf("repository", "Created ResourceWithStatus JSON: %s", string(rwsJSON))
	// } else {
	// 	r.logger.Debugf("repository", "Created ResourceWithStatus: %+v", foundResource)
	// }

	return foundResource, nil
}

func (r *configRepository) processConfigFile(ctx context.Context, cfg *systemv1.ResourceConfig, kind string) (*systemv1.Resource, error) {
	// 检查上下文是否已取消
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	// 如果指定了kind，则过滤
	if kind != "" && cfg.Kind != kind {
		return nil, nil
	}

	// 执行调谐以获取最新状态
	result, err := r.Reconcile(ctx, cfg)
	if err != nil {
		reconcileResult, _ := status.ReconcileError(cfg, status.ReasonReconcileError, fmt.Errorf("Reconciliation error: %v", err))
		return &systemv1.Resource{
			Config: cfg,
			Status: reconcileResult.Status,
		}, nil
	}

	// 返回资源状态
	return &systemv1.Resource{
		Config:          cfg,
		EffectiveConfig: result.Effective,
		Status:          result.Status,
	}, nil
}

// Save 保存配置到文件
func (r *configRepository) Save(ctx context.Context, config *systemv1.ResourceConfig) error {
	r.logger.Debugf("repository", "Saving config: %s", config.Metadata.Name)

	// 更新元数据
	config.Metadata.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
	config.Metadata.Generation++
	if config.Metadata.CreationTime == "" {
		config.Metadata.CreationTime = time.Now().Format(time.RFC3339)
	}

	// 使用 protojson 序列化配置，保持与 API 响应一致的格式
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		Indent:          "  ",
	}
	data, err := marshaler.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 确保配置目录存在
	if err := os.MkdirAll(r.configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 构建配置文件路径
	configPath := filepath.Join(r.configDir, config.Metadata.Name+".json")

	// 原子性写入文件
	if err := utils.AtomicWriteFile(data, configPath, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	r.logger.Infof("repository", "Saved config: %s", config.Metadata.Name)
	return nil
}
