package openeuler

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// OpenEulerBondNetworkManager openEuler系统Bond NetworkManager管理器
type OpenEulerBondNetworkManager struct {
	osInfo *controller.OSInfo
}

//go:embed openeuler_bond_nmconnection.tpl
var openeulerBondNmConnectionTemplate string

// NewOpenEulerBondNetworkManager 创建openEuler Bond NetworkManager实例
func NewOpenEulerBondNetworkManager(osInfo *controller.OSInfo) *OpenEulerBondNetworkManager {
	return &OpenEulerBondNetworkManager{
		osInfo: osInfo,
	}
}

func (obnm *OpenEulerBondNetworkManager) IsInstall(ctx context.Context) bool {
	// 检查NetworkManager服务是否运行
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "NetworkManager")
	output, err := cmd.Output()
	if err != nil {
		utils.Debugf("bond", "NetworkManager service check failed: %v", err)
		return false
	}

	isActive := strings.TrimSpace(string(output)) == "active"
	if !isActive {
		utils.Debug("bond", "NetworkManager service is not active")
		return false
	}

	// openEuler特定检查：验证NetworkManager版本兼容性
	if obnm.osInfo != nil {
		if obnm.osInfo.ID == "openeuler" && obnm.osInfo.VersionID != "" {
			// 检查nmcli版本，确保支持所需功能
			cmd = exec.CommandContext(ctx, "nmcli", "--version")
			if cmd.Run() != nil {
				utils.Warnf("bond", "nmcli command not available")
				return false
			}
			utils.Infof("bond", "OpenEuler Bond NetworkManager detected and verified")
		}
	}

	return true
}

func (obnm *OpenEulerBondNetworkManager) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := obnm.ConfigureWithCheck(ctx, bondConfig)
	return err
}

func (obnm *OpenEulerBondNetworkManager) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	// 验证Bond名称不能为空
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 获取模板内容，优先使用外部模板
	templateContent, err := utils.GetTemplateContent("openeuler_bond_nmconnection.tpl", openeulerBondNmConnectionTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to get openEuler Bond NetworkManager template: %v", err)
	}

	// 解析模板，添加自定义函数
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
	}
	tmpl, err := template.New("openeuler_bond_nmconnection").Funcs(funcMap).Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse openEuler Bond NetworkManager template: %v", err)
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, bondConfig); err != nil {
		return false, fmt.Errorf("failed to execute openEuler Bond NetworkManager template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/nix-operator-bond-%s.nmconnection", bondConfig.Name)
	existingData, err := os.ReadFile(configPath)
	configChanged := true
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			configChanged = false
			utils.Debugf("bond", "OpenEuler Bond NetworkManager config for bond %s unchanged", bondConfig.Name)
		}
	}

	// 获取当前接口状态
	currentStatus, err := obnm.getCurrentInterfaceStatus(ctx, bondConfig.Name)
	if err != nil {
		utils.Debugf("bond", "Failed to get current interface status for bond %s: %v", bondConfig.Name, err)
	}

	// 比较期望配置与当前状态
	statusMatches := obnm.compareBondConfig(bondConfig, currentStatus)

	// 双重检查：配置文件未变更且当前状态匹配期望配置
	if !configChanged && statusMatches {
		utils.Debugf("bond", "OpenEuler Bond NetworkManager config and interface status for bond %s both match expected, skipping reload", bondConfig.Name)
		return false, nil // 无需重新加载
	}

	// 如果配置有变更，写入新配置
	if configChanged {
		if err := utils.AtomicWriteFile(newConfigData, configPath, 0600); err != nil {
			return false, fmt.Errorf("failed to write openEuler Bond NetworkManager config: %v", err)
		}
		utils.Infof("bond", "OpenEuler Bond NetworkManager configuration written for bond %s", bondConfig.Name)
	}

	if !statusMatches {
		utils.Infof("bond", "OpenEuler Bond interface %s status does not match expected configuration, reload required", bondConfig.Name)
	}

	return true, nil // 需要重新加载
}

