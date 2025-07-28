package kylinos

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
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

//go:embed kylinos_nmconnection.tpl
var kylinOSNetworkManagerTemplate embed.FS

// KylinOSNetworkManager KylinOS NetworkManager 网络管理器
type KylinOSNetworkManager struct {
	osInfo *controller.OSInfo
}

// NewKylinOSNetworkManager 创建 KylinOS NetworkManager 实例
func NewKylinOSNetworkManager(osInfo *controller.OSInfo) *KylinOSNetworkManager {
	return &KylinOSNetworkManager{
		osInfo: osInfo,
	}
}

// IsInstall 检查 NetworkManager 是否已安装
func (nm *KylinOSNetworkManager) IsInstall(ctx context.Context) bool {

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
	utils.Infof("network", "Starting KylinOS NetworkManager configuration for interface %s", iface.Name)
	utils.Infof("network", "Interface %s details: IPv4=%s, IPv6=%s, MTU=%d", iface.Name, iface.IPv4Address, iface.IPv6Address, iface.MTU)

	// 首先获取当前接口的实际状态
	currentStatus, err := nm.getCurrentInterfaceStatus(ctx, iface.Name)
	if err != nil {
		utils.Debugf("network", "Failed to get current interface status for %s: %v", iface.Name, err)
	}

	// 如果能获取到当前状态，先比较实际状态
	if currentStatus != nil {
		if nm.compareInterfaceConfig(iface, currentStatus) {
			utils.Debugf("network", "Interface %s actual status matches expected configuration, checking config file", iface.Name)
			// 实际状态匹配，但仍需检查配置文件是否存在和正确
			configPath := filepath.Join("/etc/NetworkManager/system-connections", fmt.Sprintf("%s.nmconnection", iface.Name))
			if _, err := os.Stat(configPath); err == nil {
				// 配置文件存在且实际状态匹配，无需更改
				return false, nil
			}
			// 配置文件不存在，需要创建以确保持久化
			utils.Infof("network", "Interface %s status matches but config file missing, creating config", iface.Name)
		} else {
			utils.Infof("network", "Interface %s actual status differs from expected configuration", iface.Name)
		}
	}

	// 生成配置内容
	utils.Info("network", "Generating KylinOS NetworkManager configuration content")
	configContent, err := nm.generateConfig(iface)
	if err != nil {
		return false, fmt.Errorf("failed to generate KylinOS NetworkManager config: %v", err)
	}
	utils.Info("network", "KylinOS NetworkManager configuration content generated successfully")

	// 确定配置文件路径
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/%s.nmconnection", iface.Name)
	utils.Infof("network", "KylinOS NetworkManager config file path: %s", configPath)

	// 检查配置是否有变更
	newConfigData := []byte(configContent)
	utils.Info("network", "Checking if KylinOS NetworkManager configuration file exists")
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		utils.Info("network", "KylinOS NetworkManager configuration file exists, comparing content")
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Infof("network", "KylinOS NetworkManager config for %s unchanged", iface.Name)
			return false, nil
		}
		utils.Info("network", "KylinOS NetworkManager configuration content has changed")
	} else {
		utils.Info("network", "KylinOS NetworkManager configuration file does not exist, will create new one")
	}

	// 创建配置目录
	utils.Info("network", "Creating KylinOS NetworkManager configuration directory")
	if err := os.MkdirAll("/etc/NetworkManager/system-connections", 0755); err != nil {
		return false, fmt.Errorf("failed to create KylinOS NetworkManager config directory: %v", err)
	}

	// 写入配置文件
	utils.Infof("network", "Writing KylinOS NetworkManager configuration to file: %s", configPath)
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0600); err != nil {
		return false, fmt.Errorf("failed to write KylinOS NetworkManager config: %v", err)
	}

	utils.Infof("network", "KylinOS NetworkManager config for %s updated successfully", iface.Name)
	return true, nil
}

