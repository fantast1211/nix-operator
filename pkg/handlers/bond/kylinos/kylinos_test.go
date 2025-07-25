package kylinos

import (
	"context"
	"testing"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"google.golang.org/protobuf/types/known/anypb"
)

// TestKylinOSBondHandler_Match 测试KylinOS Bond处理器匹配
func TestKylinOSBondHandler_Match(t *testing.T) {
	tests := []struct {
		name     string
		osInfo   controller.OSInfo
		expected bool
	}{
		{
			name: "Valid KylinOS V10",
			osInfo: controller.OSInfo{
				ID:         "kylin",
				VersionID:  "V10",
				KernelName: "Linux",
			},
			expected: true,
		},
		{
			name: "Valid KylinOS 10",
			osInfo: controller.OSInfo{
				ID:         "kylin",
				VersionID:  "10",
				KernelName: "Linux",
			},
			expected: true,
		},
		{
			name: "Invalid OS ID",
			osInfo: controller.OSInfo{
				ID:         "ubuntu",
				VersionID:  "V10",
				KernelName: "Linux",
			},
			expected: false,
		},
		{
			name: "Invalid Kernel",
			osInfo: controller.OSInfo{
				ID:         "kylin",
				VersionID:  "V10",
				KernelName: "Windows",
			},
			expected: false,
		},
		{
			name: "Unsupported Version",
			osInfo: controller.OSInfo{
				ID:         "kylin",
				VersionID:  "V9",
				KernelName: "Linux",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewKylinOSBondHandler()
			result := handler.Match(tt.osInfo)
			if result != tt.expected {
				t.Errorf("Match() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestKylinOSBondHandler_validateKylinOSBondConfig 测试KylinOS Bond配置验证
func TestKylinOSBondHandler_validateKylinOSBondConfig(t *testing.T) {
	handler := &KylinOSBondHandler{}

	tests := []struct {
		name      string
		bondSpec  *systemv1.BondConfigurationSpec
		expectErr bool
	}{
		{
			name: "Valid Bond Config",
			bondSpec: &systemv1.BondConfigurationSpec{
				Name:   "bond0",
				Mode:   1,
				Miimon: 100,
				Network: &systemv1.BondNetworkConfig{
					Ip:      "192.168.1.100/24",
					Gateway: "192.168.1.1",
					Mtu:     1500,
				},
			},
			expectErr: false,
		},
		{
			name:      "Nil Bond Spec",
			bondSpec:  nil,
			expectErr: true,
		},
		{
			name: "Empty Bond Name",
			bondSpec: &systemv1.BondConfigurationSpec{
				Name:   "",
				Mode:   1,
				Miimon: 100,
			},
			expectErr: true,
		},
		{
			name: "Invalid Bond Mode",
			bondSpec: &systemv1.BondConfigurationSpec{
				Name:   "bond0",
				Mode:   8,
				Miimon: 100,
			},
			expectErr: true,
		},
		{
			name: "Negative Miimon",
			bondSpec: &systemv1.BondConfigurationSpec{
				Name:   "bond0",
				Mode:   1,
				Miimon: -1,
			},
			expectErr: true,
		},
		{
			name: "Invalid IP CIDR",
			bondSpec: &systemv1.BondConfigurationSpec{
				Name:   "bond0",
				Mode:   1,
				Miimon: 100,
				Network: &systemv1.BondNetworkConfig{
					Ip: "invalid-ip",
				},
			},
			expectErr: true,
		},
		{
			name: "Invalid Gateway IP",
			bondSpec: &systemv1.BondConfigurationSpec{
				Name:   "bond0",
				Mode:   1,
				Miimon: 100,
				Network: &systemv1.BondNetworkConfig{
					Ip:      "192.168.1.100/24",
					Gateway: "invalid-gateway",
				},
			},
			expectErr: true,
		},
		{
			name: "Invalid MTU",
			bondSpec: &systemv1.BondConfigurationSpec{
				Name:   "bond0",
				Mode:   1,
				Miimon: 100,
				Network: &systemv1.BondNetworkConfig{
					Ip:  "192.168.1.100/24",
					Mtu: 10000,
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateKylinOSBondConfig(tt.bondSpec)
			if (err != nil) != tt.expectErr {
				t.Errorf("validateKylinOSBondConfig() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestKylinOSBondIfupdown_Configure 测试KylinOS Bond ifupdown配置
func TestKylinOSBondIfupdown_Configure(t *testing.T) {
	osInfo := &controller.OSInfo{
		ID:        "kylin",
		VersionID: "V10",
	}

	bondManager := NewKylinOSBondIfupdownForTest(osInfo)
	ctx := context.Background()

	tests := []struct {
		name       string
		bondConfig types.BondConfig
		expectErr  bool
	}{
		{
			name: "Valid Bond Configuration",
			bondConfig: types.BondConfig{
				Name:   "bond0",
				Mode:   1,
				Miimon: 100,
				Network: types.BondNetworkConfig{
					IP:         "192.168.1.100/24",
					Gateway:    "192.168.1.1",
					DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					MTU:        1500,
				},
			},
			expectErr: false,
		},
		{
			name: "Empty Bond Name Should Fail",
			bondConfig: types.BondConfig{
				Name:   "",
				Mode:   1,
				Miimon: 100,
			},
			expectErr: true,
		},
		{
			name: "Invalid IPv4 CIDR Should Fail",
			bondConfig: types.BondConfig{
				Name:   "bond0",
				Mode:   1,
				Miimon: 100,
				Network: types.BondNetworkConfig{
					IP: "invalid-ip",
				},
			},
			expectErr: true,
		},
		{
			name: "DHCP Configuration",
			bondConfig: types.BondConfig{
				Name:   "bond1",
				Mode:   0,
				Miimon: 100,
				Network: types.BondNetworkConfig{
					// 空IP表示使用DHCP
					IP: "",
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, err := bondManager.ConfigureWithCheck(ctx, tt.bondConfig)
			if (err != nil) != tt.expectErr {
				t.Errorf("ConfigureWithCheck() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if !tt.expectErr && !changed {
				t.Errorf("ConfigureWithCheck() changed = %v, expected true for valid config", changed)
			}
		})
	}
}

// TestKylinOSBondHandler_Reconcile 测试KylinOS Bond调谐
func TestKylinOSBondHandler_Reconcile(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:        "kylin",
		VersionID: "V10",
	}

	handler := NewKylinOSBondHandlerForTest(osInfo)
	ctx := context.Background()

	// 创建测试配置
	bondSpec := &systemv1.BondConfigurationSpec{
		Name:   "bond0",
		Mode:   1,
		Miimon: 100,
		Network: &systemv1.BondNetworkConfig{
			Ip:         "192.168.1.100/24",
			Gateway:    "192.168.1.1",
			DnsServers: []string{"8.8.8.8"},
			Mtu:        1500,
		},
	}

	bondSpecAny, err := anypb.New(bondSpec)
	if err != nil {
		t.Fatalf("Failed to create Any from bondSpec: %v", err)
	}

	config := &systemv1.ResourceConfig{
		Metadata: &systemv1.Metadata{
			Name: "test-bond",
		},
		Spec: bondSpecAny,
	}

	_, err = handler.Reconcile(ctx, config)
	if err != nil {
		t.Errorf("Reconcile() error = %v", err)
		return
	}

	// 验证调谐成功完成
	t.Logf("Reconcile completed successfully")
}

// TestKylinOSBondManagers_IsAvailable 测试KylinOS Bond管理器可用性
func TestKylinOSBondManagers_IsAvailable(t *testing.T) {
	osInfo := &controller.OSInfo{
		ID:        "kylin",
		VersionID: "V10",
	}

	ctx := context.Background()

	tests := []struct {
		name    string
		manager types.IBondManager
	}{
		{
			name:    "KylinOS Bond Ifupdown",
			manager: NewKylinOSBondIfupdownForTest(osInfo),
		},
		{
			name:    "KylinOS Bond NetworkManager",
			manager: NewKylinOSBondNetworkManagerForTest(osInfo),
		},
		{
			name:    "KylinOS Bond Netplan",
			manager: NewKylinOSBondNetplanForTest(osInfo),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			available := tt.manager.IsInstall(ctx)
			if !available {
				t.Errorf("%s.IsInstall() = false, expected true in test mode", tt.name)
			}
		})
	}
}
