package controller

import (
	"testing"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/utils"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestDefaultNodeSelectorEvaluator_hasValidNodeSelector(t *testing.T) {
	tests := []struct {
		name         string
		nodeSelector *systemv1.NodeSelector
		expected     bool
	}{
		{
			name:         "nil nodeSelector should be invalid",
			nodeSelector: nil,
			expected:     false,
		},
		{
			name: "nodeSelector with MachineId should be valid",
			nodeSelector: &systemv1.NodeSelector{
				MachineId: "test-machine-id",
			},
			expected: true,
		},
		{
			name:         "empty nodeSelector should be invalid",
			nodeSelector: &systemv1.NodeSelector{},
			expected:     false,
		},
	}

	logger, err := utils.NewLogger("test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	evaluator := NewNodeSelectorEvaluator(logger).(*DefaultNodeSelectorEvaluator)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.hasValidNodeSelector(tt.nodeSelector)
			if result != tt.expected {
				t.Errorf("hasValidNodeSelector() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestNodeSelectorEvaluator_Basic 基本功能测试
func TestNodeSelectorEvaluator_Basic(t *testing.T) {
	logger, err := utils.NewLogger("test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	evaluator := NewNodeSelectorEvaluator(logger)
	if evaluator == nil {
		t.Error("NewNodeSelectorEvaluator() returned nil")
	}
	
	// 测试空配置列表
	emptyResult, err := evaluator.FilterConfigs(nil, nil)
	if err != nil {
		t.Errorf("FilterConfigs with nil configs should not return error: %v", err)
	}
	if emptyResult != nil {
		t.Error("FilterConfigs with nil configs should return nil")
	}
	
	t.Log("Basic NodeSelectorEvaluator functionality test passed")
}

// TestNodeSelectorEvaluator_NoFallback 测试移除fallback机制后的行为
func TestNodeSelectorEvaluator_NoFallback(t *testing.T) {
	logger, err := utils.NewLogger("test")
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	
	evaluator := NewNodeSelectorEvaluator(logger)
	
	// 创建没有nodeSelector的TimeConfiguration配置
	timeSpecWithoutSelector := &systemv1.TimeConfigurationSpec{
		// 没有NodeSelector字段
		Timezone: "UTC",
	}
	specDataWithoutSelector, _ := anypb.New(timeSpecWithoutSelector)
	configWithoutSelector := &systemv1.ResourceConfig{
		Kind: "TimeConfiguration",
		Metadata: &systemv1.Metadata{
			Name: "config-without-selector",
		},
		Spec: specDataWithoutSelector,
	}
	
	// 创建有无效nodeSelector的TimeConfiguration配置
	timeSpecWithInvalidSelector := &systemv1.TimeConfigurationSpec{
		NodeSelector: &systemv1.NodeSelector{}, // 空的nodeSelector
		Timezone:     "UTC",
	}
	specDataWithInvalidSelector, _ := anypb.New(timeSpecWithInvalidSelector)
	configWithInvalidSelector := &systemv1.ResourceConfig{
		Kind: "TimeConfiguration",
		Metadata: &systemv1.Metadata{
			Name: "config-with-invalid-selector",
		},
		Spec: specDataWithInvalidSelector,
	}
	
	configs := []*systemv1.ResourceConfig{
		configWithoutSelector,
		configWithInvalidSelector,
	}
	
	// 测试FilterConfigs - 应该返回空结果，不再有fallback
	result, err := evaluator.FilterConfigs(nil, configs)
	if err != nil {
		t.Errorf("FilterConfigs should not return error: %v", err)
	}
	
	// 验证没有配置被返回（移除fallback机制）
	if result != nil {
		t.Errorf("Expected no configurations to be returned, got %d", len(result))
	}
	
	t.Log("No fallback mechanism test passed")
}