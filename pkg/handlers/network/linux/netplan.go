package linux

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"time"

	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
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

func (np *Netplan) Configure(ctx context.Context, iface types.Interface) error {
	_, err := np.ConfigureWithCheck(ctx, iface)
	return err
}

func (np *Netplan) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	utils.Infof("network", "Starting Linux Netplan configuration for interface %s", iface.Name)

	// 验证接口名称不能为空
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}
	utils.Infof("network", "Interface %s details: IPv4=%s, IPv6=%s, MTU=%d", iface.Name, iface.IPv4Address, iface.IPv6Address, iface.MTU)

	// 构建netplan接口配置
	utils.Info("network", "Building Linux Netplan interface configuration")
	netplanIface := NetplanInterface{
		MTU: iface.MTU,
	}

	// 设置IP地址
	if iface.IPv4Address != "" {
		utils.Infof("network", "Adding IPv4 address: %s", iface.IPv4Address)
		netplanIface.Addresses = append(netplanIface.Addresses, iface.IPv4Address)
	}
	if iface.IPv6Address != "" {
		utils.Infof("network", "Adding IPv6 address: %s", iface.IPv6Address)
		netplanIface.Addresses = append(netplanIface.Addresses, iface.IPv6Address)
	}

	// 设置网关
	if iface.IPv4Gateway != "" {
		utils.Infof("network", "Setting IPv4 gateway: %s", iface.IPv4Gateway)
		netplanIface.Gateway4 = iface.IPv4Gateway
	}
	if iface.IPv6Gateway != "" {
		utils.Infof("network", "Setting IPv6 gateway: %s", iface.IPv6Gateway)
		netplanIface.Gateway6 = iface.IPv6Gateway
	}

	// 设置DNS服务器
	if len(iface.Nameservers) > 0 {
		utils.Infof("network", "Setting DNS servers: %v", iface.Nameservers)
		netplanIface.Nameservers = &NetplanNameservers{
			Addresses: iface.Nameservers,
		}
	}

	// 构建完整配置
	utils.Info("network", "Building complete Linux Netplan configuration")
	config := NetplanConfig{
		Network: NetplanNetwork{
			Version: 2,
			Ethernets: map[string]NetplanInterface{
				iface.Name: netplanIface,
			},
		},
	}

	// 序列化为YAML
	utils.Info("network", "Serializing Linux Netplan configuration to YAML")
	newYamlData, err := yaml.Marshal(config)
	if err != nil {
		return false, fmt.Errorf("failed to marshal netplan config: %v", err)
	}
	utils.Info("network", "Linux Netplan configuration serialized successfully")

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/netplan/50-nix-operator-%s.yaml", iface.Name)
	utils.Infof("network", "Linux Netplan config file path: %s", configPath)

	utils.Info("network", "Checking if Linux Netplan configuration file exists")
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		utils.Info("network", "Linux Netplan configuration file exists, comparing content")
		// 文件存在，比较内容
		if bytes.Equal(existingData, newYamlData) {
			utils.Infof("network", "Linux Netplan config for interface %s unchanged, skipping write", iface.Name)
			return false, nil // 配置未变更
		}
		utils.Info("network", "Linux Netplan configuration content has changed")
	} else {
		utils.Info("network", "Linux Netplan configuration file does not exist, will create new one")
	}

	// 写入配置文件
	utils.Infof("network", "Writing Linux Netplan configuration to file: %s", configPath)
	if err := utils.AtomicWriteFile(newYamlData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write netplan config: %v", err)
	}

	utils.Infof("network", "Linux Netplan config for interface %s updated successfully", iface.Name)
	return true, nil // 配置已变更
}

func (np *Netplan) ReloadIfy(ctx context.Context) error {
	utils.Info("network", "Applying Linux Netplan configuration")
	// 使用netplan apply重新加载配置
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "netplan", "apply")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply netplan config: %v, output: %s", err, string(output))
	}

	utils.Info("network", "Linux Netplan configuration applied successfully")
	return nil
}
