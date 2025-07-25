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
	// 检查操作系统ID（支持大小写不敏感匹配）
	osID := strings.ToLower(osInfo.ID)
	if osID != "openeuler" {
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

// Reconcile 执行Bond配置调谐，返回结构化的调谐结果
func (h *OpenEulerBondHandler) Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*status.ReconcileResult, error) {
	utils.Infof("bond", "Starting openEuler bond reconciliation for config: %s", config.Metadata.Name)

	// 解析配置规格
	bondSpec, err := utils.UnmarshalSpec[*systemv1.BondConfigurationSpec](config.Spec)
	if err != nil {
		utils.Errorf("bond", "Failed to unmarshal bond spec for %s: %v", config.Metadata.Name, err)
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonSpecError,
				Message: fmt.Sprintf("failed to unmarshal spec: %v", err),
			},
			Error: err,
		}, nil
	}

	utils.Debugf("bond", "Starting bond configuration application")
	// 应用配置
	return h.applyBondConfiguration(ctx, config, bondSpec)
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

// applyBondConfiguration 应用Bond配置
func (h *OpenEulerBondHandler) applyBondConfiguration(ctx context.Context, config *systemv1.ResourceConfig, bondSpec *systemv1.BondConfigurationSpec) (*status.ReconcileResult, error) {
	// 检测并选择合适的Bond管理器
	manager, err := h.detectBondManager(ctx)
	if err != nil {
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonReconcileError,
				Message: fmt.Sprintf("failed to detect openEuler bond manager: %v", err),
			},
			Error: err,
		}, nil
	}

	// 验证Bond配置
	if err := h.validateOpenEulerBondConfig(bondSpec); err != nil {
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonSpecError,
				Message: fmt.Sprintf("bond validation failed: %v", err),
			},
			Error: err,
		}, nil
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
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonReconcileError,
				Message: fmt.Sprintf("failed to configure bond %s: %v", bondSpec.Name, err),
			},
			Error: err,
		}, nil
	}

	utils.Infof("bond", "OpenEuler bond %s configured successfully", bondSpec.Name)

	// 只有在配置发生变更时才重新加载Bond配置
	if hasChanges {
		utils.Infof("bond", "Bond configuration changed, reloading bond services")
		if err := manager.ReloadIfy(ctx); err != nil {
			return &status.ReconcileResult{
				Config: config,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  status.ReasonReconcileError,
					Message: fmt.Sprintf("failed to reload bond configuration: %v", err),
				},
				Error: err,
			}, nil
		}
		utils.Infof("bond", "OpenEuler bond configuration applied and reloaded successfully")
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonConfigurationUpdated,
				Message: "OpenEuler bond configuration applied and reloaded successfully",
			},
		}, nil
	} else {
		utils.Infof("bond", "OpenEuler bond configuration unchanged, skipping reload")
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonAppliedSuccessfully,
				Message: "OpenEuler bond configuration unchanged, skipping reload",
			},
		}, nil
	}
}

// detectBondManager 检测并选择合适的Bond管理器
// 按优先级：NetworkManager > Ifupdown
// 注意：openEuler不支持Netplan，已移除相关代码
func (h *OpenEulerBondHandler) detectBondManager(ctx context.Context) (types.IBondManager, error) {
	utils.Debugf("bond", "Detecting openEuler bond manager")

	// 按优先级检测Bond管理器
	managers := []types.IBondManager{
		// NewOpenEulerBondIfupdown(&h.osInfo), 不再支持network 服务
		NewOpenEulerBondNetworkManager(&h.osInfo),
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
