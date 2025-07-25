package openeuler

import (
	"bytes"
	"context"
	_ "embed"
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

// OpenEulerNetworkManager openEuler 专用的 NetworkManager 实现
type OpenEulerNetworkManager struct {
	osInfo   *controller.OSInfo
	testMode bool // 测试模式标志
}

//go:embed openeuler_nmconnection.tpl
var openeulerNmConnectionTemplate string

// NewOpenEulerNetworkManager 创建 openEuler NetworkManager 实例
func NewOpenEulerNetworkManager(osInfo *controller.OSInfo) *OpenEulerNetworkManager {
	return &OpenEulerNetworkManager{
		osInfo:   osInfo,
		testMode: false,
	}
}

// NewOpenEulerNetworkManagerForTest 创建测试用的 openEuler NetworkManager 实例
func NewOpenEulerNetworkManagerForTest(osInfo *controller.OSInfo) *OpenEulerNetworkManager {
	return &OpenEulerNetworkManager{
		osInfo:   osInfo,
		testMode: true,
	}
}

func (onm *OpenEulerNetworkManager) IsInstall(ctx context.Context) bool {
	// 测试模式下模拟检测逻辑
	if onm.testMode {
		return onm.mockIsInstall()
	}

	// 检查NetworkManager服务是否运行
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "NetworkManager")
	output, err := cmd.Output()
	if err != nil {
		utils.Debugf("network", "NetworkManager service check failed: %v", err)
		return false
	}

	isActive := strings.TrimSpace(string(output)) == "active"
	if !isActive {
		utils.Debug("network", "NetworkManager service is not active")
		return false
	}

	// openEuler 特定检查：验证 NetworkManager 版本兼容性
	if onm.osInfo != nil {
		if onm.osInfo.ID == "openeuler" && onm.osInfo.VersionID != "" {
			// 检查 nmcli 版本，确保支持所需功能
			cmd = exec.CommandContext(ctx, "nmcli", "--version")
			if cmd.Run() != nil {
				utils.Errorf("network", "nmcli command not available")
				return false
			}
			utils.Infof("network", "openEuler NetworkManager detected and verified")
		}
	}

	return true
}

func (onm *OpenEulerNetworkManager) Configure(ctx context.Context, iface types.Interface) error {
	_, err := onm.ConfigureWithCheck(ctx, iface)
	return err
}

// getCurrentInterfaceStatus 获取当前网络接口的实际状态
func (onm *OpenEulerNetworkManager) getCurrentInterfaceStatus(ctx context.Context, ifaceName string) (*types.Interface, error) {
	if onm.testMode {
		return nil, fmt.Errorf("test mode: interface status check not implemented")
	}

	currentIface := &types.Interface{
		Name: ifaceName,
	}

	// 使用 nmcli 获取接口状态
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 获取接口的连接信息
	cmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "IP4.ADDRESS,IP6.ADDRESS,IP4.GATEWAY,IP6.GATEWAY,IP4.DNS,IP6.DNS,802-3-ETHERNET.MTU", "device", "show", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		utils.Debugf("network", "Failed to get interface %s status via nmcli: %v", ifaceName, err)
		return currentIface, nil // 返回空状态，不报错
	}

	// 解析 nmcli 输出
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "IP4.ADDRESS[1]":
			// IPv4 地址格式: 192.168.1.100/24
			if value != "" {
				currentIface.IPv4Address = value
			}
		case "IP6.ADDRESS[1]":
			// IPv6 地址格式: 2001:db8::1/64
			if value != "" {
				currentIface.IPv6Address = value
			}
		case "IP4.GATEWAY":
			if value != "" && value != "--" {
				currentIface.IPv4Gateway = value
			}
		case "IP6.GATEWAY":
			if value != "" && value != "--" {
				currentIface.IPv6Gateway = value
			}
		case "IP4.DNS[1]", "IP4.DNS[2]", "IP4.DNS[3]":
			if value != "" && value != "--" {
				currentIface.Nameservers = append(currentIface.Nameservers, value)
			}
		case "802-3-ETHERNET.MTU":
			if value != "" && value != "--" {
				if mtu, err := strconv.Atoi(value); err == nil {
					currentIface.MTU = mtu
				}
			}
		}
	}

	// 检查是否为 bond slave
	cmd = exec.CommandContext(ctx, "nmcli", "-t", "-f", "connection.slave-type,connection.master", "device", "show", ifaceName)
	output, err = cmd.Output()
	if err == nil {
		lines = strings.Split(string(output), "\n")
		slaveType := ""
		master := ""
		for _, line := range lines {
			line = strings.TrimSpace(line)
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				switch key {
				case "connection.slave-type":
					slaveType = value
				case "connection.master":
					master = value
				}
			}
		}
		if slaveType == "bond" && master != "" && master != "--" {
			currentIface.BondingSlave = &types.BondingSlaveConfig{
				Enabled: true,
				Master:  master,
			}
		}
	}

	return currentIface, nil
}

