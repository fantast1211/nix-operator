package openeuler

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// OpenEulerBondIfupdown openEuler系统Bond ifupdown管理器
// 专注于Bond虚拟网卡配置，只负责主接口配置
type OpenEulerBondIfupdown struct {
	osInfo *controller.OSInfo
}

//go:embed openeuler_bond_ifcfg.tpl
var openeulerBondIfcfgTemplate string

// BondIfcfgData Bond主接口配置模板数据
type BondIfcfgData struct {
	Name        string
	Mode        string
	Miimon      int
	IPv4IP      string
	IPv4Netmask string
	IPv4Gateway string
	MTU         int
	Nameservers []string
}

// NewOpenEulerBondIfupdown 创建openEuler Bond Ifupdown实例
func NewOpenEulerBondIfupdown(osInfo *controller.OSInfo) *OpenEulerBondIfupdown {
	return &OpenEulerBondIfupdown{
		osInfo: osInfo,
	}
}

func (obi *OpenEulerBondIfupdown) IsInstall(ctx context.Context) bool {
	// 检查传统网络脚本目录是否存在
	networkScriptsDir := "/etc/sysconfig/network-scripts"
	if _, err := os.Stat(networkScriptsDir); err != nil {
		utils.Debug("bond", "network-scripts directory not found")
		return false
	}

	// 检查ifup/ifdown命令是否存在
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "which", "ifup")
	if err := cmd.Run(); err != nil {
		utils.Debug("bond", "ifup command not found")
		return false
	}

	cmd = exec.CommandContext(ctx, "which", "ifdown")
	if err := cmd.Run(); err != nil {
		utils.Debug("bond", "ifdown command not found")
		return false
	}

	// 检查bonding内核模块支持
	cmd = exec.CommandContext(ctx, "modinfo", "bonding")
	if err := cmd.Run(); err != nil {
		utils.Debug("bond", "bonding kernel module not available")
		return false
	}

	// openEuler特定检查：验证网络服务
	if obi.osInfo != nil {
		if obi.osInfo.ID == "openeuler" && obi.osInfo.VersionID != "" {
			// 检查network服务状态（可能不是必需的，但有助于确认）
			cmd = exec.CommandContext(ctx, "systemctl", "is-enabled", "network")
			if output, err := cmd.Output(); err == nil {
				status := strings.TrimSpace(string(output))
				utils.Infof("bond", "OpenEuler Bond Ifupdown network service status: %s", status)
			}
			utils.Infof("bond", "OpenEuler Bond Ifupdown detected")
		}
	}

	return true
}

func (obi *OpenEulerBondIfupdown) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := obi.ConfigureWithCheck(ctx, bondConfig)
	return err
}

func (obi *OpenEulerBondIfupdown) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	// 验证Bond名称不能为空
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 只配置Bond主接口，从接口配置由network模块处理
	bondChanged, err := obi.configureBondInterface(bondConfig)
	if err != nil {
		return false, fmt.Errorf("failed to configure bond interface: %v", err)
	}

	// 获取当前接口状态
	currentStatus, err := obi.getCurrentInterfaceStatus(ctx, bondConfig.Name)
	if err != nil {
		utils.Debugf("bond", "Failed to get current interface status for bond %s: %v", bondConfig.Name, err)
	}

	// 比较期望配置与当前状态
	statusMatches := obi.compareBondConfig(bondConfig, currentStatus)

	// 双重检查：配置文件未变更且当前状态匹配期望配置
	if !bondChanged && statusMatches {
		utils.Debugf("bond", "OpenEuler Bond Ifupdown config and interface status for bond %s both match expected, skipping reload", bondConfig.Name)
		return false, nil // 无需重新加载
	}

	if bondChanged {
		utils.Infof("bond", "OpenEuler Bond Ifupdown configuration written for bond %s", bondConfig.Name)
	} else {
		utils.Debugf("bond", "OpenEuler Bond Ifupdown config for bond %s unchanged", bondConfig.Name)
	}

	if !statusMatches {
		utils.Infof("bond", "OpenEuler Bond interface %s status does not match expected configuration, reload required", bondConfig.Name)
	}

	return true, nil // 需要重新加载
}

