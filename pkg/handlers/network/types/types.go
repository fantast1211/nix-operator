package types

import (
	"context"
)

type Interface struct {
	Name         string              `json:"name" yaml:"name,omitempty"`
	IPv4Address  string              `json:"ipAddress,omitempty" yaml:"addresses,omitempty"`  // IPv4 地址
	IPv6Address  string              `json:"ipv6Address,omitempty" yaml:"-"`                  // IPv6 地址
	IPv4Gateway  string              `json:"gateway,omitempty" yaml:"gateway4,omitempty"`     // IPv4 网关
	IPv6Gateway  string              `json:"ipv6Gateway,omitempty" yaml:"gateway6,omitempty"` // IPv6 网关
	MTU          int                 `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	Nameservers  []string            `json:"nameservers,omitempty" yaml:"-"`
	BondingSlave *BondingSlaveConfig `json:"bondingSlave,omitempty" yaml:"-"` // Bond 从属配置
}

// BondingSlaveConfig Bond 从属配置
type BondingSlaveConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Master  string `json:"master,omitempty"`
}

type INetworkManager interface {
	IsInstall(ctx context.Context) bool
	Configure(ctx context.Context, iface Interface) error
	ConfigureWithCheck(ctx context.Context, iface Interface) (bool, error) // 返回是否有配置变更
	ReloadIfy(ctx context.Context) error
}
