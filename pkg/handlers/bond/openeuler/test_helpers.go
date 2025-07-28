package openeuler

import (
	"context"
	"fmt"
	"net"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// NewOpenEulerBondHandlerForTest 创建用于测试的openEuler Bond处理器
func NewOpenEulerBondHandlerForTest(osInfo controller.OSInfo) *OpenEulerBondHandler {
	handler := &OpenEulerBondHandler{
		osInfo: osInfo,
	}
	return handler
}

// NewOpenEulerBondIfupdownForTest 创建用于测试的ifupdown Bond管理器（返回mock实现）
func NewOpenEulerBondIfupdownForTest(osInfo *controller.OSInfo) types.IBondManager {
	return &MockOpenEulerBondIfupdown{
		osInfo: osInfo,
	}
}

// NewOpenEulerBondNetworkManagerForTest 创建用于测试的NetworkManager Bond管理器（返回mock实现）
func NewOpenEulerBondNetworkManagerForTest(osInfo *controller.OSInfo) types.IBondManager {
	return &MockOpenEulerBondNetworkManager{
		osInfo: osInfo,
	}
}

// MockOpenEulerBondIfupdown mock实现，用于测试
type MockOpenEulerBondIfupdown struct {
	osInfo *controller.OSInfo
}

func (m *MockOpenEulerBondIfupdown) IsInstall(ctx context.Context) bool {
	utils.Info("bond", "[TEST MODE] openEuler bond ifupdown tools detected")
	return true
}

func (m *MockOpenEulerBondIfupdown) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	// 验证Bond名称
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 验证IPv4地址格式
	if bondConfig.Network.IP != "" {
		if _, _, err := net.ParseCIDR(bondConfig.Network.IP); err != nil {
			return false, fmt.Errorf("invalid IPv4 CIDR format: %s", bondConfig.Network.IP)
		}
	}

	// 模拟配置逻辑：总是返回配置已变更
	utils.Infof("bond", "[TEST MODE] openEuler bond ifcfg configuration simulated for bond %s", bondConfig.Name)
	return true, nil
}

func (m *MockOpenEulerBondIfupdown) ReloadIfy(ctx context.Context) error {
	// 模拟重载逻辑：总是成功
	utils.Info("bond", "[TEST MODE] openEuler bond network service restart simulated")
	return nil
}

func (m *MockOpenEulerBondIfupdown) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := m.ConfigureWithCheck(ctx, bondConfig)
	return err
}

// MockOpenEulerBondNetworkManager mock实现，用于测试
type MockOpenEulerBondNetworkManager struct {
	osInfo *controller.OSInfo
}

func (m *MockOpenEulerBondNetworkManager) IsInstall(ctx context.Context) bool {
	utils.Info("bond", "[TEST MODE] openEuler bond NetworkManager detected")
	return true
}

func (m *MockOpenEulerBondNetworkManager) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	// 验证Bond名称
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 验证IPv4地址格式
	if bondConfig.Network.IP != "" {
		if _, _, err := net.ParseCIDR(bondConfig.Network.IP); err != nil {
			return false, fmt.Errorf("invalid IPv4 CIDR format: %s", bondConfig.Network.IP)
		}
	}

	// 模拟配置逻辑：总是返回配置已变更
	utils.Infof("bond", "[TEST MODE] openEuler bond NetworkManager configuration simulated for bond %s", bondConfig.Name)
	return true, nil
}

func (m *MockOpenEulerBondNetworkManager) ReloadIfy(ctx context.Context) error {
	// 模拟重载逻辑：总是成功
	utils.Info("bond", "[TEST MODE] openEuler bond NetworkManager restart simulated")
	return nil
}

func (m *MockOpenEulerBondNetworkManager) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := m.ConfigureWithCheck(ctx, bondConfig)
	return err
}