func (obi *OpenEulerBondIfupdown) configureBondInterface(bondConfig types.BondConfig) (bool, error) {
	// 准备模板数据
	templateData := BondIfcfgData{
		Name:   bondConfig.Name,
		Mode:   types.GetBondModeName(bondConfig.Mode),
		Miimon: bondConfig.Miimon,
		MTU:    bondConfig.Network.MTU,
	}

	// 解析IPv4地址和子网掩码
	if bondConfig.Network.IP != "" {
		if strings.Contains(bondConfig.Network.IP, "/") {
			// CIDR格式：192.168.1.100/24
			ip, ipNet, err := net.ParseCIDR(bondConfig.Network.IP)
			if err != nil {
				return false, fmt.Errorf("failed to parse IPv4 CIDR %s: %v", bondConfig.Network.IP, err)
			}
			templateData.IPv4IP = ip.String()
			templateData.IPv4Netmask = obi.cidrToNetmask(ipNet)
		} else {
			// 纯IP地址格式
			templateData.IPv4IP = bondConfig.Network.IP
		}
	}

	// 设置网关和DNS
	templateData.IPv4Gateway = bondConfig.Network.Gateway
	templateData.Nameservers = bondConfig.Network.DNSServers

	// 获取模板内容
	templateContent, err := utils.GetTemplateContent("openeuler_bond_ifcfg.tpl", openeulerBondIfcfgTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to get openEuler Bond Ifcfg template: %v", err)
	}

	// 解析模板，添加必要的函数
	tmpl, err := template.New("openeuler_bond_ifcfg").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse openEuler Bond Ifcfg template: %v", err)
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return false, fmt.Errorf("failed to execute openEuler Bond Ifcfg template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := filepath.Join("/etc/sysconfig/network-scripts", fmt.Sprintf("ifcfg-%s", bondConfig.Name))
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			return false, nil // 配置未变更
		}
	}

	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return false, fmt.Errorf("failed to create directory %s: %v", filepath.Dir(configPath), err)
	}

	// 写入Bond主接口配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write openEuler Bond Ifcfg config: %v", err)
	}

	return true, nil // 配置已变更
}

func (obi *OpenEulerBondIfupdown) ReloadIfy(ctx context.Context) error {
	// openEuler特定的Bond网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 1. 加载bonding内核模块
	cmd := exec.CommandContext(ctx, "modprobe", "bonding")
	if output, err := cmd.CombinedOutput(); err != nil {
		utils.Warnf("bond", "Failed to load bonding module: %v, output: %s", err, string(output))
	}

	// 2. 尝试重新加载网络配置（更温和的方式）
	cmd = exec.CommandContext(ctx, "systemctl", "reload-or-restart", "network")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 如果network服务不可用，尝试使用NetworkManager
		utils.Warnf("bond", "Failed to restart network service: %v, trying NetworkManager", err)
		cmd = exec.CommandContext(ctx, "systemctl", "restart", "NetworkManager")
		output, err = cmd.CombinedOutput()
		if err != nil {
			// 如果NetworkManager也失败，尝试手动启动bond接口
			utils.Warnf("bond", "Failed to restart NetworkManager: %v, trying manual interface activation", err)
			return obi.manualActivateBond(ctx)
		}
	}

	// 3. openEuler特定：等待网络稳定
	time.Sleep(5 * time.Second)

	// 4. 验证bond接口是否正确创建
	cmd = exec.CommandContext(ctx, "ip", "link", "show", "type", "bond")
	output, err = cmd.CombinedOutput()
	if err != nil {
		utils.Warnf("bond", "Failed to verify bond interfaces: %v", err)
	} else {
		utils.Infof("bond", "Bond interfaces status: %s", string(output))
	}

	// 5. 检查bond模块是否加载
	cmd = exec.CommandContext(ctx, "lsmod")
	output, err = cmd.CombinedOutput()
	if err == nil {
		if strings.Contains(string(output), "bonding") {
			utils.Infof("bond", "Bonding kernel module is loaded")
		} else {
			utils.Warnf("bond", "Bonding kernel module may not be loaded")
		}
	}

	utils.Infof("bond", "OpenEuler Bond Ifupdown configuration applied successfully")
	return nil
}

// cidrToNetmask 将CIDR转换为子网掩码
func (obi *OpenEulerBondIfupdown) cidrToNetmask(ipNet *net.IPNet) string {
	mask := ipNet.Mask
	if len(mask) == 4 {
		return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	}
	return ""
}

// manualActivateBond 手动激活bond接口
func (obi *OpenEulerBondIfupdown) manualActivateBond(ctx context.Context) error {
	utils.Infof("bond", "Attempting manual bond interface activation")
	
	// 1. 尝试使用ifup命令激活所有bond接口
	cmd := exec.CommandContext(ctx, "bash", "-c", "for f in /etc/sysconfig/network-scripts/ifcfg-bond*; do [ -f \"$f\" ] && ifup $(basename $f | sed 's/ifcfg-//'); done")
	if output, err := cmd.CombinedOutput(); err != nil {
		utils.Warnf("bond", "Failed to activate bond interfaces with ifup: %v, output: %s", err, string(output))
	} else {
		utils.Infof("bond", "Bond interfaces activated with ifup")
	}
	
	// 2. 等待接口稳定
	time.Sleep(3 * time.Second)
	
	// 3. 验证bond接口是否创建成功
	cmd = exec.CommandContext(ctx, "ip", "link", "show", "type", "bond")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to verify bond interfaces after manual activation: %v", err)
	}
	
	if len(strings.TrimSpace(string(output))) == 0 {
		return fmt.Errorf("no bond interfaces found after manual activation")
	}
	
	utils.Infof("bond", "Manual bond activation completed successfully")
	return nil
}



