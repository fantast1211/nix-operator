package webrepository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/interfaces"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// configRepository 配置文件仓储实现
type configRepository struct {
	configDir          string
	reconcileRepo      interfaces.ReconcileRepository
	logger             *utils.Logger
}

// NewConfigRepository 创建配置仓储
func NewConfigRepository(configDir string, reconcileRepo interfaces.ReconcileRepository, logger *utils.Logger) interfaces.ConfigRepository {
	return &configRepository{
		configDir:     configDir,
		reconcileRepo: reconcileRepo,
		logger:        logger,
	}
}

// List 查询操作 - 通过调谐获取实时状态
func (r *configRepository) List(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error) {
	r.logger.Debugf("config_repository", "Listing resources of kind: %s", kind)
	
	// 调用调谐仓储获取实时状态
	return r.reconcileRepo.ListResourceConfigs(ctx, kind)
}

// Get 获取单个配置 - 通过调谐获取实时状态
func (r *configRepository) Get(ctx context.Context, name string) (*domain.ResourceWithStatus, error) {
	r.logger.Debugf("config_repository", "Getting resource: %s", name)
	
	// 首先加载配置文件
	configPath := r.getConfigPath(name)
	cfg, err := r.loadConfigFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file %s: %v", configPath, err)
	}

	// 通过调谐获取实时状态
	result, err := r.reconcileRepo.Reconcile(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile config %s: %v", name, err)
	}

	return &domain.ResourceWithStatus{
		Resource: &domain.Resource{
			APIVersion: cfg.APIVersion,
			Kind:       cfg.Kind,
			Metadata:   cfg.Metadata,
			Spec:       cfg.Spec,
		},
		Status: result.Status,
	}, nil
}

// Save 修改操作 - 直接操作配置文件
func (r *configRepository) Save(ctx context.Context, config *domain.ResourceConfig) error {
	r.logger.Debugf("config_repository", "Saving resource config: %s", config.Metadata.Name)
	
	// 设置元数据
	if config.Metadata.CreationTime == nil {
		now := time.Now()
		config.Metadata.CreationTime = &now
	}
	config.Metadata.Generation++
	
	// 序列化配置
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// 确保配置目录存在
	configPath := r.getConfigPath(config.Metadata.Name)
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// 原子性写入文件
	if err := utils.AtomicWriteFile(data, configPath, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	r.logger.Infof("config_repository", "Config saved successfully: %s", configPath)
	return nil
}

// Delete 删除配置文件
func (r *configRepository) Delete(ctx context.Context, name string) error {
	r.logger.Debugf("config_repository", "Deleting resource config: %s", name)
	
	configPath := r.getConfigPath(name)
	if err := os.Remove(configPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config file not found: %s", name)
		}
		return fmt.Errorf("failed to delete config file: %v", err)
	}

	r.logger.Infof("config_repository", "Config deleted successfully: %s", configPath)
	return nil
}

// getConfigPath 获取配置文件路径
func (r *configRepository) getConfigPath(name string) string {
	// 确保文件名以.json结尾
	filename := name
	if !strings.HasSuffix(filename, ".json") {
		filename += ".json"
	}
	return filepath.Join(r.configDir, filename)
}

// loadConfigFile 加载配置文件
func (r *configRepository) loadConfigFile(path string) (*domain.ResourceConfig, error) {
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