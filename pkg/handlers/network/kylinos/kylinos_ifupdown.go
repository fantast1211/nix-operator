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
	osInfo *controller.OSInfo
}

// NewKylinOSIfupdown 创建 KylinOS ifupdown 实例
func NewKylinOSIfupdown(osInfo *controller.OSInfo) *KylinOSIfupdown {
	return &KylinOSIfupdown{
		osInfo: osInfo,
	}
}

// IsInstall 检查 ifupdown 是否已安装
func (i *KylinOSIfupdown) IsInstall(ctx context.Context) bool {

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
	if _, err := os.Stat("/etc/sysconfig/network-scripts"); os.IsNotExist(err) {
		utils.Debug("network", "KylinOS ifupdown network-scripts directory not found")
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

	// 确定配置文件路径
	configPath := fmt.Sprintf("/etc/sysconfig/network-scripts/ifcfg-%s", iface.Name)
	utils.Infof("network", "KylinOS ifupdown config file path: %s", configPath)

	// 检查配置是否有变更
	newConfigData := []byte(configContent)
	utils.Info("network", "Checking if KylinOS ifupdown configuration file exists")
	needsUpdate := false
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		utils.Info("network", "KylinOS ifupdown configuration file exists, comparing content")
		// 文件存在，比较内容
		if !bytes.Equal(existingData, newConfigData) {
			needsUpdate = true
			utils.Info("network", "KylinOS ifupdown configuration content has changed")
		}
	} else {
		utils.Info("network", "KylinOS ifupdown configuration file does not exist, will create new one")
		needsUpdate = true
	}

	// 获取当前接口实际状态
	currentStatus, err := i.getCurrentInterfaceStatus(ctx, iface.Name)
	statusMatches := true // 默认假设状态匹配，如果无法获取状态则依赖文件检查
	if err != nil {
		utils.Debugf("network", "Failed to get current interface status for %s: %v", iface.Name, err)
		// 如果无法获取当前状态，继续使用文件检查结果
	} else {
		// 比较期望配置与当前实际状态
		statusMatches = i.compareInterfaceConfig(&iface, currentStatus)
		utils.Infof("network", "Interface %s status comparison: file_needs_update=%t, status_matches=%t", iface.Name, needsUpdate, statusMatches)
	}

	// 如果配置文件不需要更新且当前状态匹配期望配置，则无需重新加载
	if !needsUpdate && statusMatches {
		utils.Infof("network", "KylinOS ifupdown config and status for %s are both up to date", iface.Name)
		return false, nil
	}

	// 如果配置文件不需要更新但状态不匹配，需要触发重载
	if !needsUpdate && !statusMatches {
		utils.Infof("network", "KylinOS ifupdown config for %s unchanged but interface status differs, triggering reload", iface.Name)
		return true, nil
	}

	// 如果配置文件需要更新，继续写入文件
	if !needsUpdate {
		utils.Infof("network", "KylinOS ifupdown config for %s unchanged", iface.Name)
		return false, nil
	}

	// 创建配置目录
	utils.Info("network", "Creating KylinOS ifupdown configuration directory")
	if err := os.MkdirAll("/etc/sysconfig/network-scripts", 0755); err != nil {
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

	utils.Info("network", "Reloading KylinOS network configuration with ifupdown")

	// 重启网络服务
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	commands := [][]string{
		{"systemctl", "restart", "networking"},
		{"systemctl", "restart", "network"},
	}

	var lastErr error
	var failedCommands []string

	for _, cmdArgs := range commands {
		cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
		if err := cmd.Run(); err == nil {
			utils.Infof("network", "KylinOS network reloaded successfully with: %s", strings.Join(cmdArgs, " "))
			return nil
		} else {
			lastErr = err
			failedCmd := strings.Join(cmdArgs, " ")
			failedCommands = append(failedCommands, failedCmd)
			utils.Errorf("network", "Failed to reload KylinOS network with %s: %v", failedCmd, err)
		}
	}

	return fmt.Errorf("failed to reload KylinOS network configuration, tried commands: %v, last error: %v", failedCommands, lastErr)
}

// getCurrentInterfaceStatus 获取当前网络接口的实际状态
func (kif *KylinOSIfupdown) getCurrentInterfaceStatus(ctx context.Context, ifaceName string) (*types.Interface, error) {
	currentIface := &types.Interface{
		Name: ifaceName,
	}

	// 使用 ip 命令获取接口状态
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 获取接口的IP地址信息
	cmd := exec.CommandContext(ctx, "ip", "addr", "show", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		utils.Debugf("network", "Failed to get interface %s address info: %v", ifaceName, err)
		return currentIface, nil // 返回空状态，不报错
	}

	// 解析 ip addr 输出
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "inet ") {
			// IPv4 地址行: inet 192.168.1.100/24 brd 192.168.1.255 scope global eth0
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				currentIface.IPv4Address = fields[1] // 包含CIDR格式
			}
		} else if strings.Contains(line, "inet6 ") && !strings.Contains(line, "scope link") {
			// IPv6 地址行: inet6 2001:db8::1/64 scope global
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				currentIface.IPv6Address = fields[1] // 包含CIDR格式
			}
		} else if strings.Contains(line, "mtu ") {
			// MTU信息: mtu 1500
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "mtu" && i+1 < len(fields) {
					if mtu, err := strconv.Atoi(fields[i+1]); err == nil {
						currentIface.MTU = mtu
					}
					break
				}
			}
		}
	}

	// 获取默认网关信息
	cmd = exec.CommandContext(ctx, "ip", "route", "show", "default")
	output, err = cmd.Output()
	if err == nil {
		lines = strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "dev "+ifaceName) {
				// 默认路由行: default via 192.168.1.1 dev eth0
				fields := strings.Fields(line)
				for i, field := range fields {
					if field == "via" && i+1 < len(fields) {
						currentIface.IPv4Gateway = fields[i+1]
						break
					}
				}
				break
			}
		}
	}

	// 获取DNS服务器信息
	if data, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		lines = strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "nameserver ") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					currentIface.Nameservers = append(currentIface.Nameservers, fields[1])
				}
			}
		}
	}

	return currentIface, nil
}

