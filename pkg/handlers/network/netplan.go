package network

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/utils"
	"gopkg.in/yaml.v3"
)

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
	MTU         int                 `yaml:"mtu,omitempty"`
	Addresses   []string            `yaml:"addresses,omitempty"`
	Gateway4    string              `yaml:"gateway4,omitempty"`
	Gateway6    string              `yaml:"gateway6,omitempty"`
	Nameservers *NetplanNameservers `yaml:"nameservers,omitempty"`
}

type NetplanNameservers struct {
	Addresses []string `yaml:"addresses"`
}

func (np *Netplan) IsInstall(ctx context.Context) bool {
	_, err := os.Stat("/usr/sbin/netplan")
	return err == nil
}

func (np *Netplan) findConfig(iface Interface) (string, error) {
	files, err := os.ReadDir("/etc/netplan")
	if err != nil {
		return "", fmt.Errorf("failed to read netplan directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		path := filepath.Join("/etc/netplan", file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var config map[string]any
		if err := yaml.Unmarshal(data, &config); err != nil {
			continue
		}

		network, ok := config["network"].(map[string]any)
		if !ok {
			continue
		}

		ethernets, ok := network["ethernets"].(map[string]any)
		if !ok {
			continue
		}

		if _, ok := ethernets[iface.Name]; ok {
			return path, nil
		}
	}

	return fmt.Sprintf("/etc/netplan/99-%s.yaml", iface.Name), nil
}

func (np *Netplan) buildInterfaceConfig(iface Interface) NetplanInterface {
	ifaceConfig := NetplanInterface{
		MTU: iface.MTU,
	}

	// 配置地址
	var addresses []string
	if iface.IPAddress != "" {
		addresses = append(addresses, iface.IPAddress)
	}
	if iface.IPv6Address != "" {
		addresses = append(addresses, iface.IPv6Address)
	}
	if len(addresses) > 0 {
		ifaceConfig.Addresses = addresses
	}

	// 配置网关
	if iface.Gateway != "" {
		ifaceConfig.Gateway4 = iface.Gateway
	}
	if iface.IPv6Gateway != "" {
		ifaceConfig.Gateway6 = iface.IPv6Gateway
	}

	// 配置DNS nameservers
	if len(iface.Nameservers) > 0 {
		ifaceConfig.Nameservers = &NetplanNameservers{
			Addresses: iface.Nameservers,
		}
	}

	return ifaceConfig
}

func (np *Netplan) Configure(ctx context.Context, iface Interface) error {
	configPath, err := np.findConfig(iface)
	if err != nil {
		return err
	}

	// 构建配置结构
	desired := NetplanConfig{
		Network: NetplanNetwork{
			Version: 2,
			Ethernets: map[string]NetplanInterface{
				iface.Name: np.buildInterfaceConfig(iface),
			},
		},
	}

	// 读取现有配置进行比较
	var current NetplanConfig
	if data, err := os.ReadFile(configPath); err == nil {
		if err := yaml.Unmarshal(data, &current); err == nil && reflect.DeepEqual(current, desired) {
			return nil // 配置相同，无需更新
		}
	}

	// 序列化并写入新配置
	data, err := yaml.Marshal(desired)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// 添加注释头
	configWithHeader := append([]byte(config.CommentHeader), data...)

	return utils.AtomicWriteFile(configWithHeader, configPath, 0644)
}

func (np *Netplan) ReloadIfy(ctx context.Context) error {
	// 检查 systemd-networkd 或 NetworkManager 是否在运行
	// netplan 会生成这两个服务之一的配置
	if !isServiceActive(ctx, "systemd-networkd") && !isServiceActive(ctx, "NetworkManager") {
		return nil
	}

	cmd := exec.CommandContext(ctx, "netplan", "apply")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to apply netplan: %v, output: %s", err, output)
	}
	return nil
}