// compareInterfaceConfig 比较期望配置与当前实际状态
func (onm *OpenEulerNetworkManager) compareInterfaceConfig(expected, current *types.Interface) bool {
	// 比较 IPv4 地址
	if !onm.compareIPAddress(expected.IPv4Address, current.IPv4Address) {
		utils.Infof("network", "IPv4 address differs: expected=%s, current=%s", expected.IPv4Address, current.IPv4Address)
		return false
	}

	// 比较 IPv6 地址
	if !onm.compareIPAddress(expected.IPv6Address, current.IPv6Address) {
		utils.Infof("network", "IPv6 address differs: expected=%s, current=%s", expected.IPv6Address, current.IPv6Address)
		return false
	}

	// 比较网关
	if expected.IPv4Gateway != current.IPv4Gateway {
		utils.Infof("network", "IPv4 gateway differs: expected=%s, current=%s", expected.IPv4Gateway, current.IPv4Gateway)
		return false
	}

	if expected.IPv6Gateway != current.IPv6Gateway {
		utils.Infof("network", "IPv6 gateway differs: expected=%s, current=%s", expected.IPv6Gateway, current.IPv6Gateway)
		return false
	}

	// 比较 MTU
	if expected.MTU != 0 && expected.MTU != current.MTU {
		utils.Infof("network", "MTU differs: expected=%d, current=%d", expected.MTU, current.MTU)
		return false
	}

	// 比较 DNS 服务器
	if !onm.compareStringSlices(expected.Nameservers, current.Nameservers) {
		utils.Infof("network", "DNS servers differ: expected=%v, current=%v", expected.Nameservers, current.Nameservers)
		return false
	}

	// 比较 Bond 配置
	if !onm.compareBondConfig(expected.BondingSlave, current.BondingSlave) {
		utils.Infof("network", "Bond configuration differs")
		return false
	}

	return true
}

// compareIPAddress 比较 IP 地址，支持 CIDR 格式
func (onm *OpenEulerNetworkManager) compareIPAddress(expected, current string) bool {
	if expected == "" && current == "" {
		return true
	}
	if expected == "" || current == "" {
		return expected == current
	}

	// 标准化 IP 地址格式
	expectedNorm := onm.normalizeIPAddress(expected)
	currentNorm := onm.normalizeIPAddress(current)

	return expectedNorm == currentNorm
}

