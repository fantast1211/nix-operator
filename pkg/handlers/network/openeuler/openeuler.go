package openeuler

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

// OpenEulerNetworkHandler openEuler 网络处理器
type OpenEulerNetworkHandler struct {
	osInfo   controller.OSInfo
	managers []types.INetworkManager
}

// NewOpenEulerNetworkHandler 创建 openEuler 网络处理器实例
func NewOpenEulerNetworkHandler() *OpenEulerNetworkHandler {
	return &OpenEulerNetworkHandler{}
}

// NewOpenEulerNetworkHandlerForTest 创建测试用的 openEuler 网络处理器
func NewOpenEulerNetworkHandlerForTest(osInfo controller.OSInfo) *OpenEulerNetworkHandler {
	handler := &OpenEulerNetworkHandler{
		osInfo:   osInfo,
		managers: []types.INetworkManager{},
	}
	// 初始化测试模式的网络管理器
	handler.initializeTestManagers()
	return handler
}

// initializeTestManagers 初始化测试模式的网络管理器
func (h *OpenEulerNetworkHandler) initializeTestManagers() {
	h.managers = []types.INetworkManager{
		// NewOpenEulerIfupdownForTest(&h.osInfo), network 服务在欧拉上好像不被支持了
		NewOpenEulerNetworkManagerForTest(&h.osInfo), // 优先级 2
	}
}

// Match 检查是否匹配 openEuler 系统
func (h *OpenEulerNetworkHandler) Match(osInfo controller.OSInfo) bool {
	utils.Infof("network", "[openEuler] Checking openEuler match: ID='%s', VersionID='%s', KernelName='%s'", osInfo.ID, osInfo.VersionID, osInfo.KernelName)

	// 检查是否为 openEuler 系统（支持大小写不敏感匹配）
	osID := strings.ToLower(osInfo.ID)
	if osID != "openeuler" {
		utils.Infof("network", "[openEuler] OS ID mismatch: expected 'openeuler', got '%s' (normalized: '%s')", osInfo.ID, osID)
		return false
	}

	h.osInfo = osInfo
	h.initializeManagers()
	utils.Infof("network", "openEuler %s network handler matched", osInfo.VersionID)
	return true
}

// Reconcile 执行网络配置调谐，返回结构化的调谐结果
func (h *OpenEulerNetworkHandler) Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*status.ReconcileResult, error) {
	utils.Infof("network", "Starting OpenEuler network configuration reconciliation for config: %s", config.Metadata.Name)

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
// 按照用户要求的优先级：ifupdown(1) > NetworkManager(2)
// 注意：openEuler不支持Netplan，已移除相关代码
func (h *OpenEulerNetworkHandler) initializeManagers() {
	h.managers = []types.INetworkManager{
		NewOpenEulerIfupdown(&h.osInfo),       // 1. ifupdown（传统网络脚本）- 最高优先级
		NewOpenEulerNetworkManager(&h.osInfo), // 2. NetworkManager - 中等优先级
	}

	utils.Infof("network", "Initialized openEuler %s network managers with priority: ifupdown > NetworkManager", h.osInfo.VersionID)
}

// applyConfiguration 应用网络配置
func (h *OpenEulerNetworkHandler) applyConfiguration(ctx context.Context, config *systemv1.ResourceConfig, networkSpec *systemv1.NetworkConfigurationSpec) (*status.ReconcileResult, error) {
	// 检测并选择合适的网络管理器
	manager, err := h.detectNetworkManager(ctx)
	if err != nil {
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonReconcileError,
				Message: fmt.Sprintf("failed to detect openEuler network manager: %v", err),
			},
			Error: err,
		}, nil
	}

	// 跟踪是否有任何配置变更
	hasChanges := false

	// 遍历并配置所有网络接口
	for _, interfaceSpec := range networkSpec.Interfaces {
		// 验证接口配置
		if err := h.validateOpenEulerInterface(interfaceSpec); err != nil {
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
					Message: fmt.Sprintf("failed to configure interface %s: %v", interfaceSpec.Name, err),
				},
				Error: err,
			}, nil
		}
		if changed {
			hasChanges = true
		}

		utils.Infof("network", "openEuler interface %s configured successfully", interfaceSpec.Name)
	}

	// 只有在有配置变更时才重新加载网络配置
	if hasChanges {
		utils.Info("network", "openEuler network configuration changed, reloading network services")
		if err := manager.ReloadIfy(ctx); err != nil {
			return &status.ReconcileResult{
				Config: config,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  status.ReasonReconcileError,
					Message: fmt.Sprintf("failed to reload network configuration: %v", err),
				},
				Error: err,
			}, nil
		}
		utils.Infof("network", "openEuler network configuration applied and reloaded successfully")
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonConfigurationUpdated,
				Message: "openEuler network configuration applied and reloaded successfully",
			},
		}, nil
	} else {
		utils.Info("network", "openEuler network configuration unchanged, skipping network service reload")
		utils.Infof("network", "openEuler network configuration verified, no changes needed")
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  status.ReasonAppliedSuccessfully,
				Message: "openEuler network configuration verified, no changes needed",
			},
		}, nil
	}
}

