package centos

import (
	"context"
	"fmt"
	"strings"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// CentOSBondHandler CentOS Bond处理器
type CentOSBondHandler struct {
	osInfo       controller.OSInfo
	managers     []types.IBondManager
}

// NewCentOSBondHandler 创建 CentOS Bond处理器实例
func NewCentOSBondHandler() *CentOSBondHandler {
	return &CentOSBondHandler{}
}

// Match 检查是否匹配 CentOS 7.2-7.9 系统
func (h *CentOSBondHandler) Match(osInfo controller.OSInfo) bool {
	// 检查是否为 CentOS 系统
	if osInfo.ID != "centos" {
		return false
	}

	// 使用专门的版本检查方法
	if h.isCentOS7Supported(osInfo.VersionID) {
		h.osInfo = osInfo
		h.initializeManagers()
		utils.Infof("bond", "CentOS %s bond handler matched", osInfo.VersionID)
		return true
	}

	return false
}

// Reconcile 执行Bond配置调谐，返回结构化的调谐结果
func (h *CentOSBondHandler) Reconcile(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*status.ReconcileResult, error) {
	return h.reconcileConfigs(ctx, configs)
}

// initializeManagers 初始化Bond管理器
func (h *CentOSBondHandler) initializeManagers() {
	// CentOS 7.2-7.9 Bond管理器优先级：
	// 1. CentOS专用NetworkManager（如果可用）
	// 2. CentOS专用传统网络脚本（ifcfg-*）
	h.managers = []types.IBondManager{
		NewCentOSBondNetworkManager(&h.osInfo), // CentOS专用NetworkManager优先
		NewCentOSBondIfupdown(&h.osInfo),       // CentOS专用传统网络脚本作为备选
	}

	utils.Infof("bond", "Initialized CentOS %s bond managers", h.osInfo.VersionID)
}

// reconcileConfigs 调谐Bond配置
func (h *CentOSBondHandler) reconcileConfigs(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*status.ReconcileResult, error) {
	var results []*status.ReconcileResult
	var matchedConfig *systemv1.ResourceConfig
	var fallbackConfig *systemv1.ResourceConfig

	utils.Infof("bond", "Starting CentOS bond reconciliation with %d configurations", len(configs))

	// 遍历所有配置，查找匹配的配置和fallback配置
	for _, config := range configs {
		utils.Debugf("bond", "Processing bond configuration: %s", config.Metadata.Name)
		// 解析配置规格
		bondSpec, err := utils.UnmarshalSpec[*systemv1.BondConfigurationSpec](config.Spec)
		if err != nil {
			utils.Errorf("bond", "Failed to unmarshal bond spec for %s: %v", config.Metadata.Name, err)
			results = append(results, &status.ReconcileResult{
				Config: config,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  status.ReasonSpecError,
					Message: fmt.Sprintf("failed to unmarshal spec: %v", err),
				},
				Error: err,
			})
			continue
		}

		// 检查nodeSelector匹配
		if bondSpec.NodeSelector != nil && h.hasValidNodeSelector(bondSpec.NodeSelector) {
			utils.Debugf("bond", "Checking nodeSelector for bond configuration: %s", config.Metadata.Name)
			// 有有效的nodeSelector，检查是否匹配
			matched, err := utils.MatchNodeSelector(bondSpec.NodeSelector)
			if err != nil {
				utils.Errorf("bond", "Failed to match nodeSelector for %s: %v", config.Metadata.Name, err)
				results = append(results, &status.ReconcileResult{
					Config: config,
					Status: &systemv1.ResourceStatus{
						Phase:   status.PhaseError,
						Reason:  status.ReasonNodeSelectorError,
						Message: fmt.Sprintf("failed to match nodeSelector: %v", err),
					},
					Error: err,
				})
				continue
			}
			if matched {
				utils.Infof("bond", "Bond configuration %s matches nodeSelector", config.Metadata.Name)
				matchedConfig = config
			} else {
				utils.Debugf("bond", "Bond configuration %s does not match nodeSelector", config.Metadata.Name)
				// nodeSelector不匹配时直接跳过，不更新状态
				continue
			}
		} else {
			utils.Debugf("bond", "Bond configuration %s has no valid nodeSelector, considering as fallback", config.Metadata.Name)
			// 没有有效的nodeSelector，作为fallback配置
			if fallbackConfig == nil {
				fallbackConfig = config
			} else {
				// 如果已经有fallback配置，则跳过这个
				utils.Debugf("bond", "Bond configuration %s skipped, another fallback already exists", config.Metadata.Name)
				// 多个fallback配置时直接跳过，不更新状态
				continue
			}
		}
	}

	// 确定要应用的配置
	var configToApply *systemv1.ResourceConfig
	if matchedConfig != nil {
		utils.Infof("bond", "Using matched bond configuration: %s", matchedConfig.Metadata.Name)
		configToApply = matchedConfig
	} else if fallbackConfig != nil {
		utils.Infof("bond", "Using fallback bond configuration: %s", fallbackConfig.Metadata.Name)
		configToApply = fallbackConfig
	} else {
		utils.Warnf("bond", "No applicable bond configuration found")
	}

	// 如果有配置要应用，则应用它
	if configToApply != nil {
		utils.Infof("bond", "Applying bond configuration: %s", configToApply.Metadata.Name)
		bondSpec, err := utils.UnmarshalSpec[*systemv1.BondConfigurationSpec](configToApply.Spec)
		if err != nil {
			utils.Errorf("bond", "Failed to unmarshal effective bond config spec: %v", err)
			results = append(results, &status.ReconcileResult{
				Config: configToApply,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  status.ReasonSpecError,
					Message: fmt.Sprintf("failed to unmarshal effective config spec: %v", err),
				},
				Error: err,
			})
			return results, err
		}
		
		utils.Debugf("bond", "Starting bond configuration application")
		// 应用配置
		result, err := h.applyBondConfiguration(ctx, configToApply, bondSpec)
		if err != nil {
			utils.Errorf("bond", "Failed to apply bond configuration: %v", err)
			results = append(results, &status.ReconcileResult{
					Config: configToApply,
					Status: &systemv1.ResourceStatus{
						Phase:   status.PhaseError,
						Reason:  status.ReasonReconcileError,
						Message: fmt.Sprintf("failed to apply configuration: %v", err),
					},
					Error: err,
				})
			return results, err
		}
		
		utils.Infof("bond", "Bond configuration applied successfully")
		results = append(results, result)
		
		// 其他未应用的配置保持原状态，不进行更新
	}
	// 没有配置可以应用时，保持原状态不变

	utils.Infof("bond", "CentOS bond reconciliation completed")
	return results, nil
}

