package centos

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

// CentOSIfupdown CentOS 7.2-7.9 专用的传统网络脚本实现
type CentOSIfupdown struct {
	osInfo *controller.OSInfo
}

//go:embed centos_ifcfg.tpl
var centosIfcfgTemplate string

// CentOSIfcfgData 用于模板渲染的数据结构
type CentOSIfcfgData struct {
	types.Interface
	IPv4IP      string // 分离出的 IPv4 地址
	IPv4Netmask string // 分离出的子网掩码
	IPv6IP      string // 分离出的 IPv6 地址
	IPv6Prefix  string // 分离出的 IPv6 前缀长度
}

// NewCentOSIfupdown 创建 CentOS Ifupdown 实例
func NewCentOSIfupdown(osInfo *controller.OSInfo) *CentOSIfupdown {
	return &CentOSIfupdown{
		osInfo: osInfo,
	}
}

func (cif *CentOSIfupdown) IsInstall(ctx context.Context) bool {
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

	// 检查network服务是否存在且启用（CentOS 7.x系统）
	cmd = exec.CommandContext(ctx2, "systemctl", "is-enabled", "network")
	if cmd.Run() == nil {
		utils.Info("network", "CentOS traditional network scripts detected and verified")
		return true
	}

	// 如果没有其他网络管理器在运行，且传统工具存在，则认为可用
	utils.Info("network", "CentOS ifupdown tools available as fallback")
	return true
}

func (cif *CentOSIfupdown) Configure(ctx context.Context, iface types.Interface) error {
	_, err := cif.ConfigureWithCheck(ctx, iface)
	return err
}

