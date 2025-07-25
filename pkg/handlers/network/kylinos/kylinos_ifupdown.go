package kylinos

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed kylinos_ifupdown.tpl
var kylinOSIfupdownTemplate embed.FS

// KylinOSIfupdown KylinOS ifupdown 网络管理器
type KylinOSIfupdown struct {
	osInfo   *controller.OSInfo
	testMode bool
}

// NewKylinOSIfupdown 创建 KylinOS ifupdown 实例
func NewKylinOSIfupdown(osInfo *controller.OSInfo) *KylinOSIfupdown {
	return &KylinOSIfupdown{
		osInfo:   osInfo,
		testMode: false,
	}
}

// NewKylinOSIfupdownForTest 创建测试用的 KylinOS ifupdown 实例
func NewKylinOSIfupdownForTest(osInfo *controller.OSInfo) *KylinOSIfupdown {
	return &KylinOSIfupdown{
		osInfo:   osInfo,
		testMode: true,
	}
}

// IsInstall 检查 ifupdown 是否已安装
func (i *KylinOSIfupdown) IsInstall(ctx context.Context) bool {
	if i.testMode {
		// 测试模式下模拟 ifupdown 已安装
		return true
	}

	// 检查 ifupdown 相关命令是否存在
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	commands := []string{"ifup", "ifdown", "ifconfig"}
	for _, cmdName := range commands {
		cmd := exec.CommandContext(ctx, "which", cmdName)
		if cmd.Run() != nil {
			utils.Debugf("network", "KylinOS ifupdown command not found: %s", cmdName)
			return false
		}
	}

	// 检查网络接口配置目录是否存在
	if _, err := os.Stat("/etc/network/interfaces"); os.IsNotExist(err) {
		utils.Debug("network", "KylinOS ifupdown interfaces file not found")
		return false
	}

	utils.Info("network", "KylinOS ifupdown is available")
	return true
}

// Configure 配置网络接口
func (i *KylinOSIfupdown) Configure(ctx context.Context, iface types.Interface) error {
	_, err := i.ConfigureWithCheck(ctx, iface)
	return err
}

// ConfigureWithCheck 配置网络接口并检查是否有变更
func (i *KylinOSIfupdown) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	utils.Infof("network", "Starting KylinOS ifupdown configuration for interface %s", iface.Name)
	utils.Infof("network", "Interface %s details: IPv4=%s, IPv6=%s, MTU=%d", iface.Name, iface.IPv4Address, iface.IPv6Address, iface.MTU)

	// 生成配置内容
	utils.Info("network", "Generating KylinOS ifupdown configuration content")
	configContent, err := i.generateConfig(iface)
	if err != nil {
		return false, fmt.Errorf("failed to generate KylinOS ifupdown config: %v", err)
	}
	utils.Info("network", "KylinOS ifupdown configuration content generated successfully")

	if i.testMode {
		// 测试模式下只记录配置内容
		utils.Infof("network", "[TEST MODE] KylinOS ifupdown config for %s:\n%s", iface.Name, configContent)
		return true, nil
	}

	// 确定配置文件路径
	configPath := fmt.Sprintf("/etc/network/interfaces.d/%s", iface.Name)
	utils.Infof("network", "KylinOS ifupdown config file path: %s", configPath)

	// 检查配置是否有变更
	newConfigData := []byte(configContent)
	utils.Info("network", "Checking if KylinOS ifupdown configuration file exists")
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		utils.Info("network", "KylinOS ifupdown configuration file exists, comparing content")
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Infof("network", "KylinOS ifupdown config for %s unchanged", iface.Name)
			return false, nil
		}
		utils.Info("network", "KylinOS ifupdown configuration content has changed")
	} else {
		utils.Info("network", "KylinOS ifupdown configuration file does not exist, will create new one")
	}

	// 创建配置目录
	utils.Info("network", "Creating KylinOS ifupdown configuration directory")
	if err := os.MkdirAll("/etc/network/interfaces.d", 0755); err != nil {
		return false, fmt.Errorf("failed to create KylinOS ifupdown config directory: %v", err)
	}

	// 写入配置文件
	utils.Infof("network", "Writing KylinOS ifupdown configuration to file: %s", configPath)
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write KylinOS ifupdown config: %v", err)
	}

	utils.Infof("network", "KylinOS ifupdown config for %s updated successfully", iface.Name)
	return true, nil
}

