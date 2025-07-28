package kylinos

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

// KylinOSBondHandler KylinOS系统专用的Bond处理器
type KylinOSBondHandler struct {
	osInfo   controller.OSInfo
	managers []types.IBondManager
}

// NewKylinOSBondHandler 创建KylinOS Bond处理器实例
func NewKylinOSBondHandler() *KylinOSBondHandler {
	return &KylinOSBondHandler{}
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

// Match 检查是否匹配KylinOS系统
func (h *KylinOSBondHandler) Match(osInfo controller.OSInfo) bool {
	// 检查操作系统ID（支持大小写不敏感匹配）
	osID := strings.ToLower(osInfo.ID)
	utils.Infof("bond", "[KylinOS] Checking OS ID: '%s' (lowercase: '%s')", osInfo.ID, osID)
	if osID != "kylin" {
		utils.Infof("bond", "[KylinOS] OS ID '%s' does not match 'kylin'", osInfo.ID)
		return false
	}

	// 检查内核名称
	if osInfo.KernelName != "Linux" {
		return false
	}

	// KylinOS版本支持检查
	if !h.isKylinOSSupported(osInfo.VersionID) {
		return false
	}

	h.osInfo = osInfo
	h.initializeManagers()
	utils.Infof("bond", "KylinOS Bond handler matched for version %s", osInfo.VersionID)
	return true
}

// Reconcile 执行Bond配置调谐，返回结构化的调谐结果
func (h *KylinOSBondHandler) Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*status.ReconcileResult, error) {
	utils.Infof("bond", "Starting KylinOS bond reconciliation for config: %s", config.Metadata.Name)

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

// isKylinOSSupported 检查给定的版本ID是否为支持的KylinOS版本
func (h *KylinOSBondHandler) isKylinOSSupported(versionID string) bool {
	if versionID == "" {
		return false
	}

	// 支持通用的KylinOS版本标识
	if versionID == "V10" || versionID == "10" {
		return true
	}

	// 支持的KylinOS版本范围：V10 及以上
	if strings.HasPrefix(versionID, "V10") || strings.HasPrefix(versionID, "10.") {
		utils.Debugf("bond", "KylinOS version %s is supported", versionID)
		return true
	}

	utils.Debugf("bond", "KylinOS version %s is not supported", versionID)
	return false
}

// initializeManagers 初始化Bond管理器
func (h *KylinOSBondHandler) initializeManagers() {
	h.managers = []types.IBondManager{
		// 1. ifupdown（优先级最高）
		// NewKylinOSBondIfupdown(&h.osInfo),
		// 2. NetworkManager（中等优先级）
		NewKylinOSBondNetworkManager(&h.osInfo),
		// 3. Netplan（优先级最低）
		// NewKylinOSBondNetplan(&h.osInfo),
	}
	utils.Infof("bond", "Initialized KylinOS %s bond managers with priority: ifupdown > NetworkManager > Netplan", h.osInfo.VersionID)
}

// detectBondManager 检测并选择合适的Bond管理器
func (h *KylinOSBondHandler) detectBondManager(ctx context.Context) (types.IBondManager, error) {
	for _, manager := range h.managers {
		if manager.IsInstall(ctx) {
			utils.Infof("bond", "Selected KylinOS bond manager: %T on kylin %s", manager, h.osInfo.VersionID)
			return manager, nil
		}
	}
	return nil, fmt.Errorf("no available KylinOS bond manager found")
}

// applyBondConfiguration 应用Bond配置
func (h *KylinOSBondHandler) applyBondConfiguration(ctx context.Context, config *systemv1.ResourceConfig, bondSpec *systemv1.BondConfigurationSpec) (*status.ReconcileResult, error) {
	// 检测并选择合适的Bond管理器
	manager, err := h.detectBondManager(ctx)
	if err != nil {
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonReconcileError,
				Message: fmt.Sprintf("failed to detect KylinOS bond manager: %v", err),
			},
			Error: err,
		}, nil
	}

	// 验证Bond配置
	if err := h.validateKylinOSBondConfig(bondSpec); err != nil {
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

	// 设置网络配置
	if bondSpec.Network != nil {
		bondConfig.Network = types.BondNetworkConfig{
			IP:         bondSpec.Network.Ip,
			Gateway:    bondSpec.Network.Gateway,
			DNSServers: bondSpec.Network.DnsServers,
			MTU:        int(bondSpec.Network.Mtu),
		}
	}

	// 设置可选参数
	if bondSpec.Options != nil {
		bondConfig.Options = types.BondOptions{
			ExtraOptions: bondSpec.Options.ExtraOptions,
		}
	}

	// 应用Bond配置
	changed, err := manager.ConfigureWithCheck(ctx, bondConfig)
	if err != nil {
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonReconcileError,
				Message: fmt.Sprintf("failed to configure bond: %v", err),
			},
			Error: err,
		}, nil
	}

	if changed {
		utils.Infof("bond", "KylinOS bond configuration changed, reloading bond services")
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonConfigurationUpdated,
				Message: "Bond configuration applied and services reloaded",
			},
		}, nil
	} else {
		utils.Infof("bond", "KylinOS bond configuration unchanged")
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonAppliedSuccessfully,
				Message: "Bond configuration unchanged",
			},
		}, nil
	}
}

// validateKylinOSBondConfig 验证KylinOS Bond配置
func (h *KylinOSBondHandler) validateKylinOSBondConfig(bondSpec *systemv1.BondConfigurationSpec) error {
	if bondSpec == nil {
		return fmt.Errorf("bond configuration spec cannot be nil")
	}

	// 验证Bond名称
	if bondSpec.Name == "" {
		return fmt.Errorf("bond name cannot be empty")
	}

	// 验证Bond模式
	if bondSpec.Mode < 0 || bondSpec.Mode > 7 {
		return fmt.Errorf("bond mode must be between 0 and 7, got %d", bondSpec.Mode)
	}

	// 验证Miimon值
	if bondSpec.Miimon < 0 {
		return fmt.Errorf("miimon must be non-negative, got %d", bondSpec.Miimon)
	}

	// 验证网络配置
	if bondSpec.Network != nil {
		// 验证IP地址格式
		if bondSpec.Network.Ip != "" {
			if _, _, err := net.ParseCIDR(bondSpec.Network.Ip); err != nil {
				return fmt.Errorf("invalid IP CIDR format: %s", bondSpec.Network.Ip)
			}
		}

		// 验证网关地址格式
		if bondSpec.Network.Gateway != "" {
			if net.ParseIP(bondSpec.Network.Gateway) == nil {
				return fmt.Errorf("invalid gateway IP format: %s", bondSpec.Network.Gateway)
			}
		}

		// 验证MTU值
		if bondSpec.Network.Mtu < 0 || bondSpec.Network.Mtu > 9000 {
			return fmt.Errorf("MTU must be between 0 and 9000, got %d", bondSpec.Network.Mtu)
		}
	}

	return nil
}
