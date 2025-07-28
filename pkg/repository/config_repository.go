package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/utils"
	"google.golang.org/protobuf/encoding/protojson"
)

// configRepository 配置文件操作实现
type configRepository struct {
	configDir string
	logger    *utils.Logger
}

// NewConfigRepository 创建配置仓库实例
func NewConfigRepository(configDir string, logger *utils.Logger) ConfigRepository {
	return &configRepository{
		configDir: configDir,
		logger:    logger,
	}
}

// ListConfigs 列出指定类型的配置
func (r *configRepository) ListConfigs(ctx context.Context, kind string) ([]*systemv1.ResourceConfig, error) {
	r.logger.Debugf("repository", "Listing configs of kind: %s", kind)

	var configs []*systemv1.ResourceConfig

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

		// 如果指定了kind，则过滤
		if kind != "" && cfg.Kind != kind {
			return nil
		}

		configs = append(configs, cfg)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk config directory: %w", err)
	}

	r.logger.Debugf("repository", "Found %d configs of kind: %s", len(configs), kind)
	return configs, nil
}

// GetConfig 获取指定名称的配置
func (r *configRepository) GetConfig(ctx context.Context, name string) (*systemv1.ResourceConfig, error) {
	r.logger.Debugf("repository", "Getting config: %s", name)

	// 扫描配置目录中的所有配置文件
	var foundConfig *systemv1.ResourceConfig

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

		foundConfig = cfg
		return filepath.SkipAll // 找到后停止搜索
	})

	if err != nil && err != filepath.SkipAll {
		return nil, fmt.Errorf("error searching for config: %v", err)
	}

	if foundConfig == nil {
		r.logger.Warnf("repository", "Config not found: %s", name)
		return nil, fmt.Errorf("config not found: %s", name)
	}

	return foundConfig, nil
}

// SaveConfig 保存配置到文件
func (r *configRepository) SaveConfig(ctx context.Context, config *systemv1.ResourceConfig) error {
	r.logger.Debugf("repository", "Saving config: %s", config.Metadata.Name)

	// 获取现有配置以保留元数据
	existingConfig, err := r.GetConfig(ctx, config.Metadata.Name)
	var currentGeneration int32 = 0
	var creationTime string

	if err == nil && existingConfig != nil && existingConfig.Metadata != nil {
		currentGeneration = existingConfig.Metadata.Generation
		creationTime = existingConfig.Metadata.CreationTime
	}

	// 更新元数据
	config.Metadata.ResourceVersion = fmt.Sprintf("%d", time.Now().Unix())
	// 每次PUT请求都将Generation加1，实现手动重试
	config.Metadata.Generation = currentGeneration + 1
	r.logger.Debugf("repository", "Incrementing Generation for config %s from %d to %d", config.Metadata.Name, currentGeneration, currentGeneration+1)

	if creationTime != "" {
		config.Metadata.CreationTime = creationTime // 保留原创建时间
	} else if config.Metadata.CreationTime == "" {
		config.Metadata.CreationTime = time.Now().Format(time.RFC3339)
	}

	// 初始化Annotations map（防止为nil）
	if config.Metadata.Annotations == nil {
		config.Metadata.Annotations = make(map[string]string)
	}

	// 设置或更新 syncTriggerRandom 字段, 遇到了Syncthing不同步的问题，只能暂时这样解决
	config.Metadata.Annotations["syncTriggerRandom"] = utils.RandomString()

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

	r.logger.Infof("repository", "Saved config: %s, Generation: %d", config.Metadata.Name, config.Metadata.Generation)
	return nil
}
