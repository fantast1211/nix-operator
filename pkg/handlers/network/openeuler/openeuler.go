package openeuler

import (
	"context"
	"fmt"
	"strings"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// OpenEulerNetworkHandler openEuler 系统专用网络处理器
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
		NewOpenEulerIfupdownForTest(&h.osInfo),       // 优先级 1
		NewOpenEulerNetworkManagerForTest(&h.osInfo), // 优先级 2
		NewOpenEulerNetplanForTest(&h.osInfo),        // 优先级 3
	}
}

// Match 检查是否匹配 openEuler 系统
func (h *OpenEulerNetworkHandler) Match(osInfo controller.OSInfo) bool {
	// 检查是否为 openEuler 系统
	if osInfo.ID != "openeuler" {
		return false
	}

	h.osInfo = osInfo
	h.initializeManagers()
	utils.Infof("network", "openEuler %s network handler matched", osInfo.VersionID)
	return true
}

// Reconcile 执行网络配置调谐
func (h *OpenEulerNetworkHandler) Reconcile(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
	return h.reconcileConfigs(ctx, configs)
}

// initializeManagers 初始化网络管理器
// 按照用户要求的优先级：ifupdown(1) > NetworkManager(2) > Netplan(3)
func (h *OpenEulerNetworkHandler) initializeManagers() {
	h.managers = []types.INetworkManager{
		NewOpenEulerIfupdown(&h.osInfo),       // 1. ifupdown（传统网络脚本）- 最高优先级
		NewOpenEulerNetworkManager(&h.osInfo), // 2. NetworkManager - 中等优先级
		NewOpenEulerNetplan(&h.osInfo),        // 3. Netplan - 最低优先级
	}

	utils.Infof("network", "Initialized openEuler %s network managers with priority: ifupdown > NetworkManager > Netplan", h.osInfo.VersionID)
}

// reconcileConfigs 调谐网络配置
func (h *OpenEulerNetworkHandler) reconcileConfigs(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
	var results []*domain.ReconcileResult
	var matchedConfig *systemv1.ResourceConfig
	var fallbackConfig *systemv1.ResourceConfig
	configStatusMap := make(map[string]*domain.ReconcileResult)

	// 遍历所有配置，查找匹配的配置和fallback配置
	for _, config := range configs {
		// 解析配置规格
		networkSpec, err := utils.UnmarshalSpec[*systemv1.NetworkConfigurationSpec](config.Spec)
		if err != nil {
			result, _ := status.ReconcileError(config, status.ReasonSpecError, fmt.Errorf("failed to unmarshal spec: %v", err))
			configStatusMap[config.Metadata.Name] = result
			continue
		}

		// 检查nodeSelector匹配
		if networkSpec.NodeSelector != nil && h.hasValidNodeSelector(networkSpec.NodeSelector) {
			// 有有效的nodeSelector，检查是否匹配
			matched, err := utils.MatchNodeSelector(networkSpec.NodeSelector)
			if err != nil {
				result, _ := status.ReconcileError(config, status.ReasonNodeSelectorError, fmt.Errorf("failed to match nodeSelector: %v", err))
				configStatusMap[config.Metadata.Name] = result
				continue
			}

			if matched {
				// 找到匹配的配置
				matchedConfig = config
				utils.Infof("network", "Found matching openEuler network config: %s", config.Metadata.Name)
				break
			} else {
				// nodeSelector不匹配
				result, _ := status.ReconcileSkipped(config, status.ReasonNotMatched, "nodeSelector does not match current node")
				configStatusMap[config.Metadata.Name] = result
			}
		} else {
			// 没有nodeSelector或nodeSelector无效，作为fallback配置
			if fallbackConfig == nil {
				fallbackConfig = config
				utils.Infof("network", "Found fallback openEuler network config: %s", config.Metadata.Name)
			} else {
				// 多个fallback配置，跳过后续的
				result, _ := status.ReconcileSkipped(config, status.ReasonNotMatched, "multiple fallback configs found, using first one")
				configStatusMap[config.Metadata.Name] = result
			}
		}
	}

	// 确定要应用的配置
	var configToApply *systemv1.ResourceConfig
	if matchedConfig != nil {
		configToApply = matchedConfig
	} else if fallbackConfig != nil {
		configToApply = fallbackConfig
	}

	// 应用配置
	if configToApply != nil {
		networkSpec, _ := utils.UnmarshalSpec[*systemv1.NetworkConfigurationSpec](configToApply.Spec)
		result, err := h.applyConfiguration(ctx, configToApply, networkSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to apply openEuler network configuration: %v", err)
		}
		configStatusMap[configToApply.Metadata.Name] = result
	}

	// 为所有未处理的配置设置跳过状态
	for _, config := range configs {
		if _, exists := configStatusMap[config.Metadata.Name]; !exists {
			result, _ := status.ReconcileSkipped(config, status.ReasonNotMatched, "no matching nodeSelector found")
			configStatusMap[config.Metadata.Name] = result
		}
	}

	// 收集所有结果
	for _, config := range configs {
		if result, exists := configStatusMap[config.Metadata.Name]; exists {
			results = append(results, result)
		}
	}

	return results, nil
}

// hasValidNodeSelector 检查nodeSelector是否有效
func (h *OpenEulerNetworkHandler) hasValidNodeSelector(selector *systemv1.NodeSelector) bool {
	if selector == nil {
		return false
	}
	return selector.MachineId != ""
}

// applyConfiguration 应用网络配置
func (h *OpenEulerNetworkHandler) applyConfiguration(ctx context.Context, configToApply *systemv1.ResourceConfig, networkSpec *systemv1.NetworkConfigurationSpec) (*domain.ReconcileResult, error) {
	// 检测并选择合适的网络管理器
	manager, err := h.detectNetworkManager(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect openEuler network manager: %v", err)
	}

	// 跟踪是否有任何配置变更
	hasChanges := false

	// 遍历并配置所有网络接口
	for _, interfaceSpec := range networkSpec.Interfaces {
		// 验证接口配置
		if err := h.validateOpenEulerInterface(interfaceSpec); err != nil {
			return status.ReconcileError(configToApply, status.ReasonSpecError, fmt.Errorf("interface validation failed: %v", err))
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
			return status.ReconcileError(configToApply, status.ReasonReconcileError, fmt.Errorf("failed to configure interface %s: %v", interfaceSpec.Name, err))
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
			return status.ReconcileError(configToApply, status.ReasonReconcileError, fmt.Errorf("failed to reload network configuration: %v", err))
		}
		utils.Infof("network", "openEuler network configuration applied and reloaded successfully")
		return status.ReconcileReady(configToApply, status.ReasonNoChange, "openEuler network configuration applied and reloaded successfully")
	} else {
		utils.Info("network", "openEuler network configuration unchanged, skipping network service reload")
		utils.Infof("network", "openEuler network configuration verified, no changes needed")
		return status.ReconcileReady(configToApply, status.ReasonNoChange, "openEuler network configuration verified, no changes needed")
	}
}

// detectNetworkManager 检测并选择合适的网络管理器
func (h *OpenEulerNetworkHandler) detectNetworkManager(ctx context.Context) (types.INetworkManager, error) {
	// 按优先级检测网络管理器：ifupdown(1) > NetworkManager(2) > Netplan(3)
	for _, manager := range h.managers {
		if manager.IsInstall(ctx) {
			utils.Info("network", fmt.Sprintf("Selected openEuler network manager: %T on %s %s", manager, h.osInfo.ID, h.osInfo.VersionID))
			return manager, nil
		}
	}

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
