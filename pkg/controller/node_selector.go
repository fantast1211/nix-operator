package controller

import (
	"context"
	"fmt"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// NodeSelectorEvaluator 节点选择器评估器接口
type NodeSelectorEvaluator interface {
	// EvaluateConfig 评估单个配置是否匹配当前节点
	EvaluateConfig(ctx context.Context, config *systemv1.ResourceConfig) (*NodeSelectorResult, error)
	// FilterConfigs 过滤配置列表，返回匹配的配置
	FilterConfigs(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*systemv1.ResourceConfig, error)
}

// NodeSelectorResult 节点选择器评估结果
type NodeSelectorResult struct {
	// Matched 是否匹配
	Matched bool
	// Reason 匹配或不匹配的原因
	Reason string
	// Config 原始配置
	Config *systemv1.ResourceConfig
}

// DefaultNodeSelectorEvaluator 默认节点选择器评估器实现
type DefaultNodeSelectorEvaluator struct {
	logger *utils.Logger
}

// NewNodeSelectorEvaluator 创建新的节点选择器评估器
func NewNodeSelectorEvaluator(logger *utils.Logger) NodeSelectorEvaluator {
	return &DefaultNodeSelectorEvaluator{
		logger: logger,
	}
}

// EvaluateConfig 评估单个配置是否匹配当前节点
func (e *DefaultNodeSelectorEvaluator) EvaluateConfig(ctx context.Context, config *systemv1.ResourceConfig) (*NodeSelectorResult, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// 提取NodeSelector
	nodeSelector, err := e.extractNodeSelector(config)
	if err != nil {
		return &NodeSelectorResult{
			Matched: false,
			Reason:  fmt.Sprintf("failed to extract nodeSelector: %v", err),
			Config:  config,
		}, err
	}

	// 如果没有NodeSelector或NodeSelector无效，则跳过该配置
	if nodeSelector == nil || !e.hasValidNodeSelector(nodeSelector) {
		e.logger.Debugf("node-selector", "Config %s has no valid nodeSelector, skipping", config.Metadata.Name)
		return &NodeSelectorResult{
			Matched: false,
			Reason:  "no valid nodeSelector",
			Config:  config,
		}, nil
	}

	// 执行节点匹配
	matched, err := utils.MatchNodeSelector(nodeSelector)
	if err != nil {
		e.logger.Errorf("node-selector", "Failed to match nodeSelector for config %s: %v", config.Metadata.Name, err)
		return &NodeSelectorResult{
			Matched: false,
			Reason:  fmt.Sprintf("nodeSelector matching error: %v", err),
			Config:  config,
		}, err
	}

	if matched {
		e.logger.Infof("node-selector", "Config %s matched nodeSelector", config.Metadata.Name)
		return &NodeSelectorResult{
			Matched: true,
			Reason:  "nodeSelector matched",
			Config:  config,
		}, nil
	} else {
		e.logger.Debugf("node-selector", "Config %s did not match nodeSelector", config.Metadata.Name)
		return &NodeSelectorResult{
			Matched: false,
			Reason:  "nodeSelector did not match",
			Config:  config,
		}, nil
	}
}

// FilterConfigs 过滤配置列表，返回匹配的配置
// 只返回第一个匹配nodeSelector的配置，确保每种类型只有一个配置被处理
func (e *DefaultNodeSelectorEvaluator) FilterConfigs(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*systemv1.ResourceConfig, error) {
	if len(configs) == 0 {
		return nil, nil
	}

	// 遍历所有配置，查找第一个匹配的配置
	for _, config := range configs {
		result, err := e.EvaluateConfig(ctx, config)
		if err != nil {
			e.logger.Errorf("node-selector", "Failed to evaluate config %s: %v", config.Metadata.Name, err)
			continue
		}

		if result.Matched {
			// 找到第一个匹配的配置，直接返回
			e.logger.Infof("node-selector", "Config %s matched nodeSelector, using this configuration", config.Metadata.Name)
			return []*systemv1.ResourceConfig{config}, nil
		} else {
			// 不匹配的配置直接跳过
			e.logger.Debugf("node-selector", "Config %s skipped: %s", config.Metadata.Name, result.Reason)
		}
	}

	// 没有找到匹配的配置
	e.logger.Infof("node-selector", "No matching configurations found")
	return nil, nil
}

// extractNodeSelector 从配置中提取NodeSelector
func (e *DefaultNodeSelectorEvaluator) extractNodeSelector(config *systemv1.ResourceConfig) (*systemv1.NodeSelector, error) {
	switch config.Kind {
	case "HostsConfiguration":
		hostsSpec, err := utils.UnmarshalSpec[*systemv1.HostsConfigurationSpec](config.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal HostsConfigurationSpec: %v", err)
		}
		return hostsSpec.NodeSelector, nil

	case "NetworkConfiguration":
		networkSpec, err := utils.UnmarshalSpec[*systemv1.NetworkConfigurationSpec](config.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal NetworkConfigurationSpec: %v", err)
		}
		return networkSpec.NodeSelector, nil

	case "BondConfiguration":
		bondSpec, err := utils.UnmarshalSpec[*systemv1.BondConfigurationSpec](config.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal BondConfigurationSpec: %v", err)
		}
		return bondSpec.NodeSelector, nil

	case "TimeConfiguration":
		timeSpec, err := utils.UnmarshalSpec[*systemv1.TimeConfigurationSpec](config.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal TimeConfigurationSpec: %v", err)
		}
		return timeSpec.NodeSelector, nil

	default:
		return nil, fmt.Errorf("unsupported config kind: %s", config.Kind)
	}
}

// hasValidNodeSelector 检查是否有有效的nodeSelector
func (e *DefaultNodeSelectorEvaluator) hasValidNodeSelector(selector *systemv1.NodeSelector) bool {
	if selector == nil {
		return false
	}
	return selector.MachineId != ""
}