func (obnm *OpenEulerBondNetworkManager) ReloadIfy(ctx context.Context) error {
	// openEuler特定的Bond网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. 重新加载配置
	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload openEuler Bond NetworkManager config: %v, output: %s", err, string(output))
	}

	// 2. 获取所有nix-operator管理的bond连接
	cmd = exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list connections: %v", err)
	}

	// 3. 激活所有nix-operator bond连接
	connections := strings.Split(strings.TrimSpace(string(output)), "\n")
	var activationErrors []string
	
	for _, conn := range connections {
		if strings.HasPrefix(conn, "nix-operator-bond-") {
			// openEuler可能需要适当的等待时间
			ctx2, cancel2 := context.WithTimeout(ctx, 15*time.Second)
			cmd = exec.CommandContext(ctx2, "nmcli", "connection", "up", conn)
			if output, err := cmd.CombinedOutput(); err != nil {
				errorMsg := fmt.Sprintf("Failed to activate bond connection %s: %v, output: %s", conn, err, string(output))
				utils.Errorf("bond", errorMsg)
				activationErrors = append(activationErrors, errorMsg)
			} else {
				utils.Infof("bond", "Successfully activated bond connection %s", conn)
			}
			cancel2()
		}
	}
	
	// 如果有激活失败的连接，返回错误
	if len(activationErrors) > 0 {
		return fmt.Errorf("failed to activate %d bond connections: %s", len(activationErrors), strings.Join(activationErrors, "; "))
	}

	// 4. openEuler特定：等待网络稳定
	time.Sleep(2 * time.Second)

	utils.Infof("bond", "OpenEuler Bond NetworkManager configuration reloaded successfully")
	return nil
}

// getCurrentInterfaceStatus 获取当前Bond接口状态
func (obnm *OpenEulerBondNetworkManager) getCurrentInterfaceStatus(ctx context.Context, bondName string) (types.BondConfig, error) {
	var currentStatus types.BondConfig
	currentStatus.Name = bondName

	// 使用nmcli获取接口信息
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 获取IP地址
	cmd := exec.CommandContext(ctx, "nmcli", "-t", "-f", "IP4.ADDRESS", "device", "show", bondName)
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "IP4.ADDRESS") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					currentStatus.Network.IP = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	// 获取网关
	cmd = exec.CommandContext(ctx, "nmcli", "-t", "-f", "IP4.GATEWAY", "device", "show", bondName)
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "IP4.GATEWAY") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					currentStatus.Network.Gateway = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	// 获取DNS服务器
	cmd = exec.CommandContext(ctx, "nmcli", "-t", "-f", "IP4.DNS", "device", "show", bondName)
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "IP4.DNS") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					dnsServer := strings.TrimSpace(parts[1])
					if dnsServer != "" {
						currentStatus.Network.DNSServers = append(currentStatus.Network.DNSServers, dnsServer)
					}
				}
			}
		}
	}

	// 获取MTU
	cmd = exec.CommandContext(ctx, "nmcli", "-t", "-f", "GENERAL.MTU", "device", "show", bondName)
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "GENERAL.MTU") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					mtuStr := strings.TrimSpace(parts[1])
					if mtu, err := strconv.Atoi(mtuStr); err == nil {
						currentStatus.Network.MTU = mtu
					}
					break
				}
			}
		}
	}

	return currentStatus, nil
}

// compareBondConfig 比较期望配置与当前状态
func (obnm *OpenEulerBondNetworkManager) compareBondConfig(expected types.BondConfig, current types.BondConfig) bool {
	// 比较IP地址
	if !obnm.compareIPAddress(expected.Network.IP, current.Network.IP) {
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
	if !obnm.compareStringSlices(expected.Network.DNSServers, current.Network.DNSServers) {
		utils.Debugf("bond", "DNS servers mismatch: expected %v, current %v", expected.Network.DNSServers, current.Network.DNSServers)
		return false
	}

	return true
}

// compareIPAddress 比较IP地址，支持CIDR格式
func (obnm *OpenEulerBondNetworkManager) compareIPAddress(expected, current string) bool {
	if expected == "" && current == "" {
		return true
	}
	if expected == "" || current == "" {
		return false
	}

	// 标准化IP地址格式
	expectedNorm := obnm.normalizeIPAddress(expected)
	currentNorm := obnm.normalizeIPAddress(current)

	return expectedNorm == currentNorm
}

// normalizeIPAddress 标准化IP地址格式
func (obnm *OpenEulerBondNetworkManager) normalizeIPAddress(ipStr string) string {
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
func (obnm *OpenEulerBondNetworkManager) compareStringSlices(expected, current []string) bool {
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