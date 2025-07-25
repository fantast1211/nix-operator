package openeuler

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// OpenEulerIfupdown openEuler 专用的传统网络脚本实现
// 专注于物理网卡配置，支持bond slave接口
type OpenEulerIfupdown struct {
	osInfo   *controller.OSInfo
	testMode bool // 测试模式标志
}

//go:embed openeuler_ifcfg.tpl
var openeulerIfcfgTemplate string

// OpenEulerIfcfgData 用于模板渲染的数据结构
type OpenEulerIfcfgData struct {
	Name         string
	IPv4IP       string
	IPv4Netmask  string
	IPv4Gateway  string
	IPv6IP       string
	IPv6Prefix   string
	IPv6Gateway  string
	MTU          int
	Nameservers  []string
	BondingSlave *types.BondingSlaveConfig
}

// NewOpenEulerIfupdown 创建 openEuler Ifupdown 实例
func NewOpenEulerIfupdown(osInfo *controller.OSInfo) *OpenEulerIfupdown {
	return &OpenEulerIfupdown{
		osInfo:   osInfo,
		testMode: false,
	}
}

// NewOpenEulerIfupdownForTest 创建测试用的 openEuler Ifupdown 实例
func NewOpenEulerIfupdownForTest(osInfo *controller.OSInfo) *OpenEulerIfupdown {
	return &OpenEulerIfupdown{
		osInfo:   osInfo,
		testMode: true,
	}
}

func (oif *OpenEulerIfupdown) IsInstall(ctx context.Context) bool {
	// 测试模式下模拟检测逻辑
	if oif.testMode {
		return oif.mockIsInstall()
	}

	// 检查传统网络脚本工具是否存在
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 检查ifup和ifdown命令
	cmd := exec.CommandContext(ctx, "which", "ifup")
	if cmd.Run() != nil {
		utils.Debug("network", "ifup command not found")
		return false
	}

	cmd = exec.CommandContext(ctx, "which", "ifdown")
	if cmd.Run() != nil {
		utils.Debug("network", "ifdown command not found")
		return false
	}

	// 检查 /etc/sysconfig/network-scripts 目录是否存在
	if _, err := os.Stat("/etc/sysconfig/network-scripts"); os.IsNotExist(err) {
		utils.Debug("network", "/etc/sysconfig/network-scripts directory not found")
		return false
	}

	// 检查是否有其他网络管理器在运行
	ctx2, cancel2 := context.WithTimeout(ctx, 3*time.Second)
	defer cancel2()

	// 检查NetworkManager是否在运行
	cmd = exec.CommandContext(ctx2, "systemctl", "is-active", "NetworkManager")
	if cmd.Run() == nil {
		// NetworkManager正在运行，传统网络脚本可能不是主要管理器
		utils.Debug("network", "NetworkManager is running, traditional network scripts might not be primary")
		return false
	}

	// 检查systemd-networkd是否在运行
	cmd = exec.CommandContext(ctx2, "systemctl", "is-active", "systemd-networkd")
	if cmd.Run() == nil {
		utils.Debug("network", "systemd-networkd is running, traditional network scripts might not be primary")
		return false
	}

	// 检查network服务是否存在且启用（openEuler系统）
	cmd = exec.CommandContext(ctx2, "systemctl", "is-enabled", "network")
	if cmd.Run() == nil {
		utils.Info("network", "openEuler traditional network scripts detected and verified")
		return true
	}

	// 如果没有其他网络管理器在运行，且传统工具存在，则认为可用
	utils.Info("network", "openEuler ifupdown tools available as fallback")
	return true
}

func (oif *OpenEulerIfupdown) Configure(ctx context.Context, iface types.Interface) error {
	_, err := oif.ConfigureWithCheck(ctx, iface)
	return err
}

