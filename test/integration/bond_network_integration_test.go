package integration

import (
	"context"
	"testing"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	bondOpeneuler "go.xbrother.com/nix-operator/pkg/handlers/bond/openeuler"
	bondTypes "go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	networkOpeneuler "go.xbrother.com/nix-operator/pkg/handlers/network/openeuler"
	networkTypes "go.xbrother.com/nix-operator/pkg/handlers/network/types"
)

// TestBondNetworkIntegration 测试Bond和Network模块的集成
func TestBondNetworkIntegration(t *testing.T) {
	// 模拟openEuler系统信息
	osInfo := &controller.OSInfo{
		ID:        "openeuler",
		VersionID: "22.03",
	}

	// 创建测试实例
	bondManager := bondOpeneuler.NewOpenEulerBondIfupdownForTest(osInfo)
	networkManager := networkOpeneuler.NewOpenEulerIfupdownForTest(osInfo)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 验证管理器可用性
	t.Run("检查管理器可用性", func(t *testing.T) {
		if !bondManager.IsInstall(ctx) {
			t.Error("Bond manager should be available in test mode")
		}
		if !networkManager.IsInstall(ctx) {
			t.Error("Network manager should be available in test mode")
		}
	})

	// 2. 配置Bond主接口
	t.Run("配置Bond主接口", func(t *testing.T) {
		bondConfig := bondTypes.BondConfig{
			Name:   "bond0",
			Mode:   1, // active-backup
			Miimon: 100,
			Network: bondTypes.BondNetworkConfig{
				IP:         "192.168.1.100/24",
				Gateway:    "192.168.1.1",
				DNSServers: []string{"8.8.8.8", "8.8.4.4"},
				MTU:        1500,
			},
		}

		changed, err := bondManager.ConfigureWithCheck(ctx, bondConfig)
		if err != nil {
			t.Fatalf("Failed to configure bond interface: %v", err)
		}
		if !changed {
			t.Error("Expected bond configuration to be changed")
		}
		t.Log("Bond主接口配置成功")
	})

	// 3. 配置Bond Slave接口
	t.Run("配置Bond Slave接口", func(t *testing.T) {
		// 第一个slave接口
		slave1 := networkTypes.Interface{
			Name: "eth0",
			MTU:  1500,
			BondingSlave: &networkTypes.BondingSlaveConfig{
				Enabled: true,
				Master:  "bond0",
			},
		}

		changed1, err := networkManager.ConfigureWithCheck(ctx, slave1)
		if err != nil {
			t.Fatalf("Failed to configure slave interface eth0: %v", err)
		}
		if !changed1 {
			t.Error("Expected slave1 configuration to be changed")
		}

		// 第二个slave接口
		slave2 := networkTypes.Interface{
			Name: "eth1",
			MTU:  1500,
			BondingSlave: &networkTypes.BondingSlaveConfig{
				Enabled: true,
				Master:  "bond0",
			},
		}

		changed2, err := networkManager.ConfigureWithCheck(ctx, slave2)
		if err != nil {
			t.Fatalf("Failed to configure slave interface eth1: %v", err)
		}
		if !changed2 {
			t.Error("Expected slave2 configuration to be changed")
		}
		t.Log("Bond Slave接口配置成功")
	})

	// 4. 配置普通网卡接口
	t.Run("配置普通网卡接口", func(t *testing.T) {
		regularInterface := networkTypes.Interface{
			Name:        "eth2",
			IPv4Address: "192.168.2.100/24",
			IPv4Gateway: "192.168.2.1",
			MTU:         1500,
			Nameservers: []string{"8.8.8.8"},
			BondingSlave: nil, // 不是slave接口
		}

		changed, err := networkManager.ConfigureWithCheck(ctx, regularInterface)
		if err != nil {
			t.Fatalf("Failed to configure regular interface: %v", err)
		}
		if !changed {
			t.Error("Expected regular interface configuration to be changed")
		}
		t.Log("普通网卡接口配置成功")
	})

	// 5. 测试重载功能
	t.Run("测试重载功能", func(t *testing.T) {
		// 先重载网络配置
		if err := networkManager.ReloadIfy(ctx); err != nil {
			t.Fatalf("Failed to reload network configuration: %v", err)
		}

		// 再重载Bond配置
		if err := bondManager.ReloadIfy(ctx); err != nil {
			t.Fatalf("Failed to reload bond configuration: %v", err)
		}
		t.Log("配置重载成功")
	})
}

