package openeuler

import (
	"context"
	"net"
	"testing"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
)

// TestOpenEulerBondIfupdown_IsInstall 测试Bond Ifupdown检测功能
func TestOpenEulerBondIfupdown_IsInstall(t *testing.T) {
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
			var obi *OpenEulerBondIfupdown
			if tt.testMode {
				obi = NewOpenEulerBondIfupdownForTest(tt.osInfo)
			} else {
				obi = NewOpenEulerBondIfupdown(tt.osInfo)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			got := obi.IsInstall(ctx)
			if got != tt.want {
				t.Errorf("OpenEulerBondIfupdown.IsInstall() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestOpenEulerBondIfupdown_Configure 测试Bond配置功能
func TestOpenEulerBondIfupdown_Configure(t *testing.T) {
	tests := []struct {
		name       string
		bondConfig types.BondConfig
		wantErr    bool
		wantChange bool
	}{
		{
			name: "基本Bond配置",
			bondConfig: types.BondConfig{
				Name:   "bond0",
				Mode:   1, // active-backup
				Miimon: 100,
				Network: types.BondNetworkConfig{
					IP:         "192.168.1.100/24",
					Gateway:    "192.168.1.1",
					DNSServers: []string{"8.8.8.8", "8.8.4.4"},
					MTU:        1500,
				},
			},
			wantErr:    false,
			wantChange: true,
		},
		{
			name: "DHCP Bond配置",
			bondConfig: types.BondConfig{
				Name:   "bond1",
				Mode:   0, // balance-rr
				Miimon: 100,
				Network: types.BondNetworkConfig{
					MTU: 1500,
				},
			},
			wantErr:    false,
			wantChange: true,
		},
		{
			name: "空Bond名称应该失败",
			bondConfig: types.BondConfig{
				Name:   "",
				Mode:   1,
				Miimon: 100,
			},
			wantErr:    true,
			wantChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 使用测试模式
			obi := NewOpenEulerBondIfupdownForTest(&controller.OSInfo{
				ID:        "openeuler",
				VersionID: "22.03",
			})

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			changed, err := obi.ConfigureWithCheck(ctx, tt.bondConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("OpenEulerBondIfupdown.ConfigureWithCheck() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if changed != tt.wantChange {
				t.Errorf("OpenEulerBondIfupdown.ConfigureWithCheck() changed = %v, want %v", changed, tt.wantChange)
			}
		})
	}
}

// TestOpenEulerBondIfupdown_ReloadIfy 测试Bond重载功能
func TestOpenEulerBondIfupdown_ReloadIfy(t *testing.T) {
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
			obi := NewOpenEulerBondIfupdownForTest(&controller.OSInfo{
				ID:        "openeuler",
				VersionID: "22.03",
			})

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err := obi.ReloadIfy(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("OpenEulerBondIfupdown.ReloadIfy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBondModeNames 测试Bond模式名称映射
func TestBondModeNames(t *testing.T) {
	tests := []struct {
		mode int
		want string
	}{
		{0, "balance-rr"},
		{1, "active-backup"},
		{2, "balance-xor"},
		{3, "broadcast"},
		{4, "802.3ad"},
		{5, "balance-tlb"},
		{6, "balance-alb"},
		{99, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := types.GetBondModeName(tt.mode)
			if got != tt.want {
				t.Errorf("GetBondModeName(%d) = %v, want %v", tt.mode, got, tt.want)
			}
		})
	}
}

// TestCidrToNetmask 测试CIDR到子网掩码转换
func TestCidrToNetmask(t *testing.T) {
	tests := []struct {
		cidr string
		want string
	}{
		{"192.168.1.100/24", "255.255.255.0"},
		{"10.0.0.1/8", "255.0.0.0"},
		{"172.16.0.1/16", "255.255.0.0"},
		{"192.168.1.100/30", "255.255.255.252"},
	}

	obi := NewOpenEulerBondIfupdownForTest(&controller.OSInfo{
		ID:        "openeuler",
		VersionID: "22.03",
	})

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			_, ipNet, err := net.ParseCIDR(tt.cidr)
			if err != nil {
				t.Fatalf("Failed to parse CIDR %s: %v", tt.cidr, err)
			}
			got := obi.cidrToNetmask(ipNet)
			if got != tt.want {
				t.Errorf("cidrToNetmask(%s) = %v, want %v", tt.cidr, got, tt.want)
			}
		})
	}
}