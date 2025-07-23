package openeuler

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"text/template"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// OpenEulerBondHandler openEuler系统专用的Bond处理器
type OpenEulerBondHandler struct {
	osInfo controller.OSInfo
}

// NewOpenEulerBondHandler 创建openEuler Bond处理器实例
func NewOpenEulerBondHandler() *OpenEulerBondHandler {
	return &OpenEulerBondHandler{}
}

// 初始化模板函数
func init() {
	// 注册模板函数
	template.Must(template.New("").Funcs(template.FuncMap{
		"splitCIDR": func(cidr string) []string {
			if cidr == "" {
				return []string{"", ""}
			}
			parts := strings.Split(cidr, "/")
			if len(parts) != 2 {
				return []string{cidr, ""}
			}
			return parts
		},
		"cidrToNetmask": func(prefixLen string) string {
			if prefixLen == "" {
				return ""
			}
			len, err := strconv.Atoi(prefixLen)
			if err != nil {
				return ""
			}
			mask := net.CIDRMask(len, 32)
			return net.IP(mask).String()
		},
		"add": func(a, b int) int {
			return a + b
		},
	}).Parse(""))
}

// Match 检查是否匹配openEuler系统
func (h *OpenEulerBondHandler) Match(osInfo controller.OSInfo) bool {
	// 检查操作系统ID
	if osInfo.ID != "openeuler" {
		return false
	}

	// 检查内核名称
	if osInfo.KernelName != "Linux" {
		return false
	}

	// openEuler版本支持检查
	if !h.isOpenEulerSupported(osInfo.VersionID) {
		return false
	}

	h.osInfo = osInfo
	utils.Infof("bond", "OpenEuler Bond handler matched for version %s", osInfo.VersionID)
	return true
}

// Reconcile 执行Bond配置调谐
func (h *OpenEulerBondHandler) Reconcile(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
	utils.Infof("bond", "Starting openEuler bond reconciliation with %d configurations", len(configs))
	return h.reconcileConfigs(ctx, configs)
}

// isOpenEulerSupported 检查给定的版本ID是否为支持的openEuler版本
func (h *OpenEulerBondHandler) isOpenEulerSupported(versionID string) bool {
	if versionID == "" {
		return false
	}

	// 支持通用的openEuler版本标识
	if versionID == "20" || versionID == "22" || versionID == "24" {
		return true
	}

	// 支持的openEuler版本范围：20.03 LTS 及以上
	if strings.HasPrefix(versionID, "20.") || strings.HasPrefix(versionID, "22.") || strings.HasPrefix(versionID, "24.") {
		utils.Debugf("bond", "OpenEuler version %s is supported", versionID)
		return true
	}

	utils.Debugf("bond", "OpenEuler version %s is not supported", versionID)
	return false
}

