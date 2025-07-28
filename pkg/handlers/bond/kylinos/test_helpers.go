package kylinos

import (
	"context"
	"fmt"
	"net"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// NewKylinOSBondHandlerForTest 创建用于测试的KylinOS Bond处理器
func NewKylinOSBondHandlerForTest(osInfo controller.OSInfo) *KylinOSBondHandler {
	handler := &KylinOSBondHandler{
		osInfo: osInfo,
	}
	return handler
}

// NewKylinOSBondIfupdownForTest 创建用于测试的ifupdown Bond管理器（返回mock实现）
func NewKylinOSBondIfupdownForTest(osInfo *controller.OSInfo) types.IBondManager {
	return &MockKylinOSBondIfupdown{
		osInfo: osInfo,
	}
}

// NewKylinOSBondNetworkManagerForTest 创建用于测试的NetworkManager Bond管理器（返回mock实现）
func NewKylinOSBondNetworkManagerForTest(osInfo *controller.OSInfo) types.IBondManager {
	return &MockKylinOSBondNetworkManager{
		osInfo: osInfo,
	}
}

// NewKylinOSBondNetplanForTest 创建用于测试的Netplan Bond管理器（返回mock实现）
func NewKylinOSBondNetplanForTest(osInfo *controller.OSInfo) types.IBondManager {
	return &MockKylinOSBondNetplan{
		osInfo: osInfo,
	}
}

// MockKylinOSBondIfupdown mock实现，用于测试
type MockKylinOSBondIfupdown struct {
	osInfo *controller.OSInfo
}

func (m *MockKylinOSBondIfupdown) IsInstall(ctx context.Context) bool {
	utils.Info("bond", "[TEST MODE] KylinOS bond ifupdown tools detected")
	return true
}

func (m *MockKylinOSBondIfupdown) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
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

	utils.Infof("bond", "[TEST MODE] KylinOS bond ifupdown configuration simulated for bond %s", bondConfig.Name)
	return true, nil
}

func (m *MockKylinOSBondIfupdown) ReloadIfy(ctx context.Context) error {
	utils.Info("bond", "[TEST MODE] KylinOS bond ifupdown restart simulated")
	return nil
}

func (m *MockKylinOSBondIfupdown) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := m.ConfigureWithCheck(ctx, bondConfig)
	return err
}

// MockKylinOSBondNetworkManager mock实现，用于测试
type MockKylinOSBondNetworkManager struct {
	osInfo *controller.OSInfo
}

func (m *MockKylinOSBondNetworkManager) IsInstall(ctx context.Context) bool {
	utils.Info("bond", "[TEST MODE] KylinOS bond NetworkManager detected")
	return true
}

func (m *MockKylinOSBondNetworkManager) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
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

	utils.Infof("bond", "[TEST MODE] KylinOS bond NetworkManager configuration simulated for bond %s", bondConfig.Name)
	return true, nil
}

func (m *MockKylinOSBondNetworkManager) ReloadIfy(ctx context.Context) error {
	utils.Info("bond", "[TEST MODE] KylinOS bond NetworkManager restart simulated")
	return nil
}

func (m *MockKylinOSBondNetworkManager) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := m.ConfigureWithCheck(ctx, bondConfig)
	return err
}

// MockKylinOSBondNetplan mock实现，用于测试
type MockKylinOSBondNetplan struct {
	osInfo *controller.OSInfo
}

func (m *MockKylinOSBondNetplan) IsInstall(ctx context.Context) bool {
	utils.Info("bond", "[TEST MODE] KylinOS bond Netplan detected")
	return true
}

func (m *MockKylinOSBondNetplan) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
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

	utils.Infof("bond", "[TEST MODE] KylinOS bond Netplan configuration simulated for bond %s", bondConfig.Name)
	return true, nil
}

func (m *MockKylinOSBondNetplan) ReloadIfy(ctx context.Context) error {
	utils.Info("bond", "[TEST MODE] KylinOS bond Netplan restart simulated")
	return nil
}

func (m *MockKylinOSBondNetplan) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := m.ConfigureWithCheck(ctx, bondConfig)
	return err
}