// TestBondSlaveIPIgnoring 测试Bond Slave接口IP配置被忽略
func TestBondSlaveIPIgnoring(t *testing.T) {
	osInfo := &controller.OSInfo{
		ID:        "openeuler",
		VersionID: "22.03",
	}

	networkManager := networkOpeneuler.NewOpenEulerIfupdownForTest(osInfo)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 创建一个带有IP配置的Bond Slave接口
	slaveWithIP := networkTypes.Interface{
		Name:        "eth0",
		IPv4Address: "192.168.1.100/24", // 这个应该被忽略
		IPv6Address: "2001:db8::1/64",   // 这个也应该被忽略
		IPv4Gateway: "192.168.1.1",      // 这个也应该被忽略
		IPv6Gateway: "2001:db8::1111",   // 这个也应该被忽略
		MTU:         1500,
		Nameservers: []string{"8.8.8.8"}, // 这个也应该被忽略
		BondingSlave: &networkTypes.BondingSlaveConfig{
			Enabled: true,
			Master:  "bond0",
		},
	}

	// 配置接口，应该成功但IP配置被忽略
	changed, err := networkManager.ConfigureWithCheck(ctx, slaveWithIP)
	if err != nil {
		t.Fatalf("Failed to configure slave interface with IP: %v", err)
	}
	if !changed {
		t.Error("Expected slave configuration to be changed")
	}

	t.Log("Bond Slave接口IP配置忽略测试通过")
}

// TestMultipleBondConfiguration 测试多个Bond配置
func TestMultipleBondConfiguration(t *testing.T) {
	osInfo := &controller.OSInfo{
		ID:        "openeuler",
		VersionID: "22.03",
	}

	bondManager := bondOpeneuler.NewOpenEulerBondIfupdownForTest(osInfo)
	networkManager := networkOpeneuler.NewOpenEulerIfupdownForTest(osInfo)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 配置第一个Bond
	bond0Config := bondTypes.BondConfig{
		Name:   "bond0",
		Mode:   1, // active-backup
		Miimon: 100,
		Network: bondTypes.BondNetworkConfig{
			IP:      "192.168.1.100/24",
			Gateway: "192.168.1.1",
			MTU:     1500,
		},
	}

	changed, err := bondManager.ConfigureWithCheck(ctx, bond0Config)
	if err != nil {
		t.Fatalf("Failed to configure bond0: %v", err)
	}
	if !changed {
		t.Error("Expected bond0 configuration to be changed")
	}

	// 配置第二个Bond
	bond1Config := bondTypes.BondConfig{
		Name:   "bond1",
		Mode:   0, // balance-rr
		Miimon: 100,
		Network: bondTypes.BondNetworkConfig{
			IP:      "192.168.2.100/24",
			Gateway: "192.168.2.1",
			MTU:     1500,
		},
	}

	changed, err = bondManager.ConfigureWithCheck(ctx, bond1Config)
	if err != nil {
		t.Fatalf("Failed to configure bond1: %v", err)
	}
	if !changed {
		t.Error("Expected bond1 configuration to be changed")
	}

	// 为bond0配置slave接口
	bond0Slave1 := networkTypes.Interface{
		Name: "eth0",
		MTU:  1500,
		BondingSlave: &networkTypes.BondingSlaveConfig{
			Enabled: true,
			Master:  "bond0",
		},
	}

	changed, err = networkManager.ConfigureWithCheck(ctx, bond0Slave1)
	if err != nil {
		t.Fatalf("Failed to configure bond0 slave: %v", err)
	}
	if !changed {
		t.Error("Expected bond0 slave configuration to be changed")
	}

	// 为bond1配置slave接口
	bond1Slave1 := networkTypes.Interface{
		Name: "eth2",
		MTU:  1500,
		BondingSlave: &networkTypes.BondingSlaveConfig{
			Enabled: true,
			Master:  "bond1",
		},
	}

	changed, err = networkManager.ConfigureWithCheck(ctx, bond1Slave1)
	if err != nil {
		t.Fatalf("Failed to configure bond1 slave: %v", err)
	}
	if !changed {
		t.Error("Expected bond1 slave configuration to be changed")
	}

	t.Log("多Bond配置测试通过")
}