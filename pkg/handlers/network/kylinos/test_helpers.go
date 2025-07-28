package kylinos

import (
	"context"
	"fmt"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
)

// NewKylinOSNetworkHandlerForTest 创建用于测试的KylinOS网络处理器
func NewKylinOSNetworkHandlerForTest(osInfo controller.OSInfo) *KylinOSNetworkHandler {
	handler := &KylinOSNetworkHandler{
		osInfo: osInfo,
	}
	
	// 创建mock网络管理器
	mockManagers := []types.INetworkManager{
		&MockKylinOSIfupdown{osInfo: &osInfo},
		&MockKylinOSNetworkManager{osInfo: &osInfo},
		&MockKylinOSNetplan{osInfo: &osInfo},
	}
	handler.managers = mockManagers
	
	return handler
}

// NewKylinOSIfupdownForTest 创建用于测试的ifupdown管理器（返回mock实现）
func NewKylinOSIfupdownForTest(osInfo *controller.OSInfo) types.INetworkManager {
	// 返回mock实现，避免执行真实的系统命令
	return &MockKylinOSIfupdown{
		osInfo: osInfo,
	}
}

// NewKylinOSNetworkManagerForTest 创建用于测试的NetworkManager管理器（返回mock实现）
func NewKylinOSNetworkManagerForTest(osInfo *controller.OSInfo) types.INetworkManager {
	// 返回mock实现，避免执行真实的系统命令
	return &MockKylinOSNetworkManager{
		osInfo: osInfo,
	}
}

// NewKylinOSNetplanForTest 创建用于测试的Netplan管理器（返回mock实现）
func NewKylinOSNetplanForTest(osInfo *controller.OSInfo) types.INetworkManager {
	// 返回mock实现，避免执行真实的系统命令
	return &MockKylinOSNetplan{
		osInfo: osInfo,
	}
}

// MockKylinOSIfupdown 用于测试的mock ifupdown管理器
type MockKylinOSIfupdown struct {
	osInfo *controller.OSInfo
}

// IsInstall mock实现，测试模式下总是返回true
func (kif *MockKylinOSIfupdown) IsInstall(ctx context.Context) bool {
	return true
}

// Configure mock实现，测试模式下模拟配置
func (kif *MockKylinOSIfupdown) Configure(ctx context.Context, iface types.Interface) error {
	fmt.Printf("[Mock] Configuring interface %s with ifupdown\n", iface.Name)
	return nil
}

// ConfigureWithCheck mock实现，测试模式下模拟配置变更
func (kif *MockKylinOSIfupdown) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	// 模拟配置检查和应用
	fmt.Printf("[Mock] Configuring interface %s with ifupdown\n", iface.Name)
	return true, nil
}

// ReloadIfy mock实现，测试模式下模拟重载
func (kif *MockKylinOSIfupdown) ReloadIfy(ctx context.Context) error {
	fmt.Println("[Mock] Reloading ifupdown configuration")
	return nil
}

// MockKylinOSNetworkManager 用于测试的mock NetworkManager管理器
type MockKylinOSNetworkManager struct {
	osInfo *controller.OSInfo
}

// IsInstall mock实现，测试模式下返回true（但优先级低于ifupdown）
func (nm *MockKylinOSNetworkManager) IsInstall(ctx context.Context) bool {
	return true
}

// Configure mock实现，测试模式下模拟配置
func (nm *MockKylinOSNetworkManager) Configure(ctx context.Context, iface types.Interface) error {
	fmt.Printf("[Mock] Configuring interface %s with NetworkManager\n", iface.Name)
	return nil
}

// ConfigureWithCheck mock实现，测试模式下模拟配置变更
func (nm *MockKylinOSNetworkManager) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	// 模拟配置检查和应用
	fmt.Printf("[Mock] Configuring interface %s with NetworkManager\n", iface.Name)
	return true, nil
}

// ReloadIfy mock实现，测试模式下模拟重载
func (nm *MockKylinOSNetworkManager) ReloadIfy(ctx context.Context) error {
	fmt.Println("[Mock] Reloading NetworkManager configuration")
	return nil
}

// MockKylinOSNetplan 用于测试的mock Netplan管理器
type MockKylinOSNetplan struct {
	osInfo *controller.OSInfo
}

// IsInstall mock实现，测试模式下返回true
func (np *MockKylinOSNetplan) IsInstall(ctx context.Context) bool {
	return true
}

// Configure mock实现，测试模式下模拟配置
func (np *MockKylinOSNetplan) Configure(ctx context.Context, iface types.Interface) error {
	fmt.Printf("[Mock] Configuring interface %s with Netplan\n", iface.Name)
	return nil
}

// ConfigureWithCheck mock实现，测试模式下模拟配置变更
func (np *MockKylinOSNetplan) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	// 模拟配置检查和应用
	fmt.Printf("[Mock] Configuring interface %s with Netplan\n", iface.Name)
	return true, nil
}

// ReloadIfy mock实现，测试模式下模拟重载
func (np *MockKylinOSNetplan) ReloadIfy(ctx context.Context) error {
	fmt.Println("[Mock] Reloading Netplan configuration")
	return nil
}