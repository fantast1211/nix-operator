package openeuler

import (
	"context"
	"net"
	"testing"
	"time"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
)

// TestOpenEulerNetworkHandler_Match 测试 openEuler 系统匹配
func TestOpenEulerNetworkHandler_Match(t *testing.T) {
	tests := []struct {
		name     string
		osInfo   *controller.OSInfo
		expected bool
	}{
		{
			name: "openEuler 20.03",
			osInfo: &controller.OSInfo{
				ID:         "openeuler",
				VersionID:  "20.03",
				KernelName: "Linux",
				KernelVer:  "4.19.0",
			},
			expected: true,
		},
		{
			name: "openEuler 22.03",
			osInfo: &controller.OSInfo{
				ID:         "openeuler",
				VersionID:  "22.03",
				KernelName: "Linux",
				KernelVer:  "5.10.0",
			},
			expected: true,
		},
		{
			name: "Ubuntu",
			osInfo: &controller.OSInfo{
				ID:         "ubuntu",
				VersionID:  "20.04",
				KernelName: "Linux",
				KernelVer:  "5.4.0",
			},
			expected: false,
		},
		{
			name: "CentOS",
			osInfo: &controller.OSInfo{
				ID:         "centos",
				VersionID:  "7",
				KernelName: "Linux",
				KernelVer:  "3.10.0",
			},
			expected: false,
		},
		{
			name:     "nil OSInfo",
			osInfo:   nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var handler *OpenEulerNetworkHandler
			if tt.osInfo != nil {
				handler = NewOpenEulerNetworkHandlerForTest(*tt.osInfo)
				result := handler.Match(*tt.osInfo)
				if result != tt.expected {
					t.Errorf("Match() = %v, expected %v", result, tt.expected)
				}
			} else {
				handler = NewOpenEulerNetworkHandlerForTest(controller.OSInfo{})
				result := handler.Match(controller.OSInfo{})
				if result != tt.expected {
					t.Errorf("Match() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

// TestOpenEulerNetworkHandler_NetworkManagerPriority 测试网络管理器优先级
func TestOpenEulerNetworkHandler_NetworkManagerPriority(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}

	handler := NewOpenEulerNetworkHandlerForTest(osInfo)
	ctx := context.Background()

	// 测试网络管理器检测优先级
	manager, err := handler.detectNetworkManager(ctx)
	if err != nil {
		t.Fatalf("Failed to detect network manager: %v", err)
	}

	// 在测试模式下，应该选择 ifupdown（优先级最高且模拟为可用）
	if _, ok := manager.(*OpenEulerIfupdown); !ok {
		t.Errorf("Expected OpenEulerIfupdown manager, got %T", manager)
	}
}

// TestOpenEulerNetworkHandler_Reconcile 测试网络配置调谐
func TestOpenEulerNetworkHandler_Reconcile(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}

	handler := NewOpenEulerNetworkHandlerForTest(osInfo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 测试配置
	testConfigs := []*systemv1.ResourceConfig{}

	// 执行调谐
	_, err := handler.Reconcile(ctx, testConfigs)
	if err != nil {
		t.Errorf("Reconcile() failed: %v", err)
	}
}

// TestOpenEulerIfupdown_TestMode 测试 ifupdown 测试模式
func TestOpenEulerIfupdown_TestMode(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}

	ifupdown := NewOpenEulerIfupdownForTest(&osInfo)
	ctx := context.Background()

	// 测试检测
	if !ifupdown.IsInstall(ctx) {
		t.Error("Expected ifupdown to be detected in test mode")
	}

	// 测试配置
	iface := types.Interface{
		Name:        "eth0",
		IPv4Address: "192.168.1.100/24",
		IPv4Gateway: "192.168.1.1",
		Nameservers: []string{"8.8.8.8"},
	}

	changed, err := ifupdown.ConfigureWithCheck(ctx, iface)
	if err != nil {
		t.Errorf("ConfigureWithCheck() failed: %v", err)
	}
	if !changed {
		t.Error("Expected configuration to be changed in test mode")
	}

	// 测试重载
	err = ifupdown.ReloadIfy(ctx)
	if err != nil {
		t.Errorf("ReloadIfy() failed: %v", err)
	}
}

// TestOpenEulerNetworkManager_TestMode 测试 NetworkManager 测试模式
func TestOpenEulerNetworkManager_TestMode(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}

	nm := NewOpenEulerNetworkManagerForTest(&osInfo)
	ctx := context.Background()

	// 测试检测（在测试模式下应该返回false，因为优先级低于ifupdown）
	if nm.IsInstall(ctx) {
		t.Error("Expected NetworkManager to not be detected in test mode")
	}

	// 测试配置
	iface := types.Interface{
		Name:        "eth0",
		IPv4Address: "192.168.1.100/24",
		IPv4Gateway: "192.168.1.1",
		Nameservers: []string{"8.8.8.8"},
	}

	changed, err := nm.ConfigureWithCheck(ctx, iface)
	if err != nil {
		t.Errorf("ConfigureWithCheck() failed: %v", err)
	}
	if !changed {
		t.Error("Expected configuration to be changed in test mode")
	}

	// 测试重载
	err = nm.ReloadIfy(ctx)
	if err != nil {
		t.Errorf("ReloadIfy() failed: %v", err)
	}
}

// TestOpenEulerNetplan_TestMode 测试 Netplan 测试模式
func TestOpenEulerNetplan_TestMode(t *testing.T) {
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}

	netplan := NewOpenEulerNetplanForTest(&osInfo)
	ctx := context.Background()

	// 测试检测（在测试模式下应该返回false，因为优先级最低）
	if netplan.IsInstall(ctx) {
		t.Error("Expected Netplan to not be detected in test mode")
	}

	// 测试配置
	iface := types.Interface{
		Name:        "eth0",
		IPv4Address: "192.168.1.100/24",
		IPv4Gateway: "192.168.1.1",
		Nameservers: []string{"8.8.8.8"},
	}

	changed, err := netplan.ConfigureWithCheck(ctx, iface)
	if err != nil {
		t.Errorf("ConfigureWithCheck() failed: %v", err)
	}
	if !changed {
		t.Error("Expected configuration to be changed in test mode")
	}

	// 测试重载
	err = netplan.ReloadIfy(ctx)
	if err != nil {
		t.Errorf("ReloadIfy() failed: %v", err)
	}
}

// TestValidateOpenEulerInterface 测试接口验证
func TestValidateOpenEulerInterface(t *testing.T) {
	tests := []struct {
		name      string
		iface     *systemv1.NetworkInterface
		expectErr bool
	}{
		{
			name: "valid interface with IPv4",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100/24",
				Ipv4Gateway: "192.168.1.1",
			},
			expectErr: false,
		},
		{
			name: "valid interface with IPv6",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv6Address: "2001:db8::1/64",
			},
			expectErr: false,
		},
		{
			name: "invalid interface - empty name",
			iface: &systemv1.NetworkInterface{
				Name:        "",
				Ipv4Address: "192.168.1.100/24",
			},
			expectErr: true,
		},
		{
			name: "invalid interface - invalid IPv4",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv4Address: "invalid-ip",
			},
			expectErr: true,
		},
		{
			name: "invalid interface - invalid IPv6",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv6Address: "invalid-ipv6",
			},
			expectErr: true,
		},
	}

	// 创建测试处理器
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}
	handler := NewOpenEulerNetworkHandlerForTest(osInfo)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateOpenEulerInterface(tt.iface)
			if (err != nil) != tt.expectErr {
				t.Errorf("validateOpenEulerInterface() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestCIDRToNetmask 测试 CIDR 转换为子网掩码
func TestCIDRToNetmask(t *testing.T) {
	tests := []struct {
		cidr     string
		expected string
	}{
		{"192.168.1.100/24", "255.255.255.0"},
		{"10.0.0.1/8", "255.0.0.0"},
		{"172.16.0.1/16", "255.255.0.0"},
		{"192.168.1.100/30", "255.255.255.252"},
	}

	// 创建测试用的 ifupdown 实例
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}
	ifupdown := NewOpenEulerIfupdownForTest(&osInfo)

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			// 解析 CIDR
			_, ipnet, err := net.ParseCIDR(tt.cidr)
			if err != nil {
				t.Fatalf("Failed to parse CIDR %s: %v", tt.cidr, err)
			}
			result := ifupdown.cidrToNetmask(ipnet)
			if result != tt.expected {
				t.Errorf("cidrToNetmask(%s) = %s, expected %s", tt.cidr, result, tt.expected)
			}
		})
	}
}