// detectNetworkManager 检测并选择合适的网络管理器
func (h *OpenEulerNetworkHandler) detectNetworkManager(ctx context.Context) (types.INetworkManager, error) {
	utils.Infof("network", "[openEuler] Detecting openEuler network manager, checking %d managers", len(h.managers))

	// 按优先级检测网络管理器：ifupdown(1) > NetworkManager(2)
	for i, manager := range h.managers {
		utils.Infof("network", "[openEuler] Checking manager %d: %T", i+1, manager)
		if manager.IsInstall(ctx) {
			utils.Info("network", fmt.Sprintf("Selected openEuler network manager: %T on %s %s", manager, h.osInfo.ID, h.osInfo.VersionID))
			return manager, nil
		} else {
			utils.Infof("network", "[openEuler] Manager %T is not available", manager)
		}
	}

	utils.Warnf("network", "No suitable network manager found for openEuler %s", h.osInfo.VersionID)
	return nil, fmt.Errorf("no suitable network manager found for openEuler %s", h.osInfo.VersionID)
}

// validateOpenEulerInterface 验证openEuler网络接口配置
func (h *OpenEulerNetworkHandler) validateOpenEulerInterface(iface *systemv1.NetworkInterface) error {
	// 验证接口名称
	if iface.Name == "" {
		return fmt.Errorf("interface name cannot be empty")
	}

	// 验证接口名称格式（openEuler常见格式）
	if !h.isValidOpenEulerInterfaceName(iface.Name) {
		return fmt.Errorf("invalid interface name format for openEuler: %s", iface.Name)
	}

	// 验证IP地址格式（基本格式检查）
	if iface.Ipv4Address != "" {
		// 简单的IPv4 CIDR格式检查
		if !h.isValidIPv4CIDR(iface.Ipv4Address) {
			return fmt.Errorf("invalid IPv4 address format: %s", iface.Ipv4Address)
		}
	}

	// 验证IPv6地址格式
	if iface.Ipv6Address != "" {
		if !h.isValidIPv6CIDR(iface.Ipv6Address) {
			return fmt.Errorf("invalid IPv6 address format: %s", iface.Ipv6Address)
		}
	}

	return nil
}

// isValidOpenEulerInterfaceName 检查接口名称是否符合openEuler规范
func (h *OpenEulerNetworkHandler) isValidOpenEulerInterfaceName(name string) bool {
	// openEuler支持的网络接口名称格式
	validPrefixes := []string{
		"eth",  // 传统以太网接口
		"ens",  // systemd可预测网络接口名称
		"enp",  // PCI以太网接口
		"eno",  // 板载以太网接口
		"em",   // 嵌入式网络接口
		"wlan", // 无线网络接口
		"wlp",  // PCI无线接口
		"p",    // 物理接口
		"bond", // 绑定接口
		"team", // 团队接口
		"br",   // 网桥接口
		"vlan", // VLAN接口
	}

	for _, prefix := range validPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// isValidIPv4CIDR 检查IPv4 CIDR格式是否有效
func (h *OpenEulerNetworkHandler) isValidIPv4CIDR(cidr string) bool {
	// 简单的CIDR格式检查
	if cidr == "" {
		return false
	}

	// 检查是否包含斜杠
	if !strings.Contains(cidr, "/") {
		// 如果没有斜杠，检查是否为纯IP地址
		parts := strings.Split(cidr, ".")
		return len(parts) == 4
	}

	// 检查CIDR格式
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return false
	}

	// 检查IP部分
	ipParts := strings.Split(parts[0], ".")
	if len(ipParts) != 4 {
		return false
	}

	return true
}

// isValidIPv6CIDR 检查IPv6 CIDR格式是否有效
func (h *OpenEulerNetworkHandler) isValidIPv6CIDR(cidr string) bool {
	// 简单的IPv6 CIDR格式检查
	if cidr == "" {
		return false
	}

	// 检查是否包含斜杠
	if !strings.Contains(cidr, "/") {
		// 如果没有斜杠，检查是否为纯IPv6地址
		return strings.Contains(cidr, ":")
	}

	// 检查CIDR格式
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return false
	}

	// 检查IPv6地址部分是否包含冒号
	return strings.Contains(parts[0], ":")
}