func (oif *OpenEulerIfupdown) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	utils.Infof("network", "Starting openEuler ifupdown configuration for interface %s", iface.Name)

	// 测试模式下模拟配置逻辑
	if oif.testMode {
		return oif.mockConfigureWithCheck(iface)
	}

	// 验证接口名称不能为空
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}

	utils.Infof("network", "Interface validation passed for %s", iface.Name)

	// 验证IPv4地址格式
	if iface.IPv4Address != "" {
		utils.Infof("network", "Validating IPv4 address format: %s", iface.IPv4Address)
		if _, _, err := net.ParseCIDR(iface.IPv4Address); err != nil {
			return false, fmt.Errorf("invalid IPv4 CIDR format: %s", iface.IPv4Address)
		}
	}

	// 验证IPv6地址格式
	if iface.IPv6Address != "" {
		utils.Infof("network", "Validating IPv6 address format: %s", iface.IPv6Address)
		if _, _, err := net.ParseCIDR(iface.IPv6Address); err != nil {
			return false, fmt.Errorf("invalid IPv6 CIDR format: %s", iface.IPv6Address)
		}
	}

	// 准备模板数据，解析 CIDR 格式的地址
	templateData := OpenEulerIfcfgData{
		Name:         iface.Name,
		IPv4Gateway:  iface.IPv4Gateway,
		IPv6Gateway:  iface.IPv6Gateway,
		MTU:          iface.MTU,
		Nameservers:  iface.Nameservers,
		BondingSlave: iface.BondingSlave,
	}

	// 输出bond配置信息
	if iface.BondingSlave != nil && iface.BondingSlave.Enabled {
		utils.Infof("network", "Bond slave configuration detected for interface %s: master=%s",
			iface.Name, iface.BondingSlave.Master)
		// 对于bond slave接口，不配置IP地址相关信息
		if iface.IPv4Address != "" || iface.IPv6Address != "" {
			utils.Warnf("network", "Bond slave interface %s should not have IP configuration, ignoring IP settings", iface.Name)
			// 清空IP配置，slave接口不应该有IP
			templateData.IPv4IP = ""
			templateData.IPv4Netmask = ""
			templateData.IPv4Gateway = ""
			templateData.IPv6IP = ""
			templateData.IPv6Prefix = ""
			templateData.IPv6Gateway = ""
			templateData.Nameservers = nil
		}
	} else {
		utils.Infof("network", "Regular network interface configuration for %s", iface.Name)
	}

	// 解析 IPv4 地址和子网掩码
	if iface.IPv4Address != "" {
		if strings.Contains(iface.IPv4Address, "/") {
			// CIDR 格式：192.168.1.100/24
			ip, ipNet, err := net.ParseCIDR(iface.IPv4Address)
			if err != nil {
				return false, fmt.Errorf("failed to parse IPv4 CIDR %s: %v", iface.IPv4Address, err)
			}
			templateData.IPv4IP = ip.String()
			templateData.IPv4Netmask = oif.cidrToNetmask(ipNet)
		} else {
			// 纯 IP 地址格式
			templateData.IPv4IP = iface.IPv4Address
		}
	}

	// 解析 IPv6 地址和前缀
	if iface.IPv6Address != "" {
		if strings.Contains(iface.IPv6Address, "/") {
			// CIDR 格式：2001:db8::1/64
			ip, ipNet, err := net.ParseCIDR(iface.IPv6Address)
			if err != nil {
				return false, fmt.Errorf("failed to parse IPv6 CIDR %s: %v", iface.IPv6Address, err)
			}
			templateData.IPv6IP = ip.String()
			prefixLen, _ := ipNet.Mask.Size()
			templateData.IPv6Prefix = strconv.Itoa(prefixLen)
		} else {
			// 纯 IP 地址格式
			templateData.IPv6IP = iface.IPv6Address
		}
	}

	// 获取模板内容
	utils.Infof("network", "Loading openEuler ifcfg template for interface %s", iface.Name)
	templateContent, err := utils.GetTemplateContent("openeuler_ifcfg.tpl", openeulerIfcfgTemplate)
	if err != nil {
		return false, err
	}

	// 解析模板
	tmpl, err := template.New("openeuler_ifcfg").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse openEuler ifcfg template: %v", err)
	}

	utils.Infof("network", "Template parsed successfully for interface %s", iface.Name)

	// 渲染模板
	utils.Infof("network", "Rendering ifcfg configuration template for interface %s", iface.Name)
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return false, fmt.Errorf("failed to execute openEuler ifcfg template: %v", err)
	}

	newConfigData := []byte(content.String())
	utils.Infof("network", "Template rendered successfully for interface %s, config size: %d bytes", iface.Name, len(newConfigData))

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/sysconfig/network-scripts/ifcfg-%s", iface.Name)
	utils.Infof("network", "Checking existing configuration file: %s", configPath)
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Infof("network", "openEuler ifcfg config for interface %s unchanged, skipping write", iface.Name)
			return false, nil // 配置未变更
		}
		utils.Infof("network", "Configuration changed for interface %s, updating file", iface.Name)
	} else {
		utils.Infof("network", "Configuration file does not exist for interface %s, creating new file", iface.Name)
	}

	// 确保目标目录存在
	utils.Infof("network", "Ensuring target directory exists: %s", filepath.Dir(configPath))
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return false, fmt.Errorf("failed to create directory %s: %v", filepath.Dir(configPath), err)
	}

	// 写入配置文件
	utils.Infof("network", "Writing ifcfg configuration file for interface %s to %s", iface.Name, configPath)
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write openEuler ifcfg config: %v", err)
	}

	// 如果配置了DNS，更新 /etc/resolv.conf
	if len(iface.Nameservers) > 0 {
		utils.Infof("network", "Updating DNS configuration with %d nameservers", len(iface.Nameservers))
		if err := oif.updateResolvConf(iface.Nameservers); err != nil {
			utils.Warnf("network", "Failed to update /etc/resolv.conf: %v", err)
		} else {
			utils.Infof("network", "DNS configuration updated successfully")
		}
	}

	utils.Infof("network", "openEuler ifcfg configuration written successfully for interface %s", iface.Name)
	return true, nil // 配置已变更
}