// ReloadIfy 重新加载网络配置
func (nm *KylinOSNetworkManager) ReloadIfy(ctx context.Context) error {
	utils.Info("network", "Reloading KylinOS network configuration with NetworkManager")

	// 重新加载 NetworkManager 配置
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. 重新加载配置
	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload KylinOS NetworkManager config: %v, output: %s", err, string(output))
	}
	utils.Info("network", "KylinOS NetworkManager connections reloaded successfully")

	// 2. 获取所有nix-operator管理的连接
	utils.Info("network", "Listing KylinOS NetworkManager connections")
	cmd = exec.CommandContext(ctx, "nmcli", "-t", "-f", "NAME", "connection", "show")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list connections: %v", err)
	}

	// 3. 激活所有nix-operator连接
	connections := strings.Split(strings.TrimSpace(string(output)), "\n")
	nixOperatorConnections := 0
	var activationErrors []string

	for _, conn := range connections {
		if strings.HasPrefix(conn, "nix-operator-") {
			nixOperatorConnections++
			utils.Infof("network", "Activating KylinOS NetworkManager connection: %s", conn)
			ctx2, cancel2 := context.WithTimeout(ctx, 15*time.Second)
			cmd = exec.CommandContext(ctx2, "nmcli", "connection", "up", conn)
			if output, err := cmd.CombinedOutput(); err != nil {
				errorMsg := fmt.Sprintf("Failed to activate connection %s: %v, output: %s", conn, err, string(output))
				utils.Errorf("network", errorMsg)
				activationErrors = append(activationErrors, errorMsg)
			} else {
				utils.Infof("network", "Connection %s activated successfully", conn)
			}
			cancel2()
		}
	}

	// 如果有激活失败的连接，返回错误
	if len(activationErrors) > 0 {
		return fmt.Errorf("failed to activate %d connections: %s", len(activationErrors), strings.Join(activationErrors, "; "))
	}

	utils.Infof("network", "KylinOS NetworkManager reload completed: %d nix-operator connections processed", nixOperatorConnections)
	return nil
}

// generateConfig 生成 NetworkManager 配置
func (nm *KylinOSNetworkManager) generateConfig(iface types.Interface) (string, error) {
	// 获取模板内容
	utils.Info("network", "Loading KylinOS NetworkManager template")
	tmplContent, err := utils.GetTemplateContent("kylinos_nmconnection.tpl", nm.getEmbeddedTemplate())
	if err != nil {
		return "", fmt.Errorf("failed to get KylinOS NetworkManager template: %v", err)
	}

	// 解析模板
	utils.Info("network", "Parsing KylinOS NetworkManager template")
	tmpl, err := template.New("kylinos_nmconnection").Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse KylinOS NetworkManager template: %v", err)
	}

	// 准备模板数据
	utils.Info("network", "Preparing KylinOS NetworkManager template data")
	templateData := nm.prepareTemplateData(iface)
	utils.Infof("network", "Template data prepared: HasIPv4=%t, HasIPv6=%t, HasBonding=%t", templateData.HasIPv4, templateData.HasIPv6, templateData.HasBonding)

	// 渲染模板
	utils.Info("network", "Rendering KylinOS NetworkManager template")
	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return "", fmt.Errorf("failed to execute KylinOS NetworkManager template: %v", err)
	}

	utils.Info("network", "KylinOS NetworkManager template rendered successfully")
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
func (nm *KylinOSNetworkManager) generateUUID(_ string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		hash := 0
		return fmt.Sprintf("12345678-1234-5678-9abc-%012d", hash%1000000000000)
	}

	// 设置 version(4) 与 variant(10xxxxxx)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant is 10xxxxxx

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4],   // 8  hex
		b[4:6],   // 4  hex
		b[6:8],   // 4  hex
		b[8:10],  // 4  hex
		b[10:16]) // 12 hex
}

// getCurrentInterfaceStatus 获取当前接口状态
func (nm *KylinOSNetworkManager) getCurrentInterfaceStatus(ctx context.Context, interfaceName string) (*types.Interface, error) {

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 使用 nmcli 获取接口详细信息
	cmd := exec.CommandContext(ctx, "nmcli", "device", "show", interfaceName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get interface status: %w", err)
	}

	// 解析 nmcli 输出
	iface := &types.Interface{
		Name: interfaceName,
	}

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
			// 格式: 192.168.1.100/24
			if value != "" && value != "--" {
				iface.IPv4Address = value
			}
		case "IP6.ADDRESS[1]":
			// 格式: 2001:db8::1/64
			if value != "" && value != "--" {
				iface.IPv6Address = value
			}
		case "IP4.GATEWAY":
			if value != "" && value != "--" {
				iface.IPv4Gateway = value
			}
		case "IP4.DNS[1]", "IP4.DNS[2]", "IP4.DNS[3]":
			if value != "" && value != "--" {
				iface.Nameservers = append(iface.Nameservers, value)
			}
		case "GENERAL.MTU":
			if value != "" && value != "--" {
				if mtu, err := strconv.Atoi(value); err == nil {
					iface.MTU = mtu
				}
			}
		}
	}

	return iface, nil
}

// compareInterfaceConfig 比较接口配置
func (nm *KylinOSNetworkManager) compareInterfaceConfig(expected types.Interface, current *types.Interface) bool {
	// 比较 IPv4 地址
	if expected.IPv4Address != "" {
		expectedIP, expectedNet, err := net.ParseCIDR(expected.IPv4Address)
		if err != nil {
			utils.Debugf("network", "Failed to parse expected IPv4 address %s: %v", expected.IPv4Address, err)
			return false
		}

		if current.IPv4Address == "" {
			return false
		}

		currentIP, currentNet, err := net.ParseCIDR(current.IPv4Address)
		if err != nil {
			utils.Debugf("network", "Failed to parse current IPv4 address %s: %v", current.IPv4Address, err)
			return false
		}

		if !expectedIP.Equal(currentIP) || expectedNet.String() != currentNet.String() {
			utils.Debugf("network", "IPv4 address mismatch: expected %s, current %s", expected.IPv4Address, current.IPv4Address)
			return false
		}
	}

	// 比较 IPv6 地址
	if expected.IPv6Address != "" {
		expectedIP, expectedNet, err := net.ParseCIDR(expected.IPv6Address)
		if err != nil {
			utils.Debugf("network", "Failed to parse expected IPv6 address %s: %v", expected.IPv6Address, err)
			return false
		}

		if current.IPv6Address == "" {
			return false
		}

		currentIP, currentNet, err := net.ParseCIDR(current.IPv6Address)
		if err != nil {
			utils.Debugf("network", "Failed to parse current IPv6 address %s: %v", current.IPv6Address, err)
			return false
		}

		if !expectedIP.Equal(currentIP) || expectedNet.String() != currentNet.String() {
			utils.Debugf("network", "IPv6 address mismatch: expected %s, current %s", expected.IPv6Address, current.IPv6Address)
			return false
		}
	}

	// 比较网关
	if expected.IPv4Gateway != "" && expected.IPv4Gateway != current.IPv4Gateway {
		utils.Debugf("network", "Gateway mismatch: expected %s, current %s", expected.IPv4Gateway, current.IPv4Gateway)
		return false
	}

	// 比较 MTU
	if expected.MTU > 0 && expected.MTU != current.MTU {
		utils.Debugf("network", "MTU mismatch: expected %d, current %d", expected.MTU, current.MTU)
		return false
	}

	// 比较 DNS 服务器
	if len(expected.Nameservers) > 0 {
		expectedDNSMap := make(map[string]bool)
		for _, dns := range expected.Nameservers {
			expectedDNSMap[dns] = true
		}

		currentDNSMap := make(map[string]bool)
		for _, dns := range current.Nameservers {
			currentDNSMap[dns] = true
		}

		// 检查期望的 DNS 是否都存在
		for dns := range expectedDNSMap {
			if !currentDNSMap[dns] {
				utils.Debugf("network", "DNS server missing: expected %s", dns)
				return false
			}
		}
	}

	return true
}