// getCurrentInterfaceStatus 获取当前Bond接口状态
func (obi *OpenEulerBondIfupdown) getCurrentInterfaceStatus(ctx context.Context, bondName string) (types.BondConfig, error) {
	var currentStatus types.BondConfig
	currentStatus.Name = bondName

	// 使用ip命令获取接口信息
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 获取IP地址
	cmd := exec.CommandContext(ctx, "ip", "addr", "show", bondName)
	output, err := cmd.Output()
	if err == nil {
		// 解析IP地址
		re := regexp.MustCompile(`inet\s+(\S+)`)
		matches := re.FindStringSubmatch(string(output))
		if len(matches) > 1 {
			currentStatus.Network.IP = matches[1]
		}
	}

	// 获取网关
	cmd = exec.CommandContext(ctx, "ip", "route", "show", "default")
	output, err = cmd.Output()
	if err == nil {
		// 解析默认网关
		re := regexp.MustCompile(`default\s+via\s+(\S+)`)
		matches := re.FindStringSubmatch(string(output))
		if len(matches) > 1 {
			currentStatus.Network.Gateway = matches[1]
		}
	}

	// 获取DNS服务器
	if data, err := ioutil.ReadFile("/etc/resolv.conf"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "nameserver") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					currentStatus.Network.DNSServers = append(currentStatus.Network.DNSServers, parts[1])
				}
			}
		}
	}

	// 获取MTU
	cmd = exec.CommandContext(ctx, "ip", "link", "show", bondName)
	output, err = cmd.Output()
	if err == nil {
		// 解析MTU
		re := regexp.MustCompile(`mtu\s+(\d+)`)
		matches := re.FindStringSubmatch(string(output))
		if len(matches) > 1 {
			if mtu, err := strconv.Atoi(matches[1]); err == nil {
				currentStatus.Network.MTU = mtu
			}
		}
	}

	return currentStatus, nil
}

// compareBondConfig 比较期望配置与当前状态
func (obi *OpenEulerBondIfupdown) compareBondConfig(expected types.BondConfig, current types.BondConfig) bool {
	// 比较IP地址
	if !obi.compareIPAddress(expected.Network.IP, current.Network.IP) {
		utils.Debugf("bond", "IP address mismatch: expected %s, current %s", expected.Network.IP, current.Network.IP)
		return false
	}

	// 比较网关
	if expected.Network.Gateway != current.Network.Gateway {
		utils.Debugf("bond", "Gateway mismatch: expected %s, current %s", expected.Network.Gateway, current.Network.Gateway)
		return false
	}

	// 比较MTU
	if expected.Network.MTU != 0 && expected.Network.MTU != current.Network.MTU {
		utils.Debugf("bond", "MTU mismatch: expected %d, current %d", expected.Network.MTU, current.Network.MTU)
		return false
	}

	// 比较DNS服务器
	if !obi.compareStringSlices(expected.Network.DNSServers, current.Network.DNSServers) {
		utils.Debugf("bond", "DNS servers mismatch: expected %v, current %v", expected.Network.DNSServers, current.Network.DNSServers)
		return false
	}

	return true
}

// compareIPAddress 比较IP地址，支持CIDR格式
func (obi *OpenEulerBondIfupdown) compareIPAddress(expected, current string) bool {
	if expected == "" && current == "" {
		return true
	}
	if expected == "" || current == "" {
		return false
	}

	// 标准化IP地址格式
	expectedNorm := obi.normalizeIPAddress(expected)
	currentNorm := obi.normalizeIPAddress(current)

	return expectedNorm == currentNorm
}

// normalizeIPAddress 标准化IP地址格式
func (obi *OpenEulerBondIfupdown) normalizeIPAddress(ipStr string) string {
	if ipStr == "" {
		return ""
	}

	// 尝试解析CIDR格式
	if strings.Contains(ipStr, "/") {
		_, ipNet, err := net.ParseCIDR(ipStr)
		if err == nil {
			return ipNet.String()
		}
	}

	// 尝试解析纯IP地址
	ip := net.ParseIP(ipStr)
	if ip != nil {
		return ip.String()
	}

	return ipStr
}

// compareStringSlices 比较字符串切片，忽略顺序
func (obi *OpenEulerBondIfupdown) compareStringSlices(expected, current []string) bool {
	if len(expected) != len(current) {
		return false
	}

	// 创建副本并排序
	expectedCopy := make([]string, len(expected))
	currentCopy := make([]string, len(current))
	copy(expectedCopy, expected)
	copy(currentCopy, current)

	sort.Strings(expectedCopy)
	sort.Strings(currentCopy)

	// 逐一比较
	for i := range expectedCopy {
		if expectedCopy[i] != currentCopy[i] {
			return false
		}
	}

	return true
}