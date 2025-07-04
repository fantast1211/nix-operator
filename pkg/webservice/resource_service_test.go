package webservice

import (
	"context"
	"encoding/json"
	"testing"

	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/utils"
	"go.xbrother.com/nix-operator/pkg/validator"
)

// mockConfigRepository 模拟配置仓储
type mockConfigRepository struct {
	resources map[string]*domain.ResourceWithStatus
	errors    map[string]error
}

func newMockConfigRepository() *mockConfigRepository {
	return &mockConfigRepository{
		resources: make(map[string]*domain.ResourceWithStatus),
		errors:    make(map[string]error),
	}
}

func (m *mockConfigRepository) List(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error) {
	if err, exists := m.errors["List"]; exists {
		return nil, err
	}

	var result []*domain.ResourceWithStatus
	for _, resource := range m.resources {
		if kind == "" || resource.Resource.Kind == kind {
			result = append(result, resource)
		}
	}
	return result, nil
}

func (m *mockConfigRepository) Get(ctx context.Context, name string) (*domain.ResourceWithStatus, error) {
	if err, exists := m.errors["Get"]; exists {
		return nil, err
	}

	if resource, exists := m.resources[name]; exists {
		return resource, nil
	}
	return nil, nil
}

func (m *mockConfigRepository) Save(ctx context.Context, config *domain.ResourceConfig) error {
	if err, exists := m.errors["Save"]; exists {
		return err
	}

	m.resources[config.Metadata.Name] = &domain.ResourceWithStatus{
		Resource: &domain.Resource{
			APIVersion: config.APIVersion,
			Kind:       config.Kind,
			Metadata:   config.Metadata,
			Spec:       config.Spec,
		},
		Status: &domain.ResourceStatus{Phase: "Ready"},
	}
	return nil
}

func (m *mockConfigRepository) Delete(ctx context.Context, name string) error {
	if err, exists := m.errors["Delete"]; exists {
		return err
	}

	delete(m.resources, name)
	return nil
}

// mockReconcileRepository 模拟调谐仓储
type mockReconcileRepository struct {
	errors map[string]error
}

func newMockReconcileRepository() *mockReconcileRepository {
	return &mockReconcileRepository{
		errors: make(map[string]error),
	}
}

func (m *mockReconcileRepository) Reconcile(ctx context.Context, cfg *domain.ResourceConfig) (*domain.ReconcileResult, error) {
	if err, exists := m.errors["Reconcile"]; exists {
		return nil, err
	}

	return &domain.ReconcileResult{
		Effective: cfg,
		Status:    &domain.ResourceStatus{Phase: "Ready"},
	}, nil
}

func (m *mockReconcileRepository) ReconcileAll(ctx context.Context) error {
	if err, exists := m.errors["ReconcileAll"]; exists {
		return err
	}
	return nil
}

func (m *mockReconcileRepository) ListResourceConfigs(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error) {
	if err, exists := m.errors["ListResourceConfigs"]; exists {
		return nil, err
	}
	return []*domain.ResourceWithStatus{}, nil
}

// TestResourceService_ListResources 测试列出资源
func TestResourceService_ListResources(t *testing.T) {
	// 准备测试数据
	configRepo := newMockConfigRepository()
	reconcileRepo := newMockReconcileRepository()
	validatorRegistry := validator.NewValidatorRegistry()
	logger, _ := utils.NewLogger("test")

	service := NewResourceService(configRepo, reconcileRepo, validatorRegistry, logger)

	// 添加测试资源
	testResource := &domain.ResourceWithStatus{
		Resource: &domain.Resource{
			APIVersion: "v1",
			Kind:       "HostsConfiguration",
			Metadata:   domain.Metadata{Name: "test-hosts"},
			Spec:       json.RawMessage(`{"hosts":[{"ip":"127.0.0.1","hostnames":["localhost"]}]}`),
		},
		Status: &domain.ResourceStatus{Phase: "Ready"},
	}
	configRepo.resources["test-hosts"] = testResource

	// 执行测试
	ctx := context.Background()
	resources, err := service.ListResources(ctx, "")

	// 验证结果
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(resources))
	}

	if resources[0].Resource.Metadata.Name != "test-hosts" {
		t.Errorf("Expected resource name 'test-hosts', got '%s'", resources[0].Resource.Metadata.Name)
	}
}

