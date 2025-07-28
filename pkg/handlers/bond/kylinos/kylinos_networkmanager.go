package kylinos

import (
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

	"github.com/google/uuid"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed kylinos_networkmanager.tpl
var kylinOSBondNetworkManagerTemplate string

// KylinOSBondNetworkManager KylinOS NetworkManager Bond管理器
type KylinOSBondNetworkManager struct {
	osInfo *controller.OSInfo
}

// NewKylinOSBondNetworkManager 创建KylinOS NetworkManager Bond实例
func NewKylinOSBondNetworkManager(osInfo *controller.OSInfo) *KylinOSBondNetworkManager {
	return &KylinOSBondNetworkManager{
		osInfo: osInfo,
	}
}

// IsInstall 检查NetworkManager是否已安装
func (nm *KylinOSBondNetworkManager) IsInstall(ctx context.Context) bool {
	// 检查NetworkManager服务是否运行
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, "systemctl", "is-active", "NetworkManager")
	if err := cmd.Run(); err != nil {
		return false
	}

	// 检查nmcli命令是否存在
	cmd = exec.CommandContext(ctxTimeout, "which", "nmcli")
	if err := cmd.Run(); err != nil {
		return false
	}

	// 检查NetworkManager配置目录是否存在
	if _, err := os.Stat("/etc/NetworkManager/system-connections"); err != nil {
		return false
	}

	return true
}

// Configure 配置Bond接口
func (nm *KylinOSBondNetworkManager) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := nm.ConfigureWithCheck(ctx, bondConfig)
	return err
}

// ConfigureWithCheck 配置Bond接口并检查是否有变更
func (nm *KylinOSBondNetworkManager) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	utils.Infof("bond", "Configuring KylinOS bond interface %s with NetworkManager", bondConfig.Name)

	// 生成配置内容
	configContent, err := nm.generateBondConfig(bondConfig)
	if err != nil {
		return false, fmt.Errorf("failed to generate bond config: %v", err)
	}

	// 配置文件路径
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/%s.nmconnection", bondConfig.Name)

	// 检查配置文件是否存在以及内容是否相同
	existingData, err := os.ReadFile(configPath)
	fileChanged := true
	if err == nil {
		// 文件存在，比较内容
		if string(existingData) == configContent {
			utils.Debugf("bond", "KylinOS NetworkManager bond config for %s unchanged", bondConfig.Name)
			fileChanged = false
		}
	}

	// 获取当前接口状态
	currentStatus, err := nm.getCurrentInterfaceStatus(ctx, bondConfig.Name)
	if err != nil {
		utils.Warnf("bond", "Failed to get current interface status for bond %s: %v", bondConfig.Name, err)
		// 继续执行，但假设需要重新加载
	} else {
		// 比较期望配置与当前状态
		if !fileChanged && nm.compareBondConfig(bondConfig, currentStatus) {
			utils.Infof("bond", "KylinOS NetworkManager bond config and interface status for bond %s are up-to-date, skipping reload", bondConfig.Name)
			return false, nil // 配置和状态都未变更
		}
	}

	// 写入配置文件（如果有变更）
	if fileChanged {
		if err := utils.AtomicWriteFile([]byte(configContent), configPath, 0600); err != nil {
			return false, fmt.Errorf("failed to write bond NetworkManager config: %v", err)
		}
		utils.Infof("bond", "KylinOS NetworkManager bond config for %s updated", bondConfig.Name)

		// 重新加载NetworkManager配置
		if err := nm.reloadNetworkManager(ctx); err != nil {
			utils.Warnf("bond", "Failed to reload NetworkManager: %v", err)
			// 不返回错误，因为配置文件已经更新
		}
	}

	return true, nil // 配置已变更或需要重新加载
}



// generateBondConfig 生成Bond配置内容
func (nm *KylinOSBondNetworkManager) generateBondConfig(bondConfig types.BondConfig) (string, error) {
	// 读取模板文件
	tmplPath := "/etc/nix-operator/templates/kylinos_bond_nmconnection.tpl"
	tmplContent, err := nm.getTemplateContent(tmplPath)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("kylinos_bond_nmconnection").Funcs(template.FuncMap{
		"splitCIDR": func(cidr string) []string {
			if cidr == "" {
				return []string{"", ""}
			}
			parts := strings.Split(cidr, "/")
			if len(parts) != 2 {
				return []string{cidr, ""}
			}
			return parts
		},
		"cidrToNetmask": func(prefixLen string) string {
			if prefixLen == "" {
				return ""
			}
			len, err := strconv.Atoi(prefixLen)
			if err != nil {
				return ""
			}
			mask := net.CIDRMask(len, 32)
			return net.IP(mask).String()
		},
		"generateUUID": func() string {
			return uuid.New().String()
		},
		"isTruthy": nm.isTruthy,
	}).Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	// 准备模板数据
	templateData := map[string]interface{}{
		"BondConfig": bondConfig,
	}

	// 渲染模板
	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return result.String(), nil
}

// getTemplateContent 获取模板内容
func (nm *KylinOSBondNetworkManager) getTemplateContent(tmplPath string) (string, error) {
	// 优先从外部文件读取
	if _, err := os.Stat(tmplPath); err == nil {
		content, err := os.ReadFile(tmplPath)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}

	// 如果外部模板文件不存在，使用embed嵌入的模板
	return kylinOSBondNetworkManagerTemplate, nil
}



