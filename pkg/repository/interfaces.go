package repository

import (
	"context"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/domain"
)

// ConfigRepository 配置文件操作接口
type ConfigRepository interface {
	// List 列出指定类型的配置
	List(ctx context.Context, kind string) ([]*systemv1.Resource, error)
	// Get 获取指定名称的配置
	Get(ctx context.Context, name string) (*systemv1.Resource, error)
	// Save 保存配置到文件
	Save(ctx context.Context, config *systemv1.ResourceConfig) error
	// Reconcile 调谐实现
	Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*domain.ReconcileResult, error)
}