// reconcileConfigs 调谐Bond配置
func (h *OpenEulerBondHandler) reconcileConfigs(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
	var results []*domain.ReconcileResult
	var matchedConfig *systemv1.ResourceConfig
	var fallbackConfig *systemv1.ResourceConfig
	configStatusMap := make(map[string]*domain.ReconcileResult)

	utils.Infof("bond", "Starting openEuler bond reconciliation with %d configurations", len(configs))

	// 遍历所有配置，查找匹配的配置和fallback配置
	for _, config := range configs {
		utils.Debugf("bond", "Processing bond configuration: %s", config.Metadata.Name)
		// 解析配置规格
		bondSpec, err := utils.UnmarshalSpec[*systemv1.BondConfigurationSpec](config.Spec)
		if err != nil {
			utils.Errorf("bond", "Failed to unmarshal bond spec for %s: %v", config.Metadata.Name, err)
			result, _ := status.ReconcileError(config, status.ReasonSpecError, fmt.Errorf("failed to unmarshal spec: %v", err))
			configStatusMap[config.Metadata.Name] = result
			continue
		}

		// 检查nodeSelector匹配
		if bondSpec.NodeSelector != nil && h.hasValidNodeSelector(bondSpec.NodeSelector) {
			utils.Debugf("bond", "Checking nodeSelector for bond configuration: %s", config.Metadata.Name)
			// 有有效的nodeSelector，检查是否匹配
			matched, err := utils.MatchNodeSelector(bondSpec.NodeSelector)
			if err != nil {
				utils.Errorf("bond", "Failed to match nodeSelector for %s: %v", config.Metadata.Name, err)
				result, _ := status.ReconcileError(config, status.ReasonNodeSelectorError, fmt.Errorf("failed to match nodeSelector: %v", err))
				configStatusMap[config.Metadata.Name] = result
				continue
			}
			if matched {
				utils.Infof("bond", "Bond configuration %s matches nodeSelector", config.Metadata.Name)
				matchedConfig = config
				// 暂时不设置状态，等待应用后再设置
			} else {
				utils.Debugf("bond", "Bond configuration %s does not match nodeSelector", config.Metadata.Name)
				result, _ := status.ReconcileSkipped(config, status.ReasonNotMatched, "Configuration did not match nodeSelector")
				configStatusMap[config.Metadata.Name] = result
			}
		} else {
			utils.Debugf("bond", "Bond configuration %s has no valid nodeSelector, considering as fallback", config.Metadata.Name)
			// 没有有效的nodeSelector，作为fallback配置
			if fallbackConfig == nil {
				fallbackConfig = config
				// 暂时不设置状态，等待应用后再设置
			} else {
				// 如果已经有fallback配置，则跳过这个
				utils.Debugf("bond", "Bond configuration %s skipped, another fallback already exists", config.Metadata.Name)
				result, _ := status.ReconcileSkipped(config, status.ReasonFallback, "Another fallback configuration already exists")
				configStatusMap[config.Metadata.Name] = result
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
			result, _ := status.ReconcileError(configToApply, status.ReasonSpecError, fmt.Errorf("failed to unmarshal effective config spec: %v", err))
			configStatusMap[configToApply.Metadata.Name] = result
		} else {
			utils.Debugf("bond", "Starting bond configuration application")
			// 应用配置
			applyResult, err := h.applyBondConfiguration(ctx, configToApply, bondSpec)
			if err != nil {
				utils.Errorf("bond", "Failed to apply bond configuration: %v", err)
				result, _ := status.ReconcileError(configToApply, status.ReasonReconcileError, fmt.Errorf("failed to apply configuration: %v", err))
				configStatusMap[configToApply.Metadata.Name] = result
			} else {
				utils.Infof("bond", "Bond configuration applied successfully")
				// 应用成功
				configStatusMap[configToApply.Metadata.Name] = applyResult
			}
		}

		// 为其他未应用的配置设置跳过状态
		for _, config := range configs {
			if _, exists := configStatusMap[config.Metadata.Name]; !exists {
				if config == configToApply {
					// 这种情况不应该发生，但为了安全起见
					continue
				}
				var reason, message string
				if config == fallbackConfig {
					reason = status.ReasonFallback
					message = "Configuration used as fallback but not applied due to matched configuration"
				} else {
					reason = status.ReasonNotMatched
					message = "Configuration not applied"
				}
				result, _ := status.ReconcileSkipped(config, reason, message)
				configStatusMap[config.Metadata.Name] = result
			}
		}
	} else {
		// 没有配置可以应用，所有配置都标记为跳过
		for _, config := range configs {
			if _, exists := configStatusMap[config.Metadata.Name]; !exists {
				result, _ := status.ReconcileSkipped(config, status.ReasonNotMatched, "No applicable configuration found")
				configStatusMap[config.Metadata.Name] = result
			}
		}
	}

	// 按照原始配置顺序构建结果
	for _, config := range configs {
		if result, exists := configStatusMap[config.Metadata.Name]; exists {
			results = append(results, result)
		}
	}

	utils.Infof("bond", "OpenEuler bond reconciliation completed with %d results", len(results))
	return results, nil
}

// hasValidNodeSelector 检查nodeSelector是否有效
func (h *OpenEulerBondHandler) hasValidNodeSelector(nodeSelector *systemv1.NodeSelector) bool {
	if nodeSelector == nil {
		return false
	}
	return nodeSelector.MachineId != ""
}

// applyBondConfiguration 应用Bond配置
func (h *OpenEulerBondHandler) applyBondConfiguration(ctx context.Context, configToApply *systemv1.ResourceConfig, bondSpec *systemv1.BondConfigurationSpec) (*domain.ReconcileResult, error) {
	// 检测并选择合适的Bond管理器
	manager, err := h.detectBondManager(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect openEuler bond manager: %v", err)
	}

	// 验证Bond配置
	if err := h.validateOpenEulerBondConfig(bondSpec); err != nil {
		return status.ReconcileError(configToApply, status.ReasonSpecError, fmt.Errorf("bond validation failed: %v", err))
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
		return status.ReconcileError(configToApply, status.ReasonReconcileError, fmt.Errorf("failed to configure bond %s: %v", bondSpec.Name, err))
	}

	utils.Infof("bond", "OpenEuler bond %s configured successfully", bondSpec.Name)

	// 只有在配置发生变更时才重新加载Bond配置
	if hasChanges {
		utils.Infof("bond", "Bond configuration changed, reloading bond services")
		if err := manager.ReloadIfy(ctx); err != nil {
			return status.ReconcileError(configToApply, status.ReasonReconcileError, fmt.Errorf("failed to reload bond configuration: %v", err))
		}
		utils.Infof("bond", "OpenEuler bond configuration applied and reloaded successfully")
		return status.ReconcileReady(configToApply, status.ReasonNoChange, "OpenEuler bond configuration applied and reloaded successfully")
	} else {
		utils.Infof("bond", "OpenEuler bond configuration unchanged, skipping reload")
		return status.ReconcileReady(configToApply, status.ReasonNoChange, "OpenEuler bond configuration unchanged")
	}
}

// detectBondManager 检测并选择合适的Bond管理器
func (h *OpenEulerBondHandler) detectBondManager(ctx context.Context) (types.IBondManager, error) {
	utils.Debugf("bond", "Detecting openEuler bond manager")

	// 按优先级检测Bond管理器
	managers := []types.IBondManager{
		NewOpenEulerBondNetworkManager(&h.osInfo),
		NewOpenEulerBondNetplan(&h.osInfo),
		NewOpenEulerBondIfupdown(&h.osInfo),
	}

	for _, manager := range managers {
		if manager.IsInstall(ctx) {
			utils.Infof("bond", "Selected openEuler bond manager: %T", manager)
			return manager, nil
		}
	}

	return nil, fmt.Errorf("no suitable bond manager found for openEuler")
}

// validateOpenEulerBondConfig 验证openEuler Bond配置
func (h *OpenEulerBondHandler) validateOpenEulerBondConfig(bondSpec *systemv1.BondConfigurationSpec) error {
	if bondSpec == nil {
		return fmt.Errorf("bond specification cannot be nil")
	}

	// 验证Bond名称
	if bondSpec.Name == "" {
		return fmt.Errorf("bond name cannot be empty")
	}

	// 验证Bond名称长度（Linux接口名称限制）
	if len(bondSpec.Name) > 15 {
		return fmt.Errorf("bond name too long (max 15 characters): %s", bondSpec.Name)
	}

	// 验证Bond模式
	if bondSpec.Mode < 0 || bondSpec.Mode > 7 {
		return fmt.Errorf("invalid bond mode %d, must be between 0-7", bondSpec.Mode)
	}

	// 验证Miimon值
	if bondSpec.Miimon <= 0 {
		return fmt.Errorf("invalid miimon value %d, must be positive", bondSpec.Miimon)
	}

	// 验证网络配置
	if bondSpec.Network != nil {
		if bondSpec.Network.Mtu > 0 && (bondSpec.Network.Mtu < 68 || bondSpec.Network.Mtu > 9000) {
			return fmt.Errorf("invalid MTU value %d, must be between 68-9000", bondSpec.Network.Mtu)
		}
	}

	return nil
}