// TestResourceService_GetResource 测试获取单个资源
func TestResourceService_GetResource(t *testing.T) {
	// 准备测试数据
	configRepo := newMockConfigRepository()
	reconcileRepo := newMockReconcileRepository()
	validatorRegistry := validator.NewValidatorRegistry()
	logger, _ := utils.NewLogger("test")

	service := NewResourceService(configRepo, reconcileRepo, validatorRegistry, logger)

	// 添加测试资源
	testResource := &domain.ResourceWithStatus{
		Resource: &domain.Resource{
			APIVersion: "v1",
			Kind:       "HostsConfiguration",
			Metadata:   domain.Metadata{Name: "test-hosts"},
			Spec:       json.RawMessage(`{"hosts":[{"ip":"127.0.0.1","hostnames":["localhost"]}]}`),
		},
		Status: &domain.ResourceStatus{Phase: "Ready"},
	}
	configRepo.resources["test-hosts"] = testResource

	// 执行测试
	ctx := context.Background()
	resource, err := service.GetResource(ctx, "test-hosts")

	// 验证结果
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if resource == nil {
		t.Error("Expected resource, got nil")
		return
	}

	if resource.Resource.Metadata.Name != "test-hosts" {
		t.Errorf("Expected resource name 'test-hosts', got '%s'", resource.Resource.Metadata.Name)
	}
}

// TestResourceService_UpdateResource 测试更新资源
func TestResourceService_UpdateResource(t *testing.T) {
	// 准备测试数据
	configRepo := newMockConfigRepository()
	reconcileRepo := newMockReconcileRepository()
	validatorRegistry := validator.NewValidatorRegistry()
	
	// 注册校验器
	validator.RegisterDefaultValidators(validatorRegistry)
	
	logger, _ := utils.NewLogger("test")

	service := NewResourceService(configRepo, reconcileRepo, validatorRegistry, logger)

	// 准备测试资源
	testResource := &domain.Resource{
		APIVersion: "v1",
		Kind:       "HostsConfiguration",
		Metadata:   domain.Metadata{Name: "test-hosts"},
		Spec:       json.RawMessage(`{"hosts":[{"ip":"127.0.0.1","hostnames":["localhost"]}]}`),
	}

	// 执行测试
	ctx := context.Background()
	updatedResource, err := service.UpdateResource(ctx, testResource)

	// 验证结果
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if updatedResource == nil {
		t.Error("Expected updated resource, got nil")
		return
	}

	if updatedResource.Metadata.Name != "test-hosts" {
		t.Errorf("Expected resource name 'test-hosts', got '%s'", updatedResource.Metadata.Name)
	}

	// 验证资源已保存
	if _, exists := configRepo.resources["test-hosts"]; !exists {
		t.Error("Expected resource to be saved in repository")
	}
}

// TestResourceService_DeleteResource 测试删除资源
func TestResourceService_DeleteResource(t *testing.T) {
	// 准备测试数据
	configRepo := newMockConfigRepository()
	reconcileRepo := newMockReconcileRepository()
	validatorRegistry := validator.NewValidatorRegistry()
	logger, _ := utils.NewLogger("test")

	service := NewResourceService(configRepo, reconcileRepo, validatorRegistry, logger)

	// 添加测试资源
	testResource := &domain.ResourceWithStatus{
		Resource: &domain.Resource{
			APIVersion: "v1",
			Kind:       "HostsConfiguration",
			Metadata:   domain.Metadata{Name: "test-hosts"},
			Spec:       json.RawMessage(`{"hosts":[{"ip":"127.0.0.1","hostnames":["localhost"]}]}`),
		},
		Status: &domain.ResourceStatus{Phase: "Ready"},
	}
	configRepo.resources["test-hosts"] = testResource

	// 执行测试
	ctx := context.Background()
	err := service.DeleteResource(ctx, "test-hosts")

	// 验证结果
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 验证资源已删除
	if _, exists := configRepo.resources["test-hosts"]; exists {
		t.Error("Expected resource to be deleted from repository")
	}
}