// applyBondConfiguration 应用Bond配置
func (h *CentOSBondHandler) applyBondConfiguration(ctx context.Context, configToApply *systemv1.ResourceConfig, bondSpec *systemv1.BondConfigurationSpec) (*status.ReconcileResult, error) {
	// 检测并选择合适的Bond管理器
	manager, err := h.detectBondManager(ctx)
	if err != nil {
		return &status.ReconcileResult{
			Config: configToApply,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "NetworkManagerError",
				Message: fmt.Sprintf("failed to detect CentOS bond manager: %v", err),
			},
			Error: err,
		}, err
	}

	// 验证Bond配置
	if err := h.validateCentOSBondConfig(bondSpec); err != nil {
		return &status.ReconcileResult{
			Config: configToApply,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonSpecError,
				Message: fmt.Sprintf("bond validation failed: %v", err),
			},
			Error: err,
		}, err
	}

	// 转换为内部Bond结构
	bondConfig := types.BondConfig{
		Name:    bondSpec.Name,
		Mode:    int(bondSpec.Mode),
		Miimon:  int(bondSpec.Miimon),
		Network: types.BondNetworkConfig{},
		Options: types.BondOptions{},
	}

	// 转换网络配置
	if bondSpec.Network != nil {
		bondConfig.Network = types.BondNetworkConfig{
			IP:         bondSpec.Network.Ip,
			Gateway:    bondSpec.Network.Gateway,
			DNSServers: bondSpec.Network.DnsServers,
			MTU:        int(bondSpec.Network.Mtu),
		}
	}

	// 转换选项配置
	if bondSpec.Options != nil {
		bondConfig.Options = types.BondOptions{
			ExtraOptions: bondSpec.Options.ExtraOptions,
		}
	}

	// 配置Bond接口并检查是否有变更
	hasChanges, err := manager.ConfigureWithCheck(ctx, bondConfig)
	if err != nil {
		return &status.ReconcileResult{
			Config: configToApply,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonReconcileError,
				Message: fmt.Sprintf("failed to configure bond %s: %v", bondSpec.Name, err),
			},
			Error: err,
		}, err
	}

	utils.Infof("bond", "CentOS bond %s configured successfully", bondSpec.Name)

	// 只有在配置发生变更时才重新加载Bond配置
	if hasChanges {
		utils.Infof("bond", "Bond configuration changed, reloading bond services")
		if err := manager.ReloadIfy(ctx); err != nil {
			return &status.ReconcileResult{
				Config: configToApply,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  status.ReasonReconcileError,
					Message: fmt.Sprintf("failed to reload bond configuration: %v", err),
				},
				Error: err,
			}, err
		}
		utils.Infof("bond", "CentOS bond configuration applied and reloaded successfully")
		return &status.ReconcileResult{
			Config: configToApply,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonConfigurationUpdated,
				Message: "CentOS bond configuration applied and reloaded successfully",
			},
		}, nil
	} else {
		utils.Infof("bond", "CentOS bond configuration unchanged, skipping reload")
		return &status.ReconcileResult{
			Config: configToApply,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonAppliedSuccessfully,
				Message: "CentOS bond configuration unchanged",
			},
		}, nil
	}
}