// ReloadIfy 重新加载Bond配置
func (nm *KylinOSBondNetworkManager) ReloadIfy(ctx context.Context) error {
	// KylinOS特定的Bond NetworkManager重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 1. 加载bonding内核模块
	cmd := exec.CommandContext(ctx, "modprobe", "bonding")
	if output, err := cmd.CombinedOutput(); err != nil {
		utils.Warnf("bond", "Failed to load bonding module: %v, output: %s", err, string(output))
	}

	// 2. 重新加载NetworkManager配置
	cmd = exec.CommandContext(ctx, "nmcli", "connection", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload KylinOS Bond NetworkManager config: %v, output: %s", err, string(output))
	}
	utils.Info("bond", "KylinOS Bond NetworkManager connections reloaded successfully")

	// 3. 获取所有nix-operator管理的bond连接
	utils.Info("bond", "Listing KylinOS Bond NetworkManager connections")
	cmd = exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list connections: %v", err)
	}

	// 4. 激活所有nix-operator bond连接
	connections := strings.Split(strings.TrimSpace(string(output)), "\n")
	bondConnections := 0
	var activationErrors []string
	
	for _, conn := range connections {
		if strings.HasPrefix(conn, "nix-operator-bond-") {
			bondConnections++
			utils.Infof("bond", "Activating KylinOS Bond NetworkManager connection: %s", conn)
			ctx2, cancel2 := context.WithTimeout(ctx, 15*time.Second)
			cmd = exec.CommandContext(ctx2, "nmcli", "connection", "up", conn)
			if output, err := cmd.CombinedOutput(); err != nil {
				errorMsg := fmt.Sprintf("Failed to activate bond connection %s: %v, output: %s", conn, err, string(output))
				utils.Errorf("bond", errorMsg)
				activationErrors = append(activationErrors, errorMsg)
			} else {
				utils.Infof("bond", "Bond connection %s activated successfully", conn)
			}
			cancel2()
		}
	}
	
	// 如果有激活失败的连接，返回错误
	if len(activationErrors) > 0 {
		return fmt.Errorf("failed to activate %d bond connections: %s", len(activationErrors), strings.Join(activationErrors, "; "))
	}

	// 5. 等待网络稳定
	time.Sleep(5 * time.Second)

	utils.Infof("bond", "KylinOS Bond NetworkManager reload completed: %d bond connections processed", bondConnections)
	return nil
}



// reloadNetworkManager 重新加载NetworkManager
func (nm *KylinOSBondNetworkManager) reloadNetworkManager(ctx context.Context) error {
	// 重新加载NetworkManager配置
	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload NetworkManager connections: %v", err)
	}

	return nil
}

// isTruthy 辅助函数，判断值是否为真
func (nm *KylinOSBondNetworkManager) isTruthy(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case int, int8, int16, int32, int64:
		return v != 0
	case uint, uint8, uint16, uint32, uint64:
		return v != 0
	case float32:
		return v != 0.0
	case float64:
		return v != 0.0
	case []string:
		return len(v) > 0
	default:
		return value != nil
	}
}

// ensureTemplateDir 确保模板目录存在
func (nm *KylinOSBondNetworkManager) ensureTemplateDir() error {
	templateDir := "/etc/nix-operator/templates"
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return fmt.Errorf("failed to create template directory: %v", err)
	}
	return nil
}

// installTemplate 安装模板文件
func (nm *KylinOSBondNetworkManager) installTemplate() error {
	if err := nm.ensureTemplateDir(); err != nil {
		return err
	}

	tmplPath := "/etc/nix-operator/templates/kylinos_bond_nmconnection.tpl"
	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		// 模板文件不存在，创建它
		tmplContent := kylinOSBondNetworkManagerTemplate
		if err := utils.AtomicWriteFile([]byte(tmplContent), tmplPath, 0644); err != nil {
			return fmt.Errorf("failed to write template file: %v", err)
		}
		utils.Infof("bond", "Installed KylinOS bond NetworkManager template to %s", tmplPath)
	}
	return nil
}

// getCurrentInterfaceStatus 获取当前Bond接口状态
func (nm *KylinOSBondNetworkManager) getCurrentInterfaceStatus(ctx context.Context, bondName string) (types.BondConfig, error) {
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
func (nm *KylinOSBondNetworkManager) compareBondConfig(expected types.BondConfig, current types.BondConfig) bool {
	// 比较IP地址
	if !nm.compareIPAddress(expected.Network.IP, current.Network.IP) {
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
	if !nm.compareStringSlices(expected.Network.DNSServers, current.Network.DNSServers) {
		utils.Debugf("bond", "DNS servers mismatch: expected %v, current %v", expected.Network.DNSServers, current.Network.DNSServers)
		return false
	}

	return true
}

// compareIPAddress 比较IP地址，支持CIDR格式
func (nm *KylinOSBondNetworkManager) compareIPAddress(expected, current string) bool {
	if expected == "" && current == "" {
		return true
	}
	if expected == "" || current == "" {
		return false
	}

	// 标准化IP地址格式
	expectedNorm := nm.normalizeIPAddress(expected)
	currentNorm := nm.normalizeIPAddress(current)

	return expectedNorm == currentNorm
}

// normalizeIPAddress 标准化IP地址格式
func (nm *KylinOSBondNetworkManager) normalizeIPAddress(ipStr string) string {
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
func (nm *KylinOSBondNetworkManager) compareStringSlices(expected, current []string) bool {
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
