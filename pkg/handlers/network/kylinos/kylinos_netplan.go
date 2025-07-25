package kylinos

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed kylinos_netplan.tpl
var kylinOSNetplanTemplate embed.FS

// KylinOSNetplan KylinOS Netplan 网络管理器
type KylinOSNetplan struct {
	osInfo   *controller.OSInfo
	testMode bool
}

// NewKylinOSNetplan 创建 KylinOS Netplan 实例
func NewKylinOSNetplan(osInfo *controller.OSInfo) *KylinOSNetplan {
	return &KylinOSNetplan{
		osInfo:   osInfo,
		testMode: false,
	}
}

// NewKylinOSNetplanForTest 创建测试用的 KylinOS Netplan 实例
func NewKylinOSNetplanForTest(osInfo *controller.OSInfo) *KylinOSNetplan {
	return &KylinOSNetplan{
		osInfo:   osInfo,
		testMode: true,
	}
}

// IsInstall 检查 Netplan 是否已安装
func (np *KylinOSNetplan) IsInstall(ctx context.Context) bool {
	if np.testMode {
		// 测试模式下模拟 Netplan 已安装
		return true
	}

	// 检查 Netplan 相关命令是否存在
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	commands := []string{"netplan", "systemctl"}
	for _, cmdName := range commands {
		cmd := exec.CommandContext(ctx, "which", cmdName)
		if cmd.Run() != nil {
			utils.Debugf("network", "KylinOS Netplan command not found: %s", cmdName)
			return false
		}
	}

	// 检查 Netplan 配置目录是否存在
	if _, err := os.Stat("/etc/netplan"); os.IsNotExist(err) {
		utils.Debug("network", "KylinOS Netplan config directory not found")
		return false
	}

	utils.Info("network", "KylinOS Netplan is available")
	return true
}

// Configure 配置网络接口
func (np *KylinOSNetplan) Configure(ctx context.Context, iface types.Interface) error {
	_, err := np.ConfigureWithCheck(ctx, iface)
	return err
}

// ConfigureWithCheck 配置网络接口并检查是否有变更
func (np *KylinOSNetplan) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	utils.Infof("network", "Starting KylinOS Netplan configuration for interface %s", iface.Name)
	utils.Infof("network", "Interface %s details: IPv4=%s, IPv6=%s, MTU=%d", iface.Name, iface.IPv4Address, iface.IPv6Address, iface.MTU)

	// 生成配置内容
	utils.Info("network", "Generating KylinOS Netplan configuration content")
	configContent, err := np.generateConfig(iface)
	if err != nil {
		return false, fmt.Errorf("failed to generate KylinOS Netplan config: %v", err)
	}
	utils.Info("network", "KylinOS Netplan configuration content generated successfully")

	if np.testMode {
		// 测试模式下只记录配置内容
		utils.Infof("network", "[TEST MODE] KylinOS Netplan config for %s:\n%s", iface.Name, configContent)
		return true, nil
	}

	// 确定配置文件路径
	configPath := fmt.Sprintf("/etc/netplan/50-nix-operator-%s.yaml", iface.Name)
	utils.Infof("network", "KylinOS Netplan config file path: %s", configPath)

	// 检查配置是否有变更
	newConfigData := []byte(configContent)
	utils.Info("network", "Checking if KylinOS Netplan configuration file exists")
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		utils.Info("network", "KylinOS Netplan configuration file exists, comparing content")
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Infof("network", "KylinOS Netplan config for %s unchanged", iface.Name)
			return false, nil
		}
		utils.Info("network", "KylinOS Netplan configuration content has changed")
	} else {
		utils.Info("network", "KylinOS Netplan configuration file does not exist, will create new one")
	}

	// 创建配置目录
	utils.Info("network", "Creating KylinOS Netplan configuration directory")
	if err := os.MkdirAll("/etc/netplan", 0755); err != nil {
		return false, fmt.Errorf("failed to create KylinOS Netplan config directory: %v", err)
	}

	// 写入配置文件
	utils.Infof("network", "Writing KylinOS Netplan configuration to file: %s", configPath)
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write KylinOS Netplan config: %v", err)
	}

	utils.Infof("network", "KylinOS Netplan config for %s updated successfully", iface.Name)
	return true, nil
}

