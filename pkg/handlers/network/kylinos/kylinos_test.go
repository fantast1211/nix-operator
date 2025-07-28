package kylinos

import (
	"context"
	"net"
	"testing"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
	"google.golang.org/protobuf/types/known/anypb"
)

// TestKylinOSNetworkHandler_Match 测试 KylinOS 系统匹配
func TestKylinOSNetworkHandler_Match(t *testing.T) {
	tests := []struct {
		name     string
		osInfo   controller.OSInfo
		expected bool
	}{
		{
			name: "valid_kylin_os",
			osInfo: controller.OSInfo{
				ID:         "kylin",
				VersionID:  "V10",
				KernelName: "Linux",
				KernelVer:  "4.19.0",
			},
			expected: true,
		},
		{
			name: "invalid_os_ubuntu",
			osInfo: controller.OSInfo{
				ID:         "ubuntu",
				VersionID:  "20.04",
				KernelName: "Linux",
				KernelVer:  "5.4.0",
			},
			expected: false,
		},
		{
			name: "invalid_os_centos",
			osInfo: controller.OSInfo{
				ID:         "centos",
				VersionID:  "7",
				KernelName: "Linux",
				KernelVer:  "3.10.0",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewKylinOSNetworkHandler()
			result := handler.Match(tt.osInfo)
			if result != tt.expected {
				t.Errorf("Match() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestKylinOSNetworkHandler_detectNetworkManager 测试网络管理器检测
func TestKylinOSNetworkHandler_detectNetworkManager(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "kylin",
		VersionID:  "V10",
		KernelName: "Linux",
		KernelVer:  "4.19.0",
	}

	handler := NewKylinOSNetworkHandlerForTest(osInfo)
	ctx := context.Background()

	// 测试网络管理器检测
	manager, err := handler.detectNetworkManager(ctx)
	if err != nil {
		t.Fatalf("detectNetworkManager() error = %v", err)
	}

	if manager == nil {
		t.Fatal("detectNetworkManager() returned nil manager")
	}

	// 验证返回的是 ifupdown（优先级最高）
	if _, ok := manager.(*MockKylinOSIfupdown); !ok {
		t.Errorf("Expected MockKylinOSIfupdown manager, got %T", manager)
	}
}

// TestKylinOSNetworkHandler_Reconcile 测试网络配置调谐
func TestKylinOSNetworkHandler_Reconcile(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "kylin",
		VersionID:  "V10",
		KernelName: "Linux",
		KernelVer:  "4.19.0",
	}

	handler := NewKylinOSNetworkHandlerForTest(osInfo)
	ctx := context.Background()

	// 创建测试配置
	testConfigs := []*systemv1.ResourceConfig{
		{
			Metadata: &systemv1.Metadata{
				Name: "test-kylin-network",
			},
			Spec: func() *anypb.Any {
				spec := &systemv1.NetworkConfigurationSpec{
					Interfaces: []*systemv1.NetworkInterface{
						{
							Name:        "eth0",
							Ipv4Address: "192.168.1.100/24",
							Ipv4Gateway: "192.168.1.1",
							Mtu:         1500,
							Nameservers: []string{"8.8.8.8", "8.8.4.4"},
						},
					},
				}
				specData, _ := marshalSpec(spec)
				return specData
			}(),
		},
	}

	// 执行调谐
	_, err := handler.Reconcile(ctx, testConfigs[0])
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// 验证调谐成功完成
	t.Logf("Reconcile completed successfully")
}

// TestKylinOSNetworkHandler_ReconcileWithBondSlave 测试Bond从设备配置调谐
func TestKylinOSNetworkHandler_ReconcileWithBondSlave(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "kylin",
		VersionID:  "V10",
		KernelName: "Linux",
		KernelVer:  "4.19.0",
	}

	handler := NewKylinOSNetworkHandlerForTest(osInfo)
	ctx := context.Background()

	// 创建Bond从设备测试配置
	testConfigs := []*systemv1.ResourceConfig{
		{
			Metadata: &systemv1.Metadata{
				Name: "test-kylin-bond-slave",
			},
			Spec: func() *anypb.Any {
				spec := &systemv1.NetworkConfigurationSpec{
					Interfaces: []*systemv1.NetworkInterface{
						{
							Name: "eth0",
							BondingSlave: &systemv1.BondingSlaveConfig{
								Enabled: true,
								Master:  "bond0",
							},
							Mtu: 1500,
						},
						{
							Name: "eth1",
							BondingSlave: &systemv1.BondingSlaveConfig{
								Enabled: true,
								Master:  "bond0",
							},
							Mtu: 1500,
						},
					},
				}
				specData, _ := marshalSpec(spec)
				return specData
			}(),
		},
	}

	// 执行调谐
	_, err := handler.Reconcile(ctx, testConfigs[0])
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// 验证调谐成功完成
	t.Logf("Reconcile completed successfully")
}

// TestValidateKylinOSInterface 测试接口验证
func TestValidateKylinOSInterface(t *testing.T) {
	handler := &KylinOSNetworkHandler{}

	tests := []struct {
		name      string
		iface     *systemv1.NetworkInterface
		expectErr bool
	}{
		{
			name: "valid_interface",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100/24",
				Ipv4Gateway: "192.168.1.1",
				Mtu:         1500,
				Nameservers: []string{"8.8.8.8"},
			},
			expectErr: false,
		},
		{
			name: "valid_interface_with_ipv6",
			iface: &systemv1.NetworkInterface{
				Name:        "eth1",
				Ipv4Address: "10.0.0.100/8",
				Ipv6Address: "2001:db8::1/64",
				Ipv4Gateway: "10.0.0.1",
				Ipv6Gateway: "2001:db8::1",
				Mtu:         9000,
			},
			expectErr: false,
		},
		{
			name: "invalid_interface_-_empty_name",
			iface: &systemv1.NetworkInterface{
				Name:        "",
				Ipv4Address: "192.168.1.100/24",
			},
			expectErr: true,
		},
		{
			name: "invalid_interface_-_invalid_name",
			iface: &systemv1.NetworkInterface{
				Name:        "123invalid",
				Ipv4Address: "192.168.1.100/24",
			},
			expectErr: true,
		},
		{
			name: "invalid_interface_-_invalid_IPv4",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv4Address: "invalid-ip",
			},
			expectErr: true,
		},
		{
			name: "invalid_interface_-_invalid_IPv6",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv6Address: "invalid-ipv6",
			},
			expectErr: true,
		},
		{
			name: "invalid_interface_-_invalid_MTU_low",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100/24",
				Mtu:         50,
			},
			expectErr: true,
		},
		{
			name: "invalid_interface_-_invalid_MTU_high",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100/24",
				Mtu:         10000,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateKylinOSInterface(tt.iface)
			if (err != nil) != tt.expectErr {
				t.Errorf("validateKylinOSInterface() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestKylinOSIfupdown 测试 ifupdown 网络管理器
func TestKylinOSIfupdown(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "kylin",
		VersionID:  "V10",
		KernelName: "Linux",
		KernelVer:  "4.19.0",
	}

	ifupdown := NewKylinOSIfupdownForTest(&osInfo)
	ctx := context.Background()

	// 测试 IsInstall
	if !ifupdown.IsInstall(ctx) {
		t.Error("IsInstall() should return true in test mode")
	}

	// 测试 ConfigureWithCheck - 普通接口
	iface := types.Interface{
		Name:        "eth0",
		IPv4Address: "192.168.1.100/24",
		IPv4Gateway: "192.168.1.1",
		MTU:         1500,
		Nameservers: []string{"8.8.8.8"},
	}

	changed, err := ifupdown.ConfigureWithCheck(ctx, iface)
	if err != nil {
		t.Fatalf("ConfigureWithCheck() error = %v", err)
	}

	if !changed {
		t.Error("ConfigureWithCheck() should return true in test mode")
	}

	// 测试 ConfigureWithCheck - Bond从设备
	bondSlaveIface := types.Interface{
		Name: "eth1",
		MTU:  1500,
		BondingSlave: &types.BondingSlaveConfig{
			Enabled: true,
			Master:  "bond0",
		},
	}

	changed, err = ifupdown.ConfigureWithCheck(ctx, bondSlaveIface)
	if err != nil {
		t.Fatalf("ConfigureWithCheck() for bond slave error = %v", err)
	}

	if !changed {
		t.Error("ConfigureWithCheck() for bond slave should return true in test mode")
	}

	// 测试 ReloadIfy
	if err := ifupdown.ReloadIfy(ctx); err != nil {
		t.Fatalf("ReloadIfy() error = %v", err)
	}
}

// TestKylinOSNetworkManager 测试 NetworkManager 网络管理器
func TestKylinOSNetworkManager(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "kylin",
		VersionID:  "V10",
		KernelName: "Linux",
		KernelVer:  "4.19.0",
	}

	nm := NewKylinOSNetworkManagerForTest(&osInfo)
	ctx := context.Background()

	// 测试 IsInstall
	if !nm.IsInstall(ctx) {
		t.Error("IsInstall() should return true in test mode")
	}

	// 测试 ConfigureWithCheck
	iface := types.Interface{
		Name:        "eth0",
		IPv4Address: "192.168.1.100/24",
		IPv4Gateway: "192.168.1.1",
		MTU:         1500,
		Nameservers: []string{"8.8.8.8"},
	}

	changed, err := nm.ConfigureWithCheck(ctx, iface)
	if err != nil {
		t.Fatalf("ConfigureWithCheck() error = %v", err)
	}

	if !changed {
		t.Error("ConfigureWithCheck() should return true in test mode")
	}

	// 测试 ReloadIfy
	if err := nm.ReloadIfy(ctx); err != nil {
		t.Fatalf("ReloadIfy() error = %v", err)
	}
}

// TestKylinOSNetplan 测试 Netplan 网络管理器
func TestKylinOSNetplan(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:        "kylin",
		VersionID: "V10",
	}

	netplan := NewKylinOSNetplanForTest(&osInfo)
	ctx := context.Background()

	// 测试 IsInstall
	if !netplan.IsInstall(ctx) {
		t.Error("IsInstall() should return true in test mode")
	}

	// 测试 ConfigureWithCheck
	iface := types.Interface{
		Name:        "eth0",
		IPv4Address: "192.168.1.100/24",
		IPv4Gateway: "192.168.1.1",
		MTU:         1500,
		Nameservers: []string{"8.8.8.8"},
	}

	changed, err := netplan.ConfigureWithCheck(ctx, iface)
	if err != nil {
		t.Fatalf("ConfigureWithCheck() error = %v", err)
	}

	if !changed {
		t.Error("ConfigureWithCheck() should return true in test mode")
	}

	// 测试 ReloadIfy
	if err := netplan.ReloadIfy(ctx); err != nil {
		t.Fatalf("ReloadIfy() error = %v", err)
	}
}

// TestCIDRToNetmask 测试 CIDR 到子网掩码转换
func TestCIDRToNetmask(t *testing.T) {
	ifupdown := &KylinOSIfupdown{}

	tests := []struct {
		name     string
		cidr     string
		expected string
	}{
		{
			name:     "class_c_network",
			cidr:     "192.168.1.0/24",
			expected: "255.255.255.0",
		},
		{
			name:     "class_b_network",
			cidr:     "10.0.0.0/16",
			expected: "255.255.0.0",
		},
		{
			name:     "class_a_network",
			cidr:     "10.0.0.0/8",
			expected: "255.0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ipNet, err := net.ParseCIDR(tt.cidr)
			if err != nil {
				t.Fatalf("Failed to parse CIDR %s: %v", tt.cidr, err)
			}

			result := ifupdown.cidrToNetmask(ipNet)
			if result != tt.expected {
				t.Errorf("cidrToNetmask() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestIsValidKylinOSInterfaceName 测试接口名称验证
func TestIsValidKylinOSInterfaceName(t *testing.T) {
	tests := []struct {
		name     string
		iface    string
		expected bool
	}{
		{
			name:     "valid_eth0",
			iface:    "eth0",
			expected: true,
		},
		{
			name:     "valid_ens33",
			iface:    "ens33",
			expected: true,
		},
		{
			name:     "valid_bond0",
			iface:    "bond0",
			expected: true,
		},
		{
			name:     "valid_with_underscore",
			iface:    "eth_0",
			expected: true,
		},
		{
			name:     "valid_with_dash",
			iface:    "eth-0",
			expected: true,
		},
		{
			name:     "invalid_empty",
			iface:    "",
			expected: false,
		},
		{
			name:     "invalid_starts_with_number",
			iface:    "0eth",
			expected: false,
		},
		{
			name:     "invalid_too_long",
			iface:    "verylonginterfacename",
			expected: false,
		},
		{
			name:     "invalid_special_char",
			iface:    "eth@0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidKylinOSInterfaceName(tt.iface)
			if result != tt.expected {
				t.Errorf("isValidKylinOSInterfaceName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
