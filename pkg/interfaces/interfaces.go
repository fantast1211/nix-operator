package interfaces

import (
	"context"

	"go.xbrother.com/nix-operator/pkg/domain"
)

// ResourceController Web控制器接口
type ResourceController interface {
	// ListResourceConfigs 列出资源配置
	ListResourceConfigs(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error)
	// GetResourceConfig 获取单个资源配置
	GetResourceConfig(ctx context.Context, name string) (*domain.ResourceWithStatus, error)
	// UpdateResourceConfig 更新资源配置
	UpdateResourceConfig(ctx context.Context, resource *domain.Resource) (*domain.Resource, error)
	// DeleteResourceConfig 删除资源配置
	DeleteResourceConfig(ctx context.Context, name string) error
}

// ResourceService 业务服务接口
type ResourceService interface {
	// ListResources 列出资源
	ListResources(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error)
	// GetResource 获取单个资源
	GetResource(ctx context.Context, name string) (*domain.ResourceWithStatus, error)
	// UpdateResource 更新资源
	UpdateResource(ctx context.Context, resource *domain.Resource) (*domain.Resource, error)
	// DeleteResource 删除资源
	DeleteResource(ctx context.Context, name string) error
}

// ConfigRepository 配置文件操作接口
type ConfigRepository interface {
	// List 查询操作 - 通过调谐获取实时状态
	List(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error)
	// Get 获取单个配置 - 通过调谐获取实时状态
	Get(ctx context.Context, name string) (*domain.ResourceWithStatus, error)
	// Save 修改操作 - 直接操作配置文件
	Save(ctx context.Context, config *domain.ResourceConfig) error
	// Delete 删除配置文件
	Delete(ctx context.Context, name string) error
}

// ReconcileRepository 调谐操作接口
type ReconcileRepository interface {
	// Reconcile 执行调谐获取实时状态（供查询操作使用）
	Reconcile(ctx context.Context, cfg *domain.ResourceConfig) (*domain.ReconcileResult, error)
	// ReconcileAll 批量调谐（供文件监控使用）
	ReconcileAll(ctx context.Context) error
	// ListResourceConfigs 列出所有配置并执行调谐
	ListResourceConfigs(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error)
}

// SpecValidator 类型校验器接口
type SpecValidator interface {
	// Validate 校验spec内容
	Validate(spec []byte) error
	// GetKind 获取支持的Kind
	GetKind() string
}

// ConfigHandler 配置处理器接口
type ConfigHandler interface {
	// Reconcile 执行调谐
	Reconcile(ctx context.Context, config *domain.ResourceConfig) (*domain.ReconcileResult, error)
	// Match 检查是否支持该操作系统
	Match(osInfo domain.OSInfo) bool
	// GetKind 获取处理的配置类型
	GetKind() string
}