// ReloadIfy 重新加载网络配置
func (np *KylinOSNetplan) ReloadIfy(ctx context.Context) error {
	if np.testMode {
		utils.Info("network", "[TEST MODE] KylinOS Netplan reload network configuration")
		return nil
	}

	utils.Info("network", "Reloading KylinOS network configuration with Netplan")

	// 应用 Netplan 配置
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 首先尝试 netplan apply
	cmd := exec.CommandContext(ctx, "netplan", "apply")
	if err := cmd.Run(); err == nil {
		utils.Info("network", "KylinOS Netplan configuration applied successfully")
		return nil
	} else {
		utils.Debugf("network", "Failed to apply KylinOS Netplan config with netplan apply: %v", err)
	}

	// 备用方案：生成配置并重启服务
	cmd = exec.CommandContext(ctx, "netplan", "generate")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate KylinOS Netplan configuration: %v", err)
	}

	cmd = exec.CommandContext(ctx, "systemctl", "restart", "systemd-networkd")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart systemd-networkd: %v", err)
	}

	utils.Info("network", "KylinOS Netplan configuration applied successfully with generate + restart")
	return nil
}

// generateConfig 生成 Netplan 配置
func (np *KylinOSNetplan) generateConfig(iface types.Interface) (string, error) {
	// 获取模板内容
	utils.Info("network", "Loading KylinOS Netplan template")
	tmplContent, err := utils.GetTemplateContent("kylinos_netplan.tpl", np.getEmbeddedTemplate())
	if err != nil {
		return "", fmt.Errorf("failed to get KylinOS Netplan template: %v", err)
	}

	// 解析模板
	utils.Info("network", "Parsing KylinOS Netplan template")
	tmpl, err := template.New("kylinos_netplan").Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse KylinOS Netplan template: %v", err)
	}

	// 准备模板数据
	utils.Info("network", "Preparing KylinOS Netplan template data")
	templateData := np.prepareTemplateData(iface)
	utils.Infof("network", "Template data prepared: HasIPv4=%t, HasIPv6=%t, HasBonding=%t, Renderer=%s", templateData.HasIPv4, templateData.HasIPv6, templateData.HasBonding, templateData.Renderer)

	// 渲染模板
	utils.Info("network", "Rendering KylinOS Netplan template")
	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return "", fmt.Errorf("failed to execute KylinOS Netplan template: %v", err)
	}

	utils.Info("network", "KylinOS Netplan template rendered successfully")
	return result.String(), nil
}

// NetplanTemplateData Netplan 模板数据结构
type NetplanTemplateData struct {
	Interface  types.Interface
	HasBonding bool
	HasIPv4    bool
	HasIPv6    bool
	DNSServers []string
	Renderer   string
}

// prepareTemplateData 准备模板数据
func (np *KylinOSNetplan) prepareTemplateData(iface types.Interface) NetplanTemplateData {
	data := NetplanTemplateData{
		Interface:  iface,
		HasBonding: iface.BondingSlave != nil && iface.BondingSlave.Enabled,
		HasIPv4:    iface.IPv4Address != "",
		HasIPv6:    iface.IPv6Address != "",
		DNSServers: iface.Nameservers,
		Renderer:   np.detectRenderer(),
	}

	return data
}

// getEmbeddedTemplate 获取嵌入的模板内容
func (np *KylinOSNetplan) getEmbeddedTemplate() string {
	content, err := kylinOSNetplanTemplate.ReadFile("kylinos_netplan.tpl")
	if err != nil {
		return ""
	}
	return string(content)
}

// detectRenderer 检测 Netplan 渲染器
func (np *KylinOSNetplan) detectRenderer() string {
	if np.testMode {
		return "networkd"
	}

	// 检查是否使用 NetworkManager 作为渲染器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 检查 nmcli 命令是否存在
	cmd := exec.CommandContext(ctx, "which", "nmcli")
	if cmd.Run() == nil {
		// 检查 NetworkManager 服务是否活跃
		cmd = exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "NetworkManager")
		if cmd.Run() == nil {
			return "NetworkManager"
		}
	}

	// 默认使用 networkd
	return "networkd"
}
