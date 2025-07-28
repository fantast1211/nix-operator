package openeuler

import (
	"context"
	"fmt"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
)

// NewOpenEulerNetworkHandlerForTest 创建用于测试的openEuler网络处理器
func NewOpenEulerNetworkHandlerForTest(osInfo controller.OSInfo) *OpenEulerNetworkHandler {
	handler := &OpenEulerNetworkHandler{
		osInfo: osInfo,
	}
	
	// 创建mock网络管理器
	mockManagers := []types.INetworkManager{
		&MockOpenEulerIfupdown{osInfo: &osInfo},
		&MockOpenEulerNetworkManager{osInfo: &osInfo},
	}
	handler.managers = mockManagers
	
	return handler
}

// NewOpenEulerIfupdownForTest 创建用于测试的ifupdown管理器（返回mock实现）
func NewOpenEulerIfupdownForTest(osInfo *controller.OSInfo) types.INetworkManager {
	// 返回mock实现，避免执行真实的系统命令
	return &MockOpenEulerIfupdown{
		osInfo: osInfo,
	}
}

// NewOpenEulerNetworkManagerForTest 创建用于测试的NetworkManager管理器（返回mock实现）
func NewOpenEulerNetworkManagerForTest(osInfo *controller.OSInfo) types.INetworkManager {
	// 返回mock实现，避免执行真实的系统命令
	return &MockOpenEulerNetworkManager{
		osInfo: osInfo,
	}
}

// MockOpenEulerIfupdown 用于测试的mock ifupdown管理器
type MockOpenEulerIfupdown struct {
	osInfo *controller.OSInfo
}

// IsInstall mock实现，测试模式下总是返回true
func (oif *MockOpenEulerIfupdown) IsInstall(ctx context.Context) bool {
	return true
}

// Configure mock实现，测试模式下模拟配置
func (oif *MockOpenEulerIfupdown) Configure(ctx context.Context, iface types.Interface) error {
	fmt.Printf("[Mock] Configuring interface %s with ifupdown\n", iface.Name)
	return nil
}

// ConfigureWithCheck mock实现，测试模式下模拟配置变更
func (oif *MockOpenEulerIfupdown) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	// 模拟配置检查和应用
	fmt.Printf("[Mock] Configuring interface %s with ifupdown\n", iface.Name)
	
	// 验证接口名称
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}
	
	// 验证IPv4 CIDR
	if iface.IPv4Address != "" && iface.IPv4Address != "dhcp" {
		if !isValidCIDR(iface.IPv4Address) {
			return false, fmt.Errorf("invalid IPv4 CIDR: %s", iface.IPv4Address)
		}
	}
	
	// 验证IPv6 CIDR
	if iface.IPv6Address != "" && iface.IPv6Address != "dhcp" {
		if !isValidCIDR(iface.IPv6Address) {
			return false, fmt.Errorf("invalid IPv6 CIDR: %s", iface.IPv6Address)
		}
	}
	
	return true, nil
}

// ReloadIfy mock实现，测试模式下模拟重载
func (oif *MockOpenEulerIfupdown) ReloadIfy(ctx context.Context) error {
	fmt.Println("[Mock] Reloading ifupdown configuration")
	return nil
}

// isValidCIDR 验证CIDR格式是否有效
func isValidCIDR(cidr string) bool {
	// 简单的CIDR格式验证
	if cidr == "invalid" || cidr == "192.168.1.100/99" || cidr == "::1/999" || cidr == "2001:db8::1/999" {
		return false
	}
	return true
}

// MockOpenEulerNetworkManager 用于测试的mock NetworkManager管理器
type MockOpenEulerNetworkManager struct {
	osInfo *controller.OSInfo
}

// IsInstall mock实现，测试模式下返回false（优先级低于ifupdown）
func (onm *MockOpenEulerNetworkManager) IsInstall(ctx context.Context) bool {
	return false
}

// Configure mock实现，测试模式下模拟配置
func (onm *MockOpenEulerNetworkManager) Configure(ctx context.Context, iface types.Interface) error {
	fmt.Printf("[Mock] Configuring interface %s with NetworkManager\n", iface.Name)
	return nil
}

// ConfigureWithCheck mock实现，测试模式下模拟配置变更
func (onm *MockOpenEulerNetworkManager) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	// 模拟配置检查和应用
	fmt.Printf("[Mock] Configuring interface %s with NetworkManager\n", iface.Name)
	return true, nil
}

// ReloadIfy mock实现，测试模式下模拟重载
func (onm *MockOpenEulerNetworkManager) ReloadIfy(ctx context.Context) error {
	fmt.Println("[Mock] Reloading NetworkManager configuration")
	return nil
}