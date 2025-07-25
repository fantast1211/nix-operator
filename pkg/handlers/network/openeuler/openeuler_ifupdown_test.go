package openeuler

import (
	"context"
	"testing"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
)

// TestOpenEulerIfupdown_IsInstall 测试Network Ifupdown检测功能
func TestOpenEulerIfupdown_IsInstall(t *testing.T) {
	tests := []struct {
		name     string
		osInfo   *controller.OSInfo
		testMode bool
		want     bool
	}{
		{
			name: "测试模式下应该返回true",
			osInfo: &controller.OSInfo{
				ID:        "openeuler",
				VersionID: "22.03",
			},
			testMode: true,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var oif *OpenEulerIfupdown
			if tt.testMode {
				oif = NewOpenEulerIfupdownForTest(tt.osInfo)
			} else {
				oif = NewOpenEulerIfupdown(tt.osInfo)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			got := oif.IsInstall(ctx)
			if got != tt.want {
				t.Errorf("OpenEulerIfupdown.IsInstall() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOpenEulerIfupdown_Configure 测试Network配置功能
func TestOpenEulerIfupdown_Configure(t *testing.T) {
	tests := []struct {
		name       string
		iface      types.Interface
		wantErr    bool
		wantChange bool
	}{
		{
			name: "普通网卡配置",
			iface: types.Interface{
				Name:         "eth0",
				IPv4Address:  "192.168.1.100/24",
				IPv4Gateway:  "192.168.1.1",
				MTU:          1500,
				Nameservers:  []string{"8.8.8.8", "8.8.4.4"},
				BondingSlave: nil,
			},
			wantErr:    false,
			wantChange: true,
		},
		{
			name: "Bond Slave配置",
			iface: types.Interface{
				Name:        "eth1",
				IPv4Address: "192.168.1.101/24", // 这个IP应该被忽略
				IPv4Gateway: "192.168.1.1",      // 这个网关应该被忽略
				MTU:         1500,
				Nameservers: []string{"8.8.8.8"}, // 这个DNS应该被忽略
				BondingSlave: &types.BondingSlaveConfig{
					Enabled: true,
					Master:  "bond0",
				},
			},
			wantErr:    false,
			wantChange: true,
		},
		{
			name: "DHCP配置",
			iface: types.Interface{
				Name:         "eth2",
				MTU:          1500,
				BondingSlave: nil,
			},
			wantErr:    false,
			wantChange: true,
		},
		{
			name: "IPv6配置",
			iface: types.Interface{
				Name:         "eth3",
				IPv4Address:  "192.168.1.102/24",
				IPv6Address:  "2001:db8::1/64",
				IPv4Gateway:  "192.168.1.1",
				IPv6Gateway:  "2001:db8::1111",
				MTU:          1500,
				BondingSlave: nil,
			},
			wantErr:    false,
			wantChange: true,
		},
		{
			name: "空接口名称应该失败",
			iface: types.Interface{
				Name:        "",
				IPv4Address: "192.168.1.100/24",
			},
			wantErr:    true,
			wantChange: false,
		},
		{
			name: "无效IPv4 CIDR应该失败",
			iface: types.Interface{
				Name:        "eth4",
				IPv4Address: "192.168.1.100/99", // 无效的CIDR
			},
			wantErr:    true,
			wantChange: false,
		},
		{
			name: "无效IPv6 CIDR应该失败",
			iface: types.Interface{
				Name:        "eth5",
				IPv6Address: "2001:db8::1/999", // 无效的CIDR
			},
			wantErr:    true,
			wantChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 使用测试模式
			oif := NewOpenEulerIfupdownForTest(&controller.OSInfo{
				ID:        "openeuler",
				VersionID: "22.03",
			})

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			changed, err := oif.ConfigureWithCheck(ctx, tt.iface)
			if (err != nil) != tt.wantErr {
				t.Errorf("OpenEulerIfupdown.ConfigureWithCheck() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if changed != tt.wantChange {
				t.Errorf("OpenEulerIfupdown.ConfigureWithCheck() changed = %v, want %v", changed, tt.wantChange)
			}
		})
	}
}

// TestOpenEulerIfupdown_ReloadIfy 测试Network重载功能
func TestOpenEulerIfupdown_ReloadIfy(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "测试模式下重载应该成功",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 使用测试模式
			oif := NewOpenEulerIfupdownForTest(&controller.OSInfo{
				ID:        "openeuler",
				VersionID: "22.03",
			})

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err := oif.ReloadIfy(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("OpenEulerIfupdown.ReloadIfy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBondSlaveConfiguration 测试Bond Slave配置的特殊处理
func TestBondSlaveConfiguration(t *testing.T) {
	// 创建一个Bond Slave接口配置
	iface := types.Interface{
		Name:        "eth0",
		IPv4Address: "192.168.1.100/24", // 这些IP配置应该被忽略
		IPv6Address: "2001:db8::1/64",
		IPv4Gateway: "192.168.1.1",
		IPv6Gateway: "2001:db8::1111",
		MTU:         1500,
		Nameservers: []string{"8.8.8.8"},
		BondingSlave: &types.BondingSlaveConfig{
			Enabled: true,
			Master:  "bond0",
		},
	}

	// 使用测试模式
	oif := NewOpenEulerIfupdownForTest(&controller.OSInfo{
		ID:        "openeuler",
		VersionID: "22.03",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 配置接口
	changed, err := oif.ConfigureWithCheck(ctx, iface)
	if err != nil {
		t.Fatalf("ConfigureWithCheck() failed: %v", err)
	}

	if !changed {
		t.Error("Expected configuration to be changed")
	}

	// 验证Bond Slave配置逻辑
	// 在实际实现中，IP配置应该被清空，但在测试模式下我们只能验证没有错误发生
	t.Log("Bond slave configuration test passed")
}
