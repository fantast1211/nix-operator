package centos

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

// CentOSNetworkHandler CentOS 7.2-7.9 专用网络处理器
type CentOSNetworkHandler struct {
	osInfo   controller.OSInfo
	managers []types.INetworkManager
}

// NewCentOSNetworkHandler 创建 CentOS 网络处理器实例
func NewCentOSNetworkHandler() *CentOSNetworkHandler {
	return &CentOSNetworkHandler{}
}

// Match 检查是否匹配 CentOS 7.2-7.9 系统
func (h *CentOSNetworkHandler) Match(osInfo controller.OSInfo) bool {
	// 检查是否为 CentOS 系统
	if osInfo.ID != "centos" {
		return false
	}

	// 使用专门的版本检查方法
	if h.isCentOS7Supported(osInfo.VersionID) {
		h.osInfo = osInfo
		h.initializeManagers()
		utils.Infof("network", "CentOS %s network handler matched", osInfo.VersionID)
		return true
	}

	return false
}

// Reconcile 执行网络配置调谐
func (h *CentOSNetworkHandler) Reconcile(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
	return h.reconcileConfigs(ctx, configs)
}

// initializeManagers 初始化网络管理器
func (h *CentOSNetworkHandler) initializeManagers() {
	// CentOS 7.2-7.9 网络管理器优先级：
	// 1. CentOS专用NetworkManager（如果可用）
	// 2. CentOS专用传统网络脚本（ifcfg-*）
	h.managers = []types.INetworkManager{
		NewCentOSNetworkManager(&h.osInfo), // CentOS专用NetworkManager优先
		NewCentOSIfupdown(&h.osInfo),       // CentOS专用传统网络脚本作为备选
	}

	utils.Infof("network", "Initialized CentOS %s network managers", h.osInfo.VersionID)
}

