package openeuler

import (
	"context"
	"testing"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
)

func TestConfigureWithCheckStatusComparison(t *testing.T) {
	// 创建测试用的 NetworkManager 实例
	osInfo := &controller.OSInfo{
		ID:        "openeuler",
		VersionID: "22.03",
	}
	onm := NewOpenEulerNetworkManagerForTest(osInfo)

	// 测试接口配置
	iface := types.Interface{
		Name:        "eth0",
		IPv4Address: "192.168.1.100/24",
		IPv4Gateway: "192.168.1.1",
		MTU:         1500,
		Nameservers: []string{"8.8.8.8", "8.8.4.4"},
	}

	ctx := context.Background()

	// 测试配置检查功能
	changed, err := onm.ConfigureWithCheck(ctx, iface)
	if err != nil {
		t.Fatalf("ConfigureWithCheck failed: %v", err)
	}

	// 在测试模式下，应该总是返回配置已变更
	if !changed {
		t.Error("Expected configuration to be changed in test mode")
	}
}

func TestCompareIPAddress(t *testing.T) {
	onm := &OpenEulerNetworkManager{}

	tests := []struct {
		name     string
		expected string
		current  string
		want     bool
	}{
		{
			name:     "both empty",
			expected: "",
			current:  "",
			want:     true,
		},
		{
			name:     "same CIDR",
			expected: "192.168.1.100/24",
			current:  "192.168.1.100/24",
			want:     true,
		},
		{
			name:     "different IP",
			expected: "192.168.1.100/24",
			current:  "192.168.1.101/24",
			want:     false,
		},
		{
			name:     "different subnet",
			expected: "192.168.1.100/24",
			current:  "192.168.1.100/25",
			want:     false,
		},
		{
			name:     "one empty",
			expected: "192.168.1.100/24",
			current:  "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := onm.compareIPAddress(tt.expected, tt.current)
			if got != tt.want {
				t.Errorf("compareIPAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareStringSlices(t *testing.T) {
	onm := &OpenEulerNetworkManager{}

	tests := []struct {
		name     string
		expected []string
		current  []string
		want     bool
	}{
		{
			name:     "both empty",
			expected: []string{},
			current:  []string{},
			want:     true,
		},
		{
			name:     "same order",
			expected: []string{"8.8.8.8", "8.8.4.4"},
			current:  []string{"8.8.8.8", "8.8.4.4"},
			want:     true,
		},
		{
			name:     "different order",
			expected: []string{"8.8.8.8", "8.8.4.4"},
			current:  []string{"8.8.4.4", "8.8.8.8"},
			want:     true,
		},
		{
			name:     "different content",
			expected: []string{"8.8.8.8", "8.8.4.4"},
			current:  []string{"1.1.1.1", "8.8.8.8"},
			want:     false,
		},
		{
			name:     "different length",
			expected: []string{"8.8.8.8"},
			current:  []string{"8.8.8.8", "8.8.4.4"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := onm.compareStringSlices(tt.expected, tt.current)
			if got != tt.want {
				t.Errorf("compareStringSlices() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareBondConfig(t *testing.T) {
	onm := &OpenEulerNetworkManager{}

	tests := []struct {
		name     string
		expected *types.BondingSlaveConfig
		current  *types.BondingSlaveConfig
		want     bool
	}{
		{
			name:     "both nil",
			expected: nil,
			current:  nil,
			want:     true,
		},
		{
			name:     "same config",
			expected: &types.BondingSlaveConfig{Enabled: true, Master: "bond0"},
			current:  &types.BondingSlaveConfig{Enabled: true, Master: "bond0"},
			want:     true,
		},
		{
			name:     "different master",
			expected: &types.BondingSlaveConfig{Enabled: true, Master: "bond0"},
			current:  &types.BondingSlaveConfig{Enabled: true, Master: "bond1"},
			want:     false,
		},
		{
			name:     "different enabled",
			expected: &types.BondingSlaveConfig{Enabled: true, Master: "bond0"},
			current:  &types.BondingSlaveConfig{Enabled: false, Master: "bond0"},
			want:     false,
		},
		{
			name:     "one nil",
			expected: &types.BondingSlaveConfig{Enabled: true, Master: "bond0"},
			current:  nil,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := onm.compareBondConfig(tt.expected, tt.current)
			if got != tt.want {
				t.Errorf("compareBondConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}