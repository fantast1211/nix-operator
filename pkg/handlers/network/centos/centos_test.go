package centos

import (
	"testing"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
)

func TestCentOSNetworkHandler_Match(t *testing.T) {
	tests := []struct {
		name     string
		osInfo   controller.OSInfo
		expected bool
	}{
		{
			name: "CentOS 7.5 should match",
			osInfo: controller.OSInfo{
				ID:         "centos",
				KernelName: "Linux",
				VersionID:  "7.5",
			},
			expected: true,
		},
		{
			name: "CentOS 7.9 should match",
			osInfo: controller.OSInfo{
				ID:         "centos",
				KernelName: "Linux",
				VersionID:  "7.9",
			},
			expected: true,
		},
		{
			name: "CentOS 7.2 should match",
			osInfo: controller.OSInfo{
				ID:         "centos",
				KernelName: "Linux",
				VersionID:  "7.2",
			},
			expected: true,
		},
		{
			name: "CentOS 8.0 should not match",
			osInfo: controller.OSInfo{
				ID:         "centos",
				KernelName: "Linux",
				VersionID:  "8.0",
			},
			expected: false,
		},
		{
			name: "CentOS 7.1 should match",
			osInfo: controller.OSInfo{
				ID:         "centos",
				KernelName: "Linux",
				VersionID:  "7.1",
			},
			expected: true,
		},
		{
			name: "Ubuntu should not match",
			osInfo: controller.OSInfo{
				ID:         "ubuntu",
				KernelName: "Linux",
				VersionID:  "20.04",
			},
			expected: false,
		},
		{
			name: "CentOS 7.5.1804 should match",
			osInfo: controller.OSInfo{
				ID:         "centos",
				KernelName: "Linux",
				VersionID:  "7.5.1804",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &CentOSNetworkHandler{osInfo: tt.osInfo}
			result := handler.Match(tt.osInfo)
			if result != tt.expected {
				t.Errorf("Match() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestCentOSNetworkHandler_isCentOS7Supported(t *testing.T) {
	tests := []struct {
		name      string
		versionID string
		expected  bool
	}{
		{"7", "7", true},
		{"7.2", "7.2", true},
		{"7.3", "7.3", true},
		{"7.4", "7.4", true},
		{"7.5", "7.5", true},
		{"7.6", "7.6", true},
		{"7.7", "7.7", true},
		{"7.8", "7.8", true},
		{"7.9", "7.9", true},
		{"7.5.1804", "7.5.1804", true},
		{"7.0", "7.0", true},
		{"7.1", "7.1", true},
		{"8.0", "8.0", false},
		{"6.10", "6.10", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &CentOSNetworkHandler{}
			result := handler.isCentOS7Supported(tt.versionID)
			if result != tt.expected {
				t.Errorf("isCentOS7Supported(%s) = %v, expected %v", tt.versionID, result, tt.expected)
			}
		})
	}
}

func TestCentOSNetworkHandler_isValidCentOSInterfaceName(t *testing.T) {
	tests := []struct {
		name          string
		interfaceName string
		expected      bool
	}{
		{"eth0", "eth0", true},
		{"eth1", "eth1", true},
		{"ens33", "ens33", true},
		{"enp0s3", "enp0s3", true},
		{"em1", "em1", true},
		{"wlan0", "wlan0", true},
		{"wlp2s0", "wlp2s0", true},
		{"p1p1", "p1p1", true},
		{"invalid", "invalid", false},
		{"lo", "lo", false},
		{"docker0", "docker0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &CentOSNetworkHandler{}
			result := handler.isValidCentOSInterfaceName(tt.interfaceName)
			if result != tt.expected {
				t.Errorf("isValidCentOSInterfaceName(%s) = %v, expected %v", tt.interfaceName, result, tt.expected)
			}
		})
	}
}

// TestCentOSNetworkHandler_WithRealConfig 使用真实配置文件测试
func TestCentOSNetworkHandler_WithRealConfig(t *testing.T) {
	// 模拟 CentOS 7.5 环境
	osInfo := controller.OSInfo{
		ID:         "centos",
		KernelName: "Linux",
		VersionID:  "7.5",
	}

	// 创建 CentOS Handler
	handler := &CentOSNetworkHandler{osInfo: osInfo}

	// 测试匹配
	if !handler.Match(osInfo) {
		t.Error("Handler should match CentOS 7.5")
	}

	// 测试接口名称验证
	validNames := []string{"eth0", "eth1", "ens33", "enp0s3", "em1"}
	for _, name := range validNames {
		if !handler.isValidCentOSInterfaceName(name) {
			t.Errorf("Expected interface name '%s' to be valid for CentOS", name)
		}
	}

	// 测试无效接口名称
	invalidNames := []string{"lo", "docker0", "invalid"}
	for _, name := range invalidNames {
		if handler.isValidCentOSInterfaceName(name) {
			t.Errorf("Expected interface name '%s' to be invalid for CentOS", name)
		}
	}

	t.Log("CentOS Handler basic functionality verified")
}

// TestCentOSNetworkHandler_ConfigValidation 测试配置验证
func TestCentOSNetworkHandler_ConfigValidation(t *testing.T) {
	handler := &CentOSNetworkHandler{}

	tests := []struct {
		name      string
		iface     *systemv1.NetworkInterface
		expectErr bool
	}{
		{
			name: "valid eth0 interface",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100/24",
				Mtu:         1500,
			},
			expectErr: false,
		},
		{
			name: "invalid interface name",
			iface: &systemv1.NetworkInterface{
				Name:        "invalid0",
				Ipv4Address: "192.168.1.100/24",
				Mtu:         1500,
			},
			expectErr: true,
		},
		{
			name: "invalid MTU",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100/24",
				Mtu:         50, // 太小
			},
			expectErr: true,
		},
		{
			name: "invalid IP format",
			iface: &systemv1.NetworkInterface{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100", // 缺少CIDR
				Mtu:         1500,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateCentOSInterface(tt.iface)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
