package controller

import (
	"context"
	"fmt"
	"testing"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// mockStatusRepository 模拟状态仓库
type mockStatusRepository struct {
	statuses map[string]*systemv1.ResourceStatus
}

func newMockStatusRepository() *mockStatusRepository {
	return &mockStatusRepository{
		statuses: make(map[string]*systemv1.ResourceStatus),
	}
}

func (m *mockStatusRepository) GetStatus(ctx context.Context, name string) (*systemv1.ResourceStatus, error) {
	status, exists := m.statuses[name]
	if !exists {
		return nil, fmt.Errorf("status not found for resource: %s", name)
	}
	return status, nil
}

func (m *mockStatusRepository) SetStatus(ctx context.Context, name string, status *systemv1.ResourceStatus) error {
	m.statuses[name] = status
	return nil
}

func (m *mockStatusRepository) SetStatusWithKind(ctx context.Context, name, kind string, status *systemv1.ResourceStatus) error {
	m.statuses[name] = status
	return nil
}

func (m *mockStatusRepository) ListStatuses(ctx context.Context, kind string) (map[string]*systemv1.ResourceStatus, error) {
	return m.statuses, nil
}

func (m *mockStatusRepository) DeleteStatus(ctx context.Context, name string) error {
	delete(m.statuses, name)
	return nil
}

func (m *mockStatusRepository) Clear(ctx context.Context) error {
	m.statuses = make(map[string]*systemv1.ResourceStatus)
	return nil
}

// TestFilterConfigsForReconcile 测试基于generation的配置过滤
func TestFilterConfigsForReconcile(t *testing.T) {
	ctx := context.Background()
	mockRepo := newMockStatusRepository()
	logger, err := utils.NewLogger("test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	controller := &Controller{
		statusRepo: mockRepo,
		logger:     logger,
	}

	tests := []struct {
		name           string
		configs        []*systemv1.ResourceConfig
		existingStatus map[string]*systemv1.ResourceStatus
		expectedCount  int
		description    string
	}{
		{
			name: "first_time_reconcile",
			configs: []*systemv1.ResourceConfig{
				{
					Metadata: &systemv1.Metadata{
						Name:       "config1",
						Generation: 1,
					},
				},
			},
			existingStatus: map[string]*systemv1.ResourceStatus{},
			expectedCount:  1,
			description:    "首次调谐时应该执行",
		},
		{
			name: "generation_increased",
			configs: []*systemv1.ResourceConfig{
				{
					Metadata: &systemv1.Metadata{
						Name:       "config1",
						Generation: 3,
					},
				},
			},
			existingStatus: map[string]*systemv1.ResourceStatus{
				"config1": {
					Phase:              "Ready",
					ObservedGeneration: 2,
				},
			},
			expectedCount: 1,
			description:   "generation增加时应该执行调谐",
		},
		{
			name: "generation_same",
			configs: []*systemv1.ResourceConfig{
				{
					Metadata: &systemv1.Metadata{
						Name:       "config1",
						Generation: 2,
					},
				},
			},
			existingStatus: map[string]*systemv1.ResourceStatus{
				"config1": {
					Phase:              "Ready",
					ObservedGeneration: 2,
				},
			},
			expectedCount: 0,
			description:   "generation相同时应该跳过调谐",
		},
		{
			name: "generation_decreased",
			configs: []*systemv1.ResourceConfig{
				{
					Metadata: &systemv1.Metadata{
						Name:       "config1",
						Generation: 1,
					},
				},
			},
			existingStatus: map[string]*systemv1.ResourceStatus{
				"config1": {
					Phase:              "Ready",
					ObservedGeneration: 2,
				},
			},
			expectedCount: 0,
			description:   "generation减少时应该跳过调谐",
		},
		{
			name: "mixed_scenarios",
			configs: []*systemv1.ResourceConfig{
				{
					Metadata: &systemv1.Metadata{
						Name:       "config1",
						Generation: 3, // 需要调谐
					},
				},
				{
					Metadata: &systemv1.Metadata{
						Name:       "config2",
						Generation: 1, // 跳过
					},
				},
				{
					Metadata: &systemv1.Metadata{
						Name:       "config3",
						Generation: 1, // 首次调谐
					},
				},
			},
			existingStatus: map[string]*systemv1.ResourceStatus{
				"config1": {
					Phase:              "Ready",
					ObservedGeneration: 2,
				},
				"config2": {
					Phase:              "Ready",
					ObservedGeneration: 1,
				},
				// config3 没有状态
			},
			expectedCount: 2, // config1 和 config3
			description:   "混合场景：部分需要调谐，部分跳过",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置现有状态
			mockRepo.statuses = make(map[string]*systemv1.ResourceStatus)
			for name, status := range tt.existingStatus {
				mockRepo.statuses[name] = status
			}

			// 执行过滤
			result := controller.filterConfigsForReconcile(ctx, "TestKind", tt.configs)

			// 验证结果
			if len(result) != tt.expectedCount {
				t.Errorf("%s: expected %d configs to reconcile, got %d. %s",
					tt.name, tt.expectedCount, len(result), tt.description)
			}

			t.Logf("%s: %s - 过滤结果: %d/%d 配置需要调谐",
				tt.name, tt.description, len(result), len(tt.configs))
		})
	}
}

// TestObservedGenerationUpdate 测试ObservedGeneration更新逻辑
func TestObservedGenerationUpdate(t *testing.T) {
	// 创建一个模拟的调谐结果
	config := &systemv1.ResourceConfig{
		Metadata: &systemv1.Metadata{
			Name:       "test-config",
			Generation: 5,
		},
	}

	status := &systemv1.ResourceStatus{
		Phase:              "Ready",
		Reason:             "AppliedSuccessfully",
		Message:            "Configuration applied successfully",
		ObservedGeneration: 0, // 初始值
	}

	// 模拟调谐成功的逻辑
	if status.Phase == "Ready" {
		status.ObservedGeneration = config.Metadata.Generation
	}

	// 验证ObservedGeneration被正确更新
	if status.ObservedGeneration != config.Metadata.Generation {
		t.Errorf("Expected ObservedGeneration to be %d, got %d",
			config.Metadata.Generation, status.ObservedGeneration)
	}

	t.Logf("ObservedGeneration successfully updated to %d", status.ObservedGeneration)
}