// normalizeIPAddress 标准化 IP 地址格式
func (onm *OpenEulerNetworkManager) normalizeIPAddress(addr string) string {
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
func (onm *OpenEulerNetworkManager) compareStringSlices(expected, current []string) bool {
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

// compareBondConfig 比较 Bond 配置
func (onm *OpenEulerNetworkManager) compareBondConfig(expected, current *types.BondingSlaveConfig) bool {
	if expected == nil && current == nil {
		return true
	}
	if expected == nil || current == nil {
		return false
	}

	return expected.Enabled == current.Enabled && expected.Master == current.Master
}

func (onm *OpenEulerNetworkManager) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	utils.Infof("network", "Starting openEuler NetworkManager configuration for interface %s", iface.Name)

	// 测试模式下模拟配置逻辑
	if onm.testMode {
		return onm.mockConfigureWithCheck(iface)
	}

	// 验证接口名称不能为空
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}

	utils.Infof("network", "Interface validation passed for %s", iface.Name)

	// 获取当前接口的实际状态
	utils.Infof("network", "Checking current interface status for %s", iface.Name)
	currentStatus, err := onm.getCurrentInterfaceStatus(ctx, iface.Name)
	if err != nil {
		utils.Warnf("network", "Failed to get current interface status for %s: %v", iface.Name, err)
	} else {
		utils.Infof("network", "Current interface %s status: IPv4=%s, IPv6=%s, Gateway4=%s, Gateway6=%s, MTU=%d", 
			iface.Name, currentStatus.IPv4Address, currentStatus.IPv6Address, 
			currentStatus.IPv4Gateway, currentStatus.IPv6Gateway, currentStatus.MTU)
		
		// 比较期望配置与当前状态
		if onm.compareInterfaceConfig(&iface, currentStatus) {
			utils.Infof("network", "Interface %s configuration matches current status, no changes needed", iface.Name)
			return false, nil // 配置未变更
		}
		utils.Infof("network", "Interface %s configuration differs from current status, update needed", iface.Name)
	}

	// 获取模板内容
	utils.Infof("network", "Loading openEuler NetworkManager template for interface %s", iface.Name)
	templateContent, err := utils.GetTemplateContent("openeuler_nmconnection.tpl", openeulerNmConnectionTemplate)
	if err != nil {
		return false, err
	}

	// 解析模板
	tmpl, err := template.New("openeuler_nmconnection").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse openEuler NetworkManager template: %v", err)
	}

	utils.Infof("network", "Template parsed successfully for interface %s", iface.Name)

	// 创建模板数据结构
	templateData := struct {
		Name         string
		IPv4Address  string
		IPv4Gateway  string
		IPv6Address  string
		IPv6Gateway  string
		MTU          int
		Nameservers  []string
		BondingSlave *types.BondingSlaveConfig
	}{
		Name:         iface.Name,
		IPv4Address:  iface.IPv4Address,
		IPv4Gateway:  iface.IPv4Gateway,
		IPv6Address:  iface.IPv6Address,
		IPv6Gateway:  iface.IPv6Gateway,
		MTU:          iface.MTU,
		Nameservers:  iface.Nameservers,
		BondingSlave: iface.BondingSlave,
	}

	// 记录配置详情
	if iface.BondingSlave != nil && iface.BondingSlave.Enabled {
		utils.Infof("network", "Configuring bond slave interface %s with master %s", iface.Name, iface.BondingSlave.Master)
	} else {
		utils.Infof("network", "Configuring regular interface %s with IPv4: %s, IPv6: %s", iface.Name, iface.IPv4Address, iface.IPv6Address)
	}

	// 渲染模板
	utils.Infof("network", "Rendering NetworkManager configuration template for interface %s", iface.Name)
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return false, fmt.Errorf("failed to execute openEuler NetworkManager template: %v", err)
	}

	newConfigData := []byte(content.String())
	utils.Infof("network", "Template rendered successfully for interface %s, config size: %d bytes", iface.Name, len(newConfigData))

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/nix-operator-%s.nmconnection", iface.Name)
	utils.Infof("network", "Checking existing configuration file: %s", configPath)
	existingData, err := os.ReadFile(configPath)
	configFileChanged := true
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Infof("network", "openEuler NetworkManager config file for interface %s unchanged", iface.Name)
			configFileChanged = false
		} else {
			utils.Infof("network", "Configuration file changed for interface %s, updating file", iface.Name)
		}
	} else {
		utils.Infof("network", "Configuration file does not exist for interface %s, creating new file", iface.Name)
	}

	// 如果配置文件没有变化且当前状态匹配期望配置，则无需更新
	if !configFileChanged && currentStatus != nil && onm.compareInterfaceConfig(&iface, currentStatus) {
		utils.Infof("network", "Both config file and interface status are up-to-date for %s, no changes needed", iface.Name)
		return false, nil // 配置未变更
	}

	// 写入连接配置文件
	if configFileChanged {
		utils.Infof("network", "Writing NetworkManager configuration file for interface %s to %s", iface.Name, configPath)
		if err := utils.AtomicWriteFile(newConfigData, configPath, 0600); err != nil {
			return false, fmt.Errorf("failed to write openEuler NetworkManager config: %v", err)
		}
		utils.Infof("network", "openEuler NetworkManager configuration written successfully for interface %s", iface.Name)
	} else {
		utils.Infof("network", "Configuration file for interface %s is up-to-date, but interface status needs update", iface.Name)
	}

	return true, nil // 配置已变更或需要重新应用
}

func (onm *OpenEulerNetworkManager) ReloadIfy(ctx context.Context) error {
	// 测试模式下模拟重载逻辑
	if onm.testMode {
		return onm.mockReloadIfy()
	}

	// openEuler 特定的网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. 重新加载配置
	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload openEuler NetworkManager config: %v, output: %s", err, string(output))
	}

	// 2. 获取所有nix-operator管理的连接
	cmd = exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list connections: %v", err)
	}

	// 3. 激活所有nix-operator连接
	connections := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, conn := range connections {
		if strings.HasPrefix(conn, "nix-operator-") {
			// openEuler 可能需要更长的等待时间
			ctx2, cancel2 := context.WithTimeout(ctx, 15*time.Second)
			cmd = exec.CommandContext(ctx2, "nmcli", "connection", "up", conn)
			if output, err := cmd.CombinedOutput(); err != nil {
				// 记录警告但继续处理其他连接
				utils.Errorf("network", "Failed to activate connection %s: %v, output: %s", conn, err, string(output))
			} else {
				utils.Infof("network", "Successfully activated connection %s", conn)
			}
			cancel2()
		}
	}

	// 4. openEuler 特定：等待网络稳定
	time.Sleep(2 * time.Second)

	utils.Infof("network", "openEuler NetworkManager configuration reloaded successfully")
	return nil
}

// 测试模式相关方法
func (onm *OpenEulerNetworkManager) mockIsInstall() bool {
	// 模拟检测逻辑：假设NetworkManager不可用（优先级低于ifupdown）
	utils.Info("network", "[TEST MODE] openEuler NetworkManager not detected (simulated)")
	return false
}

func (onm *OpenEulerNetworkManager) mockConfigureWithCheck(iface types.Interface) (bool, error) {
	// 模拟配置逻辑：总是返回配置已变更
	utils.Infof("network", "[TEST MODE] openEuler NetworkManager configuration simulated for interface %s", iface.Name)
	return true, nil
}

func (onm *OpenEulerNetworkManager) mockReloadIfy() error {
	// 模拟重载逻辑：总是成功
	utils.Info("network", "[TEST MODE] openEuler NetworkManager reload simulated")
	return nil
}