// compareInterfaceConfig 比较期望配置与当前实际状态
func (kif *KylinOSIfupdown) compareInterfaceConfig(expected, current *types.Interface) bool {
	// 比较 IPv4 地址
	if !kif.compareIPAddress(expected.IPv4Address, current.IPv4Address) {
		utils.Infof("network", "IPv4 address differs: expected=%s, current=%s", expected.IPv4Address, current.IPv4Address)
		return false
	}

	// 比较 IPv6 地址
	if !kif.compareIPAddress(expected.IPv6Address, current.IPv6Address) {
		utils.Infof("network", "IPv6 address differs: expected=%s, current=%s", expected.IPv6Address, current.IPv6Address)
		return false
	}

	// 比较网关
	if expected.IPv4Gateway != current.IPv4Gateway {
		utils.Infof("network", "IPv4 gateway differs: expected=%s, current=%s", expected.IPv4Gateway, current.IPv4Gateway)
		return false
	}

	// 比较 MTU
	if expected.MTU != 0 && expected.MTU != current.MTU {
		utils.Infof("network", "MTU differs: expected=%d, current=%d", expected.MTU, current.MTU)
		return false
	}

	// 比较 DNS 服务器
	if !kif.compareStringSlices(expected.Nameservers, current.Nameservers) {
		utils.Infof("network", "DNS servers differ: expected=%v, current=%v", expected.Nameservers, current.Nameservers)
		return false
	}

	return true
}

// compareIPAddress 比较 IP 地址，支持 CIDR 格式
func (kif *KylinOSIfupdown) compareIPAddress(expected, current string) bool {
	if expected == "" && current == "" {
		return true
	}
	if expected == "" || current == "" {
		return expected == current
	}

	// 标准化 IP 地址格式
	expectedNorm := kif.normalizeIPAddress(expected)
	currentNorm := kif.normalizeIPAddress(current)

	return expectedNorm == currentNorm
}

// normalizeIPAddress 标准化 IP 地址格式
func (kif *KylinOSIfupdown) normalizeIPAddress(addr string) string {
	if addr == "" {
		return ""
	}

	// 如果包含 CIDR，解析并重新格式化
	if strings.Contains(addr, "/") {
		ip, ipNet, err := net.ParseCIDR(addr)
		if err == nil {
			// 返回 IP/prefix 格式，确保 IP 地址和子网掩码都被考虑
			prefixLen, _ := ipNet.Mask.Size()
			return fmt.Sprintf("%s/%d", ip.String(), prefixLen)
		}
	}

	// 如果是纯 IP 地址，尝试解析
	if ip := net.ParseIP(addr); ip != nil {
		return ip.String()
	}

	return addr
}

// compareStringSlices 比较字符串切片
func (kif *KylinOSIfupdown) compareStringSlices(expected, current []string) bool {
	if len(expected) != len(current) {
		return false
	}

	// 创建映射来比较（忽略顺序）
	expectedMap := make(map[string]bool)
	for _, item := range expected {
		expectedMap[item] = true
	}

	for _, item := range current {
		if !expectedMap[item] {
			return false
		}
	}

	return true
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
	utils.Infof("network", "Template data prepared: HasBonding=%t, IPv4IP=%s, IPv6IP=%s", templateData.HasBonding, templateData.IPv4IP, templateData.IPv6IP)

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
	types.Interface
	IPv4IP      string
	IPv4Netmask string
	IPv6IP      string
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
			data.IPv4IP = ip.String()
			data.IPv4Netmask = i.cidrToNetmask(ipNet)
		}
	}

	// 处理 IPv6 地址
	if iface.IPv6Address != "" {
		if ip, ipNet, err := net.ParseCIDR(iface.IPv6Address); err == nil {
			data.IPv6IP = ip.String()
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
