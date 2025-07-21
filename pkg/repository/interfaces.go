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

// StatusRepository 状态缓存操作接口
type StatusRepository interface {
	// GetStatus 获取资源状态
	GetStatus(ctx context.Context, name string) (*systemv1.ResourceStatus, error)
	// SetStatus 设置资源状态
	SetStatus(ctx context.Context, name string, status *systemv1.ResourceStatus) error
	// SetStatusWithKind 设置资源状态并记录kind信息
	SetStatusWithKind(ctx context.Context, name, kind string, status *systemv1.ResourceStatus) error
	// ListStatuses 列出指定类型的所有状态
	ListStatuses(ctx context.Context, kind string) (map[string]*systemv1.ResourceStatus, error)
	// DeleteStatus 删除资源状态
	DeleteStatus(ctx context.Context, name string) error
	// Clear 清空所有状态缓存
	Clear(ctx context.Context) error
}