func (cif *CentOSIfupdown) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	utils.Infof("network", "Starting CentOS ifupdown configuration for interface %s", iface.Name)

	// 验证接口名称不能为空
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}
	utils.Infof("network", "Interface %s details: IPv4=%s, IPv6=%s, MTU=%d", iface.Name, iface.IPv4Address, iface.IPv6Address, iface.MTU)

	// 首先获取当前接口的实际状态
	utils.Infof("network", "Checking current interface status for %s", iface.Name)
	currentStatus, err := cif.getCurrentInterfaceStatus(ctx, iface.Name)
	if err != nil {
		utils.Warnf("network", "Failed to get current interface status for %s: %v", iface.Name, err)
	} else {
		utils.Infof("network", "Current interface %s status: IPv4=%s, IPv6=%s, Gateway4=%s, MTU=%d", 
			iface.Name, currentStatus.IPv4Address, currentStatus.IPv6Address, 
			currentStatus.IPv4Gateway, currentStatus.MTU)
		
		// 比较期望配置与当前状态
		if cif.compareInterfaceConfig(&iface, currentStatus) {
			utils.Infof("network", "Interface %s configuration matches current status, no changes needed", iface.Name)
			return false, nil // 配置未变更
		}
		utils.Infof("network", "Interface %s configuration differs from current status, update needed", iface.Name)
	}

	// 准备模板数据，解析 CIDR 格式的地址
	utils.Info("network", "Preparing CentOS ifcfg template data")
	templateData := CentOSIfcfgData{
		Interface: iface,
	}

	// 调试日志：输出bond配置信息
	if iface.BondingSlave != nil {
		utils.Infof("network", "Bond configuration detected for interface %s: enabled=%v, master=%s",
			iface.Name, iface.BondingSlave.Enabled, iface.BondingSlave.Master)
	} else {
		utils.Infof("network", "No bond configuration for interface %s", iface.Name)
	}

	// 解析 IPv4 地址和子网掩码
	if iface.IPv4Address != "" {
		utils.Infof("network", "Parsing IPv4 address: %s", iface.IPv4Address)
		if strings.Contains(iface.IPv4Address, "/") {
			// CIDR 格式：192.168.1.100/24
			ip, ipNet, err := net.ParseCIDR(iface.IPv4Address)
			if err != nil {
				return false, fmt.Errorf("failed to parse IPv4 CIDR %s: %v", iface.IPv4Address, err)
			}
			templateData.IPv4IP = ip.String()
			templateData.IPv4Netmask = cif.cidrToNetmask(ipNet)
			utils.Infof("network", "IPv4 parsed: IP=%s, Netmask=%s", templateData.IPv4IP, templateData.IPv4Netmask)
		} else {
			// 纯 IP 地址格式
			templateData.IPv4IP = iface.IPv4Address
			utils.Infof("network", "IPv4 address (no CIDR): %s", templateData.IPv4IP)
		}
	}

	// 解析 IPv6 地址和前缀
	if iface.IPv6Address != "" {
		utils.Infof("network", "Parsing IPv6 address: %s", iface.IPv6Address)
		if strings.Contains(iface.IPv6Address, "/") {
			// CIDR 格式：2001:db8::1/64
			ip, ipNet, err := net.ParseCIDR(iface.IPv6Address)
			if err != nil {
				return false, fmt.Errorf("failed to parse IPv6 CIDR %s: %v", iface.IPv6Address, err)
			}
			templateData.IPv6IP = ip.String()
			prefixLen, _ := ipNet.Mask.Size()
			templateData.IPv6Prefix = strconv.Itoa(prefixLen)
			utils.Infof("network", "IPv6 parsed: IP=%s, Prefix=%s", templateData.IPv6IP, templateData.IPv6Prefix)
		} else {
			// 纯 IP 地址格式
			templateData.IPv6IP = iface.IPv6Address
			utils.Infof("network", "IPv6 address (no CIDR): %s", templateData.IPv6IP)
		}
	}

	// 获取模板内容
	utils.Info("network", "Loading CentOS ifcfg template")
	templateContent, err := utils.GetTemplateContent("centos_ifcfg.tpl", centosIfcfgTemplate)
	if err != nil {
		return false, err
	}

	// 解析模板
	utils.Info("network", "Parsing CentOS ifcfg template")
	tmpl, err := template.New("centos_ifcfg").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse CentOS ifcfg template: %v", err)
	}

	// 渲染模板
	utils.Info("network", "Rendering CentOS ifcfg template")
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return false, fmt.Errorf("failed to execute CentOS ifcfg template: %v", err)
	}
	utils.Info("network", "CentOS ifcfg template rendered successfully")

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/sysconfig/network-scripts/ifcfg-%s", iface.Name)
	utils.Infof("network", "CentOS ifcfg config file path: %s", configPath)

	utils.Info("network", "Checking if CentOS ifcfg configuration file exists")
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		utils.Info("network", "CentOS ifcfg configuration file exists, comparing content")
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Infof("network", "CentOS ifcfg config for interface %s unchanged, skipping write", iface.Name)
			return false, nil // 配置未变更
		}
		utils.Info("network", "CentOS ifcfg configuration content has changed")
	} else {
		utils.Info("network", "CentOS ifcfg configuration file does not exist, will create new one")
	}

	// 确保目标目录存在
	utils.Infof("network", "Ensuring directory exists: %s", filepath.Dir(configPath))
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return false, fmt.Errorf("failed to create directory %s: %v", filepath.Dir(configPath), err)
	}

	// 写入配置文件
	utils.Infof("network", "Writing CentOS ifcfg configuration to file: %s", configPath)
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write CentOS ifcfg config: %v", err)
	}

	// 如果配置了DNS，更新 /etc/resolv.conf
	if len(iface.Nameservers) > 0 {
		utils.Infof("network", "Updating DNS configuration with %d nameservers", len(iface.Nameservers))
		if err := cif.updateResolvConf(iface.Nameservers); err != nil {
			utils.Warnf("network", "Failed to update resolv.conf: %v", err)
		} else {
			utils.Info("network", "DNS configuration updated successfully")
		}
	}

	utils.Infof("network", "CentOS ifcfg configuration written successfully for interface %s", iface.Name)
	return true, nil // 配置已变更
}

