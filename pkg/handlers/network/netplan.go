package network

import (
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"time"

	"go.xbrother.com/nix-operator/pkg/utils"
	"gopkg.in/yaml.v3"
)

//go:embed netplan.tpl
var netplanTemplate string

type Netplan struct{}

// NetplanConfig 表示netplan配置结构
type NetplanConfig struct {
	Network NetplanNetwork `yaml:"network"`
}

type NetplanNetwork struct {
	Version   int                         `yaml:"version"`
	Ethernets map[string]NetplanInterface `yaml:"ethernets"`
}

type NetplanInterface struct {
	Addresses   []string            `yaml:"addresses,omitempty"`
	Gateway4    string              `yaml:"gateway4,omitempty"`
	Gateway6    string              `yaml:"gateway6,omitempty"`
	MTU         int                 `yaml:"mtu,omitempty"`
	Nameservers *NetplanNameservers `yaml:"nameservers,omitempty"`
}

type NetplanNameservers struct {
	Addresses []string `yaml:"addresses"`
}

func (np *Netplan) IsInstall(ctx context.Context) bool {
	// 检查netplan命令是否存在
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "which", "netplan")
	return cmd.Run() == nil
}

func (np *Netplan) Configure(ctx context.Context, iface Interface) error {
	// 验证接口名称不能为空
	if iface.Name == "" {
		return fmt.Errorf("interface name cannot be empty")
	}

	// 构建netplan接口配置
	netplanIface := NetplanInterface{
		MTU: iface.MTU,
	}

	// 设置IP地址
	if iface.IPv4Address != "" {
		netplanIface.Addresses = append(netplanIface.Addresses, iface.IPv4Address)
	}
	if iface.IPv6Address != "" {
		netplanIface.Addresses = append(netplanIface.Addresses, iface.IPv6Address)
	}

	// 设置网关
	if iface.IPv4Gateway != "" {
		netplanIface.Gateway4 = iface.IPv4Gateway
	}
	if iface.IPv6Gateway != "" {
		netplanIface.Gateway6 = iface.IPv6Gateway
	}

	// 设置DNS服务器
	if len(iface.Nameservers) > 0 {
		netplanIface.Nameservers = &NetplanNameservers{
			Addresses: iface.Nameservers,
		}
	}

	// 构建完整配置
	config := NetplanConfig{
		Network: NetplanNetwork{
			Version: 2,
			Ethernets: map[string]NetplanInterface{
				iface.Name: netplanIface,
			},
		},
	}

	// 序列化为YAML
	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal netplan config: %v", err)
	}

	// 写入配置文件
	configPath := fmt.Sprintf("/etc/netplan/50-nix-operator-%s.yaml", iface.Name)
	if err := utils.AtomicWriteFile(yamlData, configPath, 0644); err != nil {
		return fmt.Errorf("failed to write netplan config: %v", err)
	}

	return nil
}

func (np *Netplan) ReloadIfy(ctx context.Context) error {
	// 使用netplan apply重新加载配置
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "netplan", "apply")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply netplan config: %v, output: %s", err, string(output))
	}

	return nil
}
