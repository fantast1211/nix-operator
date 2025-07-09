package repository

import (
	"context"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
)

// ConfigRepository 配置文件操作接口
type ConfigRepository interface {
	// ListConfigs 列出指定类型的配置
	ListConfigs(ctx context.Context, kind string) ([]*systemv1.ResourceConfig, error)
	// GetConfig 获取指定名称的配置
	GetConfig(ctx context.Context, name string) (*systemv1.ResourceConfig, error)
	// SaveConfig 保存配置到文件
	SaveConfig(ctx context.Context, config *systemv1.ResourceConfig) error
}