// detectBondManager 检测并选择合适的Bond管理器
func (h *CentOSBondHandler) detectBondManager(ctx context.Context) (types.IBondManager, error) {
	for _, manager := range h.managers {
		if manager.IsInstall(ctx) {
			utils.Infof("bond", "Selected CentOS bond manager: %T on centos %s", manager, h.osInfo.VersionID)
			return manager, nil
		}
	}
	return nil, fmt.Errorf("no suitable CentOS bond manager found")
}

// validateCentOSBondConfig 验证CentOS Bond配置
func (h *CentOSBondHandler) validateCentOSBondConfig(bondSpec *systemv1.BondConfigurationSpec) error {
	if bondSpec == nil {
		return fmt.Errorf("bond configuration spec cannot be nil")
	}

	// 验证Bond名称
	if bondSpec.Name == "" {
		return fmt.Errorf("bond name cannot be empty")
	}

	// 验证Bond名称格式（bond0, bond1等）
	if !strings.HasPrefix(bondSpec.Name, "bond") {
		return fmt.Errorf("bond name must start with 'bond' (e.g., bond0, bond1)")
	}

	// 验证Bond模式
	if bondSpec.Mode < 0 || bondSpec.Mode > 7 {
		return fmt.Errorf("bond mode must be between 0 and 7")
	}

	// 验证miimon值
	if bondSpec.Miimon < 0 || bondSpec.Miimon > 10000 {
		return fmt.Errorf("miimon must be between 0 and 10000")
	}

	return nil
}

// hasValidNodeSelector 检查是否有有效的 nodeSelector
func (h *CentOSBondHandler) hasValidNodeSelector(selector *systemv1.NodeSelector) bool {
	if selector == nil {
		return false
	}
	return selector.MachineId != ""
}

// isCentOS7Supported 检查是否为支持的CentOS 7版本
func (h *CentOSBondHandler) isCentOS7Supported(versionID string) bool {
	if versionID == "" {
		return false
	}

	// 支持的版本：7, 7.2, 7.3, 7.4, 7.5, 7.6, 7.7, 7.8, 7.9
	// 也支持带有构建号的版本，如 7.5.1804
	supportedVersions := []string{"7", "7.1", "7.2", "7.3", "7.4", "7.5", "7.6", "7.7", "7.8", "7.9"}

	for _, supported := range supportedVersions {
		if versionID == supported || strings.HasPrefix(versionID, supported+".") {
			return true
		}
	}

	return false
}