func (cif *CentOSIfupdown) ReloadIfy(ctx context.Context) error {
	utils.Info("network", "Reloading CentOS ifupdown network configuration")
	// CentOS 7.2-7.9 特定的网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 尝试多种重启网络的方法，按优先级排序
	reloadMethods := []struct {
		name string
		cmd  []string
	}{
		{"systemctl restart network", []string{"systemctl", "restart", "network"}},
		{"service network restart", []string{"service", "network", "restart"}},
		// {"systemctl restart NetworkManager", []string{"systemctl", "restart", "NetworkManager"}},
	}

	for _, method := range reloadMethods {
		utils.Infof("network", "Trying CentOS network reload method: %s", method.name)
		cmd := exec.CommandContext(ctx, method.cmd[0], method.cmd[1:]...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			utils.Infof("network", "Successfully reloaded CentOS network using: %s", method.name)
			// 等待网络稳定
			utils.Info("network", "Waiting for network to stabilize...")
			time.Sleep(3 * time.Second)
			return nil
		}
		utils.Infof("network", "CentOS reload method %s failed: %v, output: %s", method.name, err, string(output))
	}

	return fmt.Errorf("all CentOS network reload methods failed")
}

// cidrToNetmask 将 CIDR 网络转换为子网掩码
func (cif *CentOSIfupdown) cidrToNetmask(ipNet *net.IPNet) string {
	mask := ipNet.Mask
	if len(mask) == 4 {
		// IPv4 子网掩码
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	}
	return "" // IPv6 不需要子网掩码格式
}

// updateResolvConf 更新 /etc/resolv.conf 文件
func (cif *CentOSIfupdown) updateResolvConf(nameservers []string) error {
	if len(nameservers) == 0 {
		return nil
	}

	// 构建 resolv.conf 内容
	var content strings.Builder
	content.WriteString("# Generated by nix-operator\n")
	for _, ns := range nameservers {
		content.WriteString(fmt.Sprintf("nameserver %s\n", ns))
	}

	// 写入文件
	if err := utils.AtomicWriteFile([]byte(content.String()), "/etc/resolv.conf", 0644); err != nil {
		return fmt.Errorf("failed to write resolv.conf: %v", err)
	}

	return nil
}

// getCurrentInterfaceStatus 获取当前网络接口的实际状态
func (cif *CentOSIfupdown) getCurrentInterfaceStatus(ctx context.Context, ifaceName string) (*types.Interface, error) {
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
func (cif *CentOSIfupdown) compareInterfaceConfig(expected, current *types.Interface) bool {
	// 比较 IPv4 地址
	if !cif.compareIPAddress(expected.IPv4Address, current.IPv4Address) {
		utils.Infof("network", "IPv4 address differs: expected=%s, current=%s", expected.IPv4Address, current.IPv4Address)
		return false
	}

	// 比较 IPv6 地址
	if !cif.compareIPAddress(expected.IPv6Address, current.IPv6Address) {
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
	if !cif.compareStringSlices(expected.Nameservers, current.Nameservers) {
		utils.Infof("network", "DNS servers differ: expected=%v, current=%v", expected.Nameservers, current.Nameservers)
		return false
	}

	return true
}

// compareIPAddress 比较 IP 地址，支持 CIDR 格式
func (cif *CentOSIfupdown) compareIPAddress(expected, current string) bool {
	if expected == "" && current == "" {
		return true
	}
	if expected == "" || current == "" {
		return expected == current
	}

	// 标准化 IP 地址格式
	expectedNorm := cif.normalizeIPAddress(expected)
	currentNorm := cif.normalizeIPAddress(current)

	return expectedNorm == currentNorm
}

// normalizeIPAddress 标准化 IP 地址格式
func (cif *CentOSIfupdown) normalizeIPAddress(addr string) string {
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
func (cif *CentOSIfupdown) compareStringSlices(expected, current []string) bool {
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