// ReloadIfy 重新加载网络配置
func (i *KylinOSIfupdown) ReloadIfy(ctx context.Context) error {
	if i.testMode {
		utils.Info("network", "[TEST MODE] KylinOS ifupdown reload network configuration")
		return nil
	}

	utils.Info("network", "Reloading KylinOS network configuration with ifupdown")

	// 重启网络服务
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	commands := [][]string{
		{"systemctl", "restart", "networking"},
		{"systemctl", "restart", "network"},
	}

	for _, cmdArgs := range commands {
		cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
		if err := cmd.Run(); err == nil {
			utils.Infof("network", "KylinOS network reloaded successfully with: %s", strings.Join(cmdArgs, " "))
			return nil
		} else {
			utils.Debugf("network", "Failed to reload KylinOS network with %s: %v", strings.Join(cmdArgs, " "), err)
		}
	}

	return fmt.Errorf("failed to reload KylinOS network configuration")
}

// generateConfig 生成 ifupdown 配置
func (i *KylinOSIfupdown) generateConfig(iface types.Interface) (string, error) {
	// 获取模板内容
	utils.Info("network", "Loading KylinOS ifupdown template")
	tmplContent, err := utils.GetTemplateContent("kylinos_ifupdown.tpl", i.getEmbeddedTemplate())
	if err != nil {
		return "", fmt.Errorf("failed to get KylinOS ifupdown template: %v", err)
	}

	// 解析模板
	utils.Info("network", "Parsing KylinOS ifupdown template")
	tmpl, err := template.New("kylinos_ifupdown").Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse KylinOS ifupdown template: %v", err)
	}

	// 准备模板数据
	utils.Info("network", "Preparing KylinOS ifupdown template data")
	templateData := i.prepareTemplateData(iface)
	utils.Infof("network", "Template data prepared: HasBonding=%t, IPv4Network=%s, IPv6Network=%s", templateData.HasBonding, templateData.IPv4Network, templateData.IPv6Network)

	// 渲染模板
	utils.Info("network", "Rendering KylinOS ifupdown template")
	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return "", fmt.Errorf("failed to execute KylinOS ifupdown template: %v", err)
	}

	utils.Info("network", "KylinOS ifupdown template rendered successfully")
	return result.String(), nil
}

// getEmbeddedTemplate 获取嵌入的模板内容
func (i *KylinOSIfupdown) getEmbeddedTemplate() string {
	content, err := kylinOSIfupdownTemplate.ReadFile("kylinos_ifupdown.tpl")
	if err != nil {
		return ""
	}
	return string(content)
}

// IfupdownTemplateData ifupdown 模板数据结构
type IfupdownTemplateData struct {
	Interface   types.Interface
	IPv4Network string
	IPv4Netmask string
	IPv6Network string
	IPv6Prefix  string
	HasBonding  bool
}

// prepareTemplateData 准备模板数据
func (i *KylinOSIfupdown) prepareTemplateData(iface types.Interface) IfupdownTemplateData {
	data := IfupdownTemplateData{
		Interface:  iface,
		HasBonding: iface.BondingSlave != nil && iface.BondingSlave.Enabled,
	}

	// 处理 IPv4 地址
	if iface.IPv4Address != "" {
		if ip, ipNet, err := net.ParseCIDR(iface.IPv4Address); err == nil {
			data.IPv4Network = ip.String()
			data.IPv4Netmask = i.cidrToNetmask(ipNet)
		}
	}

	// 处理 IPv6 地址
	if iface.IPv6Address != "" {
		if ip, ipNet, err := net.ParseCIDR(iface.IPv6Address); err == nil {
			data.IPv6Network = ip.String()
			ones, _ := ipNet.Mask.Size()
			data.IPv6Prefix = strconv.Itoa(ones)
		}
	}

	return data
}

// cidrToNetmask 将 CIDR 转换为子网掩码
func (i *KylinOSIfupdown) cidrToNetmask(ipNet *net.IPNet) string {
	mask := ipNet.Mask
	if len(mask) == 4 {
		// IPv4
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	}
	return ""
}