func (oif *OpenEulerIfupdown) ReloadIfy(ctx context.Context) error {
	utils.Info("network", "Reloading openEuler ifupdown network configuration")
	// 测试模式下模拟重载逻辑
	if oif.testMode {
		return oif.mockReloadIfy()
	}

	// openEuler 特定的网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 尝试多种重启网络的方法，按优先级排序
	reloadMethods := []struct {
		name string
		cmd  []string
	}{
		{"systemctl restart network", []string{"systemctl", "restart", "network"}},
		{"service network restart", []string{"service", "network", "restart"}},
		{"systemctl restart NetworkManager", []string{"systemctl", "restart", "NetworkManager"}},
		{"systemctl restart systemd-networkd", []string{"systemctl", "restart", "systemd-networkd"}},
	}

	for _, method := range reloadMethods {
		utils.Infof("network", "Trying openEuler network reload method: %s", method.name)
		cmd := exec.CommandContext(ctx, method.cmd[0], method.cmd[1:]...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			utils.Infof("network", "Successfully reloaded openEuler network using: %s", method.name)
			// 等待网络稳定
			utils.Info("network", "Waiting for network to stabilize...")
			time.Sleep(3 * time.Second)
			return nil
		}
		utils.Infof("network", "openEuler reload method %s failed: %v, output: %s", method.name, err, string(output))
	}

	// 如果所有系统级重载方法都失败，尝试使用 ifup/ifdown 命令逐个重载接口
	utils.Info("network", "All system-level reload methods failed, trying interface-level reload")
	return oif.reloadInterfacesIndividually(ctx)
}

