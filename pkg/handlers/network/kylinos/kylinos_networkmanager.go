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

//go:embed kylinos_nmconnection.tpl
var kylinOSNetworkManagerTemplate embed.FS

// KylinOSNetworkManager KylinOS NetworkManager 网络管理器
type KylinOSNetworkManager struct {
	osInfo   *controller.OSInfo
	testMode bool
}

// NewKylinOSNetworkManager 创建 KylinOS NetworkManager 实例
func NewKylinOSNetworkManager(osInfo *controller.OSInfo) *KylinOSNetworkManager {
	return &KylinOSNetworkManager{
		osInfo:   osInfo,
		testMode: false,
	}
}

// NewKylinOSNetworkManagerForTest 创建测试用的 KylinOS NetworkManager 实例
func NewKylinOSNetworkManagerForTest(osInfo *controller.OSInfo) *KylinOSNetworkManager {
	return &KylinOSNetworkManager{
		osInfo:   osInfo,
		testMode: true,
	}
}

// IsInstall 检查 NetworkManager 是否已安装
func (nm *KylinOSNetworkManager) IsInstall(ctx context.Context) bool {
	if nm.testMode {
		// 测试模式下模拟 NetworkManager 已安装
		return true
	}

	// 检查 NetworkManager 相关命令是否存在
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	commands := []string{"nmcli", "systemctl"}
	for _, cmdName := range commands {
		cmd := exec.CommandContext(ctx, "which", cmdName)
		if cmd.Run() != nil {
			utils.Debugf("network", "KylinOS NetworkManager command not found: %s", cmdName)
			return false
		}
	}

	// 检查 NetworkManager 服务状态
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "NetworkManager")
	if cmd.Run() != nil {
		utils.Debug("network", "KylinOS NetworkManager service is not active")
		return false
	}

	utils.Info("network", "KylinOS NetworkManager is available")
	return true
}

// Configure 配置网络接口
func (nm *KylinOSNetworkManager) Configure(ctx context.Context, iface types.Interface) error {
	_, err := nm.ConfigureWithCheck(ctx, iface)
	return err
}

// ConfigureWithCheck 配置网络接口并检查是否有变更
func (nm *KylinOSNetworkManager) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	utils.Infof("network", "Configuring KylinOS interface %s with NetworkManager", iface.Name)

	// 生成配置内容
	configContent, err := nm.generateConfig(iface)
	if err != nil {
		return false, fmt.Errorf("failed to generate KylinOS NetworkManager config: %v", err)
	}

	if nm.testMode {
		// 测试模式下只记录配置内容
		utils.Infof("network", "[TEST MODE] KylinOS NetworkManager config for %s:\n%s", iface.Name, configContent)
		return true, nil
	}

	// 确定配置文件路径
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/%s.nmconnection", iface.Name)

	// 检查配置是否有变更
	newConfigData := []byte(configContent)
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Infof("network", "KylinOS NetworkManager config for %s unchanged", iface.Name)
			return false, nil
		}
	}

	// 创建配置目录
	if err := os.MkdirAll("/etc/NetworkManager/system-connections", 0755); err != nil {
		return false, fmt.Errorf("failed to create KylinOS NetworkManager config directory: %v", err)
	}

	// 写入配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0600); err != nil {
		return false, fmt.Errorf("failed to write KylinOS NetworkManager config: %v", err)
	}

	utils.Infof("network", "KylinOS NetworkManager config for %s updated", iface.Name)
	return true, nil
}

// ReloadIfy 重新加载网络配置
func (nm *KylinOSNetworkManager) ReloadIfy(ctx context.Context) error {
	if nm.testMode {
		utils.Info("network", "[TEST MODE] KylinOS NetworkManager reload network configuration")
		return nil
	}

	utils.Info("network", "Reloading KylinOS network configuration with NetworkManager")

	// 重新加载 NetworkManager 配置
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload KylinOS NetworkManager configuration: %v", err)
	}

	utils.Info("network", "KylinOS NetworkManager configuration reloaded successfully")
	return nil
}

// generateConfig 生成 NetworkManager 配置
func (nm *KylinOSNetworkManager) generateConfig(iface types.Interface) (string, error) {
	// 获取模板内容
	tmplContent, err := utils.GetTemplateContent("kylinos_nmconnection.tpl", nm.getEmbeddedTemplate())
	if err != nil {
		return "", fmt.Errorf("failed to get KylinOS NetworkManager template: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("kylinos_nmconnection").Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse KylinOS NetworkManager template: %v", err)
	}

	// 准备模板数据
	templateData := nm.prepareTemplateData(iface)

	// 渲染模板
	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return "", fmt.Errorf("failed to execute KylinOS NetworkManager template: %v", err)
	}

	return result.String(), nil
}

// getEmbeddedTemplate 获取嵌入的模板内容
func (nm *KylinOSNetworkManager) getEmbeddedTemplate() string {
	content, err := kylinOSNetworkManagerTemplate.ReadFile("kylinos_nmconnection.tpl")
	if err != nil {
		return ""
	}
	return string(content)
}

// NetworkManagerTemplateData NetworkManager 模板数据结构
type NetworkManagerTemplateData struct {
	Interface      types.Interface
	ConnectionUUID string
	HasBonding     bool
	HasIPv4        bool
	HasIPv6        bool
	DNSServers     string
}

// prepareTemplateData 准备模板数据
func (nm *KylinOSNetworkManager) prepareTemplateData(iface types.Interface) NetworkManagerTemplateData {
	data := NetworkManagerTemplateData{
		Interface:      iface,
		ConnectionUUID: nm.generateUUID(iface.Name),
		HasBonding:     iface.BondingSlave != nil && iface.BondingSlave.Enabled,
		HasIPv4:        iface.IPv4Address != "",
		HasIPv6:        iface.IPv6Address != "",
	}

	// 处理 DNS 服务器
	if len(iface.Nameservers) > 0 {
		data.DNSServers = strings.Join(iface.Nameservers, ";")
	}

	return data
}

// generateUUID 为连接生成 UUID
func (nm *KylinOSNetworkManager) generateUUID(interfaceName string) string {
	// 简单的 UUID 生成逻辑，基于接口名称
	// 在实际环境中，可以使用更复杂的 UUID 生成算法
	hash := 0
	for _, char := range interfaceName {
		hash = hash*31 + int(char)
	}
	return fmt.Sprintf("12345678-1234-5678-9abc-%012d", hash%1000000000000)
}
