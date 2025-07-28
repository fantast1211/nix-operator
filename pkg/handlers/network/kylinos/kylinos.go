package kylinos

import (
	"context"
	"fmt"
	"strings"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// KylinOSNetworkHandler KylinOS 系统专用网络处理器
type KylinOSNetworkHandler struct {
	osInfo   controller.OSInfo
	managers []types.INetworkManager
}

// NewKylinOSNetworkHandler 创建 KylinOS 网络处理器实例
func NewKylinOSNetworkHandler() *KylinOSNetworkHandler {
	return &KylinOSNetworkHandler{}
}

// Match 检查是否匹配 KylinOS 系统
func (h *KylinOSNetworkHandler) Match(osInfo controller.OSInfo) bool {
	// 检查是否为 KylinOS 系统（支持大小写不敏感匹配）
	osID := strings.ToLower(osInfo.ID)
	utils.Infof("network", "[KylinOS] Checking OS ID: '%s' (lowercase: '%s')", osInfo.ID, osID)
	if osID != "kylin" {
		utils.Infof("network", "[KylinOS] OS ID '%s' does not match 'kylin'", osInfo.ID)
		return false
	}

	h.osInfo = osInfo
	h.initializeManagers()
	utils.Infof("network", "KylinOS %s network handler matched", osInfo.VersionID)
	return true
}

// Reconcile 执行网络配置调谐
func (h *KylinOSNetworkHandler) Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*status.ReconcileResult, error) {
	utils.Infof("network", "Starting KylinOS network configuration reconciliation for config: %s", config.Metadata.Name)

	// 解析配置规格
	networkSpec, err := utils.UnmarshalSpec[*systemv1.NetworkConfigurationSpec](config.Spec)
	if err != nil {
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

	// 应用配置
	return h.applyConfiguration(ctx, config, networkSpec)
}

// initializeManagers 初始化网络管理器
// 按照用户要求的优先级：ifupdown(1) > NetworkManager(2) > Netplan(3)
func (h *KylinOSNetworkHandler) initializeManagers() {
	h.managers = []types.INetworkManager{
		// NewKylinOSIfupdown(&h.osInfo), // 1. ifupdown（传统网络脚本）- 最高优先级
		NewKylinOSNetworkManager(&h.osInfo), // 2. NetworkManager - 中等优先级
		// NewKylinOSNetplan(&h.osInfo),        // 3. Netplan - 最低优先级 暂时注释掉
	}

	utils.Infof("network", "Initialized KylinOS %s network managers with priority: ifupdown > NetworkManager > Netplan", h.osInfo.VersionID)
}

// applyConfiguration 应用网络配置
func (h *KylinOSNetworkHandler) applyConfiguration(ctx context.Context, config *systemv1.ResourceConfig, networkSpec *systemv1.NetworkConfigurationSpec) (*status.ReconcileResult, error) {
	// 检测并选择合适的网络管理器
	manager, err := h.detectNetworkManager(ctx)
	if err != nil {
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonReconcileError,
				Message: fmt.Sprintf("failed to detect KylinOS network manager: %v", err),
			},
			Error: err,
		}, nil
	}

	// 跟踪是否有任何配置变更
	hasChanges := false

	// 遍历并配置所有网络接口
	for _, interfaceSpec := range networkSpec.Interfaces {
		// 验证接口配置
		if err := h.validateKylinOSInterface(interfaceSpec); err != nil {
			return &status.ReconcileResult{
				Config: config,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  status.ReasonSpecError,
					Message: fmt.Sprintf("interface validation failed: %v", err),
				},
				Error: err,
			}, nil
		}

		// 转换为内部接口结构
		iface := types.Interface{
			Name:        interfaceSpec.Name,
			IPv4Address: interfaceSpec.Ipv4Address,
			IPv6Address: interfaceSpec.Ipv6Address,
			IPv4Gateway: interfaceSpec.Ipv4Gateway,
			IPv6Gateway: interfaceSpec.Ipv6Gateway,
			MTU:         int(interfaceSpec.Mtu),
			Nameservers: interfaceSpec.Nameservers,
		}

		// 转换BondingSlave配置
		if interfaceSpec.BondingSlave != nil {
			utils.Infof("network", "Converting bond config for interface %s: enabled=%v, master=%s",
				interfaceSpec.Name, interfaceSpec.BondingSlave.Enabled, interfaceSpec.BondingSlave.Master)
			iface.BondingSlave = &types.BondingSlaveConfig{
				Enabled: interfaceSpec.BondingSlave.Enabled,
				Master:  interfaceSpec.BondingSlave.Master,
			}
		} else {
			utils.Infof("network", "No bond config found for interface %s", interfaceSpec.Name)
		}

		// 使用ConfigureWithCheck检查配置是否有变更
		changed, err := manager.ConfigureWithCheck(ctx, iface)
		if err != nil {
			return &status.ReconcileResult{
				Config: config,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  status.ReasonReconcileError,
					Message: fmt.Sprintf("failed to configure network interface %s: %v", interfaceSpec.Name, err),
				},
				Error: err,
			}, nil
		}
		if changed {
			hasChanges = true
		}
	}

	// 只有在有配置变更时才重新加载网络配置
	if hasChanges {
		utils.Info("network", "KylinOS network configuration changed, reloading network services")
		if err := manager.ReloadIfy(ctx); err != nil {
			return &status.ReconcileResult{
				Config: config,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  status.ReasonReconcileError,
					Message: fmt.Sprintf("failed to reload KylinOS network configuration: %v", err),
				},
				Error: err,
			}, nil
		}
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonConfigurationUpdated,
				Message: "KylinOS network configuration applied and reloaded successfully",
			},
		}, nil
	} else {
		utils.Info("network", "KylinOS network configuration unchanged, skipping network service reload")
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonAppliedSuccessfully,
				Message: "KylinOS network configuration verified, no changes needed",
			},
		}, nil
	}
}

