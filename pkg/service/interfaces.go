package service

import (
	"context"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
)

// ResourceService 资源服务接口
type ResourceService interface {
	// ListResources 列出指定类型的资源
	ListResources(ctx context.Context, kind string) ([]*systemv1.Resource, error)
	// GetResource 获取指定名称的资源
	GetResource(ctx context.Context, name string) (*systemv1.Resource, error)
	// UpdateResource 更新资源
	UpdateResource(ctx context.Context, resource *systemv1.ResourceConfig) (*systemv1.Resource, error)
}