// reconcileConfigs 调谐网络配置
func (h *CentOSNetworkHandler) reconcileConfigs(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
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
				utils.Infof("network", "Found matching CentOS network config: %s", config.Metadata.Name)
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
				utils.Infof("network", "Found fallback CentOS network config: %s", config.Metadata.Name)
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
			return nil, fmt.Errorf("failed to apply CentOS network configuration: %v", err)
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
func (h *CentOSNetworkHandler) hasValidNodeSelector(selector *systemv1.NodeSelector) bool {
	if selector == nil {
		return false
	}
	return selector.MachineId != ""
}

// applyConfiguration 应用网络配置
func (h *CentOSNetworkHandler) applyConfiguration(ctx context.Context, configToApply *systemv1.ResourceConfig, networkSpec *systemv1.NetworkConfigurationSpec) (*domain.ReconcileResult, error) {
	// 检测并选择合适的网络管理器
	manager, err := h.detectNetworkManager(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect CentOS network manager: %v", err)
	}

	// 跟踪是否有任何配置变更
	hasChanges := false

	// 遍历并配置所有网络接口
	for _, interfaceSpec := range networkSpec.Interfaces {
		// 验证接口配置
		if err := h.validateCentOSInterface(interfaceSpec); err != nil {
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

		utils.Infof("network", "CentOS interface %s configured successfully", interfaceSpec.Name)
	}

	// 只有在有配置变更时才重新加载网络配置
	if hasChanges {
		utils.Info("network", "CentOS network configuration changed, reloading network services")
		if err := manager.ReloadIfy(ctx); err != nil {
			return status.ReconcileError(configToApply, status.ReasonReconcileError, fmt.Errorf("failed to reload network configuration: %v", err))
		}
		utils.Infof("network", "CentOS network configuration applied and reloaded successfully")
		return status.ReconcileReady(configToApply, status.ReasonNoChange, "CentOS network configuration applied and reloaded successfully")
	} else {
		utils.Info("network", "CentOS network configuration unchanged, skipping network service reload")
		utils.Infof("network", "CentOS network configuration verified, no changes needed")
		return status.ReconcileReady(configToApply, status.ReasonNoChange, "CentOS network configuration verified, no changes needed")
	}
}

// validateCentOSInterface 验证CentOS网络接口配置
func (h *CentOSNetworkHandler) validateCentOSInterface(iface *systemv1.NetworkInterface) error {
	// 验证接口名称
	if iface.Name == "" {
		return fmt.Errorf("interface name cannot be empty")
	}

	// 验证接口名称格式（CentOS常见格式）
	if !h.isValidCentOSInterfaceName(iface.Name) {
		return fmt.Errorf("invalid interface name format for CentOS: %s", iface.Name)
	}

	// 验证IP地址格式（基本格式检查）
	if iface.Ipv4Address != "" {
		// 简单的IPv4 CIDR格式检查
		if !h.isValidIPv4CIDR(iface.Ipv4Address) {
			return fmt.Errorf("invalid IPv4 address format: %s", iface.Ipv4Address)
		}
	}

	if iface.Ipv6Address != "" {
		// 简单的IPv6 CIDR格式检查
		if !h.isValidIPv6CIDR(iface.Ipv6Address) {
			return fmt.Errorf("invalid IPv6 address format: %s", iface.Ipv6Address)
		}
	}

	// 验证网关地址格式
	if iface.Ipv4Gateway != "" {
		if !h.isValidIPv4(iface.Ipv4Gateway) {
			return fmt.Errorf("invalid IPv4 gateway format: %s", iface.Ipv4Gateway)
		}
	}

	if iface.Ipv6Gateway != "" {
		if !h.isValidIPv6(iface.Ipv6Gateway) {
			return fmt.Errorf("invalid IPv6 gateway format: %s", iface.Ipv6Gateway)
		}
	}

	// 验证MTU值
	if iface.Mtu > 0 && (iface.Mtu < 68 || iface.Mtu > 9000) {
		return fmt.Errorf("MTU value %d is out of valid range (68-9000)", iface.Mtu)
	}

	// 验证DNS服务器地址
	for _, ns := range iface.Nameservers {
		if !h.isValidIP(ns) {
			return fmt.Errorf("invalid nameserver address: %s", ns)
		}
	}

	return nil
}

// isValidCentOSInterfaceName 检查接口名称是否符合CentOS规范
func (h *CentOSNetworkHandler) isValidCentOSInterfaceName(name string) bool {
	// CentOS常见的网络接口名称模式
	validPatterns := []string{
		"eth",  // 传统以太网接口
		"ens",  // systemd预测命名
		"enp",  // PCI设备命名
		"eno",  // 板载设备命名
		"em",   // 嵌入式设备命名
		"wlan", // 无线接口
		"wlp",  // 无线PCI设备
		"p",    // 物理端口命名
		"bond", // 绑定接口
		"br",   // 网桥接口
		"vlan", // VLAN接口
		"tun",  // 隧道接口
		"tap",  // TAP接口
	}

	// 检查是否匹配任何有效模式
	for _, pattern := range validPatterns {
		if len(name) >= len(pattern) && name[:len(pattern)] == pattern {
			return true
		}
	}

	// 特殊情况：lo（回环接口）和docker0等
	specialNames := []string{"lo", "docker0", "virbr0"}
	for _, special := range specialNames {
		if name == special {
			return false // 这些接口通常不应该被手动配置
		}
	}

	return false
}

// detectNetworkManager 检测并选择合适的网络管理器
func (h *CentOSNetworkHandler) detectNetworkManager(ctx context.Context) (types.INetworkManager, error) {
	// 按优先级检测网络管理器
	for _, manager := range h.managers {
		if manager.IsInstall(ctx) {
			utils.Info("network", fmt.Sprintf("Selected CentOS network manager: %T on %s %s", manager, h.osInfo.ID, h.osInfo.VersionID))
			return manager, nil
		}
	}

	return nil, fmt.Errorf("no suitable network manager found for CentOS %s", h.osInfo.VersionID)
}

// 简单的IP地址验证函数
func (h *CentOSNetworkHandler) isValidIP(ip string) bool {
	return h.isValidIPv4(ip) || h.isValidIPv6(ip)
}

func (h *CentOSNetworkHandler) isValidIPv4(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 3 {
			return false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}

func (h *CentOSNetworkHandler) isValidIPv6(ip string) bool {
	// 简单的IPv6格式检查
	return strings.Contains(ip, ":") && len(ip) >= 2
}

func (h *CentOSNetworkHandler) isValidIPv4CIDR(cidr string) bool {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return false
	}
	return h.isValidIPv4(parts[0]) && len(parts[1]) > 0
}

func (h *CentOSNetworkHandler) isValidIPv6CIDR(cidr string) bool {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return false
	}
	return h.isValidIPv6(parts[0]) && len(parts[1]) > 0
}

// isCentOS7Supported 检查给定的版本ID是否为支持的CentOS 7版本
func (h *CentOSNetworkHandler) isCentOS7Supported(versionID string) bool {
	if versionID == "" {
		return false
	}

	// 支持通用的 CentOS 7 版本标识
	if versionID == "7" {
		return true
	}

	// 支持的CentOS 7版本范围：7.0 到 7.9
	// 也支持带有构建号的版本，如 7.5.1804
	if strings.HasPrefix(versionID, "7.") {
		// 提取主版本号
		parts := strings.Split(versionID, ".")
		if len(parts) >= 2 {
			minorVersion := parts[1]
			// 如果有构建号，只取第一部分
			if len(parts) > 2 {
				minorVersion = parts[1]
			}
			// 检查是否在支持的范围内 (7.0 到 7.9)
			if minorVersion >= "0" && minorVersion <= "9" {
				return true
			}
		}
	}

	return false
}