// detectNetworkManager 检测系统中可用的网络管理器
func (h *KylinOSNetworkHandler) detectNetworkManager(ctx context.Context) (types.INetworkManager, error) {
	// 按优先级检测网络管理器：ifupdown > NetworkManager > Netplan
	for _, manager := range h.managers {
		if manager.IsInstall(ctx) {
			utils.Infof("network", "Selected KylinOS network manager: %T on %s %s", manager, h.osInfo.ID, h.osInfo.VersionID)
			return manager, nil
		}
	}

	return nil, fmt.Errorf("no supported network manager found on KylinOS %s", h.osInfo.VersionID)
}

// validateKylinOSInterface 验证 KylinOS 网络接口配置
func (h *KylinOSNetworkHandler) validateKylinOSInterface(iface *systemv1.NetworkInterface) error {
	// 验证接口名称
	if iface.Name == "" {
		return fmt.Errorf("interface name cannot be empty")
	}

	// 验证接口名称格式（KylinOS 特定规则）
	if !isValidKylinOSInterfaceName(iface.Name) {
		return fmt.Errorf("invalid interface name format for KylinOS: %s", iface.Name)
	}

	// 验证 IPv4 地址格式
	if iface.Ipv4Address != "" {
		if !isValidIPv4CIDR(iface.Ipv4Address) {
			return fmt.Errorf("invalid IPv4 address format: %s", iface.Ipv4Address)
		}
	}

	// 验证 IPv6 地址格式
	if iface.Ipv6Address != "" {
		if !isValidIPv6CIDR(iface.Ipv6Address) {
			return fmt.Errorf("invalid IPv6 address format: %s", iface.Ipv6Address)
		}
	}

	// 验证 MTU 值
	if iface.Mtu > 0 && (iface.Mtu < 68 || iface.Mtu > 9000) {
		return fmt.Errorf("invalid MTU value: %d (must be between 68 and 9000)", iface.Mtu)
	}

	return nil
}

// isValidKylinOSInterfaceName 验证 KylinOS 接口名称格式
func isValidKylinOSInterfaceName(name string) bool {
	// KylinOS 接口名称规则：
	// - 以字母开头
	// - 可包含字母、数字、下划线、连字符
	// - 长度不超过15个字符
	if len(name) == 0 || len(name) > 15 {
		return false
	}

	// 检查首字符是否为字母
	if !((name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z')) {
		return false
	}

	// 检查其他字符
	for _, char := range name[1:] {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-') {
			return false
		}
	}

	return true
}

// isValidIPv4CIDR 验证 IPv4 CIDR 格式
func isValidIPv4CIDR(cidr string) bool {
	if !strings.Contains(cidr, "/") {
		return false
	}
	// 这里可以添加更详细的 IPv4 CIDR 验证逻辑
	return true
}

// isValidIPv6CIDR 验证 IPv6 CIDR 格式
func isValidIPv6CIDR(cidr string) bool {
	if !strings.Contains(cidr, "/") {
		return false
	}
	// 这里可以添加更详细的 IPv6 CIDR 验证逻辑
	return true
}