// reloadInterfacesIndividually 尝试使用 ifup/ifdown 命令逐个重载网络接口
func (oif *OpenEulerIfupdown) reloadInterfacesIndividually(ctx context.Context) error {
	// 获取所有网络接口配置文件
	files, err := filepath.Glob("/etc/sysconfig/network-scripts/ifcfg-*")
	if err != nil {
		return fmt.Errorf("failed to find interface config files: %v", err)
	}

	var lastError error
	successCount := 0

	for _, file := range files {
		// 从文件名提取接口名
		basename := filepath.Base(file)
		if !strings.HasPrefix(basename, "ifcfg-") {
			continue
		}
		ifaceName := strings.TrimPrefix(basename, "ifcfg-")

		// 跳过回环接口和特殊接口
		if ifaceName == "lo" || strings.HasPrefix(ifaceName, "ifcfg-") {
			continue
		}

		utils.Infof("network", "Reloading interface: %s", ifaceName)

		// 先 down 接口
		downCmd := exec.CommandContext(ctx, "ifdown", ifaceName)
		downOutput, downErr := downCmd.CombinedOutput()
		if downErr != nil {
			utils.Infof("network", "Warning: ifdown %s failed: %v, output: %s", ifaceName, downErr, string(downOutput))
		}

		// 等待一下
		time.Sleep(1 * time.Second)

		// 再 up 接口
		upCmd := exec.CommandContext(ctx, "ifup", ifaceName)
		upOutput, upErr := upCmd.CombinedOutput()
		if upErr != nil {
			utils.Infof("network", "Warning: ifup %s failed: %v, output: %s", ifaceName, upErr, string(upOutput))
			lastError = fmt.Errorf("failed to reload interface %s: %v", ifaceName, upErr)
		} else {
			utils.Infof("network", "Successfully reloaded interface: %s", ifaceName)
			successCount++
		}
	}

	if successCount > 0 {
		utils.Infof("network", "Successfully reloaded %d interfaces", successCount)
		return nil
	}

	if lastError != nil {
		return lastError
	}

	return fmt.Errorf("no network interfaces found to reload")
}

// cidrToNetmask 将CIDR转换为子网掩码
func (oif *OpenEulerIfupdown) cidrToNetmask(ipNet *net.IPNet) string {
	mask := ipNet.Mask
	if len(mask) == 4 {
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	}
	return ""
}

// updateResolvConf 更新DNS配置
func (oif *OpenEulerIfupdown) updateResolvConf(nameservers []string) error {
	var content strings.Builder
	content.WriteString("# Generated by nix-operator for openEuler\n")
	for _, ns := range nameservers {
		content.WriteString(fmt.Sprintf("nameserver %s\n", ns))
	}

	return utils.AtomicWriteFile([]byte(content.String()), "/etc/resolv.conf", 0644)
}

// 测试模式相关方法
func (oif *OpenEulerIfupdown) mockIsInstall() bool {
	// 模拟检测逻辑：假设ifupdown总是可用
	utils.Info("network", "[TEST MODE] openEuler ifupdown tools detected")
	return true
}

func (oif *OpenEulerIfupdown) mockConfigureWithCheck(iface types.Interface) (bool, error) {
	// 验证接口名称
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}

	// 验证IPv4地址格式
	if iface.IPv4Address != "" {
		if _, _, err := net.ParseCIDR(iface.IPv4Address); err != nil {
			return false, fmt.Errorf("invalid IPv4 CIDR format: %s", iface.IPv4Address)
		}
	}

	// 验证IPv6地址格式
	if iface.IPv6Address != "" {
		if _, _, err := net.ParseCIDR(iface.IPv6Address); err != nil {
			return false, fmt.Errorf("invalid IPv6 CIDR format: %s", iface.IPv6Address)
		}
	}

	// 如果是Bond从属接口，清空IP配置并发出警告
	if iface.BondingSlave != nil && iface.BondingSlave.Enabled {
		if iface.IPv4Address != "" || iface.IPv6Address != "" || iface.IPv4Gateway != "" || iface.IPv6Gateway != "" || len(iface.Nameservers) > 0 {
			utils.Warnf("network", "[TEST MODE] Bond从属接口不应配置IP地址，将忽略IP配置: %s", iface.Name)
		}
	}

	// 模拟配置逻辑：总是返回配置已变更
	utils.Infof("network", "[TEST MODE] openEuler ifcfg configuration simulated for interface %s", iface.Name)
	return true, nil
}

func (oif *OpenEulerIfupdown) mockReloadIfy() error {
	// 模拟重载逻辑：总是成功
	utils.Info("network", "[TEST MODE] openEuler network service restart simulated")
	return nil
}
