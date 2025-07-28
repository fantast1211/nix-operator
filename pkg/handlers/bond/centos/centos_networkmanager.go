package centos

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

// CentOSBondNetworkManager CentOS 7.2-7.9 专用的 Bond NetworkManager 实现
type CentOSBondNetworkManager struct {
	osInfo *controller.OSInfo
}

//go:embed centos_bond_nmconnection.tpl
var centosBondNmConnectionTemplate string

// NewCentOSBondNetworkManager 创建 CentOS Bond NetworkManager 实例
func NewCentOSBondNetworkManager(osInfo *controller.OSInfo) *CentOSBondNetworkManager {
	return &CentOSBondNetworkManager{
		osInfo: osInfo,
	}
}

func (cbnm *CentOSBondNetworkManager) IsInstall(ctx context.Context) bool {
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

	// CentOS 7.2-7.9 特定检查：验证 NetworkManager 版本兼容性
	if cbnm.osInfo != nil {
		if cbnm.osInfo.ID == "centos" && cbnm.osInfo.VersionID != "" {
			// 检查 nmcli 版本，确保支持所需功能
			cmd = exec.CommandContext(ctx, "nmcli", "--version")
			if cmd.Run() != nil {
				utils.Warnf("bond", "nmcli command not available")
				return false
			}
			utils.Infof("bond", "CentOS Bond NetworkManager detected and verified")
		}
	}

	return true
}

func (cbnm *CentOSBondNetworkManager) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := cbnm.ConfigureWithCheck(ctx, bondConfig)
	return err
}

func (cbnm *CentOSBondNetworkManager) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	// 验证Bond名称不能为空
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 获取模板内容，优先使用外部模板
	templateContent, err := utils.GetTemplateContent("centos_bond_nmconnection.tpl", centosBondNmConnectionTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to get CentOS Bond NetworkManager template: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("centos_bond_nmconnection").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse CentOS Bond NetworkManager template: %v", err)
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, bondConfig); err != nil {
		return false, fmt.Errorf("failed to execute CentOS Bond NetworkManager template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/nix-operator-bond-%s.nmconnection", bondConfig.Name)
	existingData, err := os.ReadFile(configPath)
	fileChanged := true
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Debugf("bond", "CentOS Bond NetworkManager config for bond %s unchanged", bondConfig.Name)
			fileChanged = false
		}
	}

	// 获取当前接口状态
	currentStatus, err := cbnm.getCurrentInterfaceStatus(ctx, bondConfig.Name)
	if err != nil {
		utils.Warnf("bond", "Failed to get current interface status for bond %s: %v", bondConfig.Name, err)
		// 继续执行，但假设需要重新加载
	} else {
		// 比较期望配置与当前状态
		if !fileChanged && cbnm.compareBondConfig(bondConfig, currentStatus) {
			utils.Infof("bond", "CentOS Bond NetworkManager config and interface status for bond %s are up-to-date, skipping reload", bondConfig.Name)
			return false, nil // 配置和状态都未变更
		}
	}

	// 写入连接配置文件（如果有变更）
	if fileChanged {
		if err := utils.AtomicWriteFile(newConfigData, configPath, 0600); err != nil {
			return false, fmt.Errorf("failed to write CentOS Bond NetworkManager config: %v", err)
		}
		utils.Infof("bond", "CentOS Bond NetworkManager configuration written for bond %s", bondConfig.Name)
	}

	return true, nil // 配置已变更或需要重新加载
}

func (cbnm *CentOSBondNetworkManager) ReloadIfy(ctx context.Context) error {
	// CentOS 7.2-7.9 特定的Bond网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. 重新加载配置
	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload CentOS Bond NetworkManager config: %v, output: %s", err, string(output))
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
			// CentOS 7.x 可能需要更长的等待时间
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

	// 4. CentOS 7.x 特定：等待网络稳定
	time.Sleep(2 * time.Second)

	utils.Infof("bond", "CentOS Bond NetworkManager configuration reloaded successfully")
	return nil
}

// getCurrentInterfaceStatus 获取当前Bond接口状态
func (cbnm *CentOSBondNetworkManager) getCurrentInterfaceStatus(ctx context.Context, bondName string) (types.BondConfig, error) {
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
func (cbnm *CentOSBondNetworkManager) compareBondConfig(expected types.BondConfig, current types.BondConfig) bool {
	// 比较IP地址
	if !cbnm.compareIPAddress(expected.Network.IP, current.Network.IP) {
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
	if !cbnm.compareStringSlices(expected.Network.DNSServers, current.Network.DNSServers) {
		utils.Debugf("bond", "DNS servers mismatch: expected %v, current %v", expected.Network.DNSServers, current.Network.DNSServers)
		return false
	}

	return true
}

// compareIPAddress 比较IP地址，支持CIDR格式
func (cbnm *CentOSBondNetworkManager) compareIPAddress(expected, current string) bool {
	if expected == "" && current == "" {
		return true
	}
	if expected == "" || current == "" {
		return false
	}

	// 标准化IP地址格式
	expectedNorm := cbnm.normalizeIPAddress(expected)
	currentNorm := cbnm.normalizeIPAddress(current)

	return expectedNorm == currentNorm
}

// normalizeIPAddress 标准化IP地址格式
func (cbnm *CentOSBondNetworkManager) normalizeIPAddress(ipStr string) string {
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
func (cbnm *CentOSBondNetworkManager) compareStringSlices(expected, current []string) bool {
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
