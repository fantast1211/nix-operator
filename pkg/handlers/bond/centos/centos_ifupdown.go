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
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// CentOSBondIfupdown CentOS 7.2-7.9 专用的传统Bond网络脚本实现
type CentOSBondIfupdown struct {
	osInfo *controller.OSInfo
}

//go:embed centos_bond_ifcfg.tpl
var centosBondIfcfgTemplate string

// CentOSBondIfcfgData 用于模板渲染的数据结构
type CentOSBondIfcfgData struct {
	types.BondConfig
	IPv4IP      string // 分离出的 IPv4 地址
	IPv4Netmask string // 分离出的子网掩码
}

// NewCentOSBondIfupdown 创建 CentOS Bond Ifupdown 实例
func NewCentOSBondIfupdown(osInfo *controller.OSInfo) *CentOSBondIfupdown {
	return &CentOSBondIfupdown{
		osInfo: osInfo,
	}
}

func (cbif *CentOSBondIfupdown) IsInstall(ctx context.Context) bool {
	// 检查传统网络脚本工具是否存在
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 检查ifup和ifdown命令
	cmd := exec.CommandContext(ctx, "which", "ifup")
	if cmd.Run() != nil {
		utils.Debug("bond", "ifup command not found")
		return false
	}

	cmd = exec.CommandContext(ctx, "which", "ifdown")
	if cmd.Run() != nil {
		utils.Debug("bond", "ifdown command not found")
		return false
	}

	// 检查 /etc/sysconfig/network-scripts 目录是否存在
	if _, err := os.Stat("/etc/sysconfig/network-scripts"); os.IsNotExist(err) {
		utils.Debug("bond", "/etc/sysconfig/network-scripts directory not found")
		return false
	}

	// 检查是否有其他网络管理器在运行
	ctx2, cancel2 := context.WithTimeout(ctx, 3*time.Second)
	defer cancel2()

	// 检查NetworkManager是否在运行
	cmd = exec.CommandContext(ctx2, "systemctl", "is-active", "NetworkManager")
	if cmd.Run() == nil {
		// NetworkManager正在运行，传统网络脚本可能不是主要管理器
		utils.Debug("bond", "NetworkManager is running, traditional network scripts might not be primary")
		return false
	}

	// 检查systemd-networkd是否在运行
	cmd = exec.CommandContext(ctx2, "systemctl", "is-active", "systemd-networkd")
	if cmd.Run() == nil {
		utils.Debug("bond", "systemd-networkd is running, traditional network scripts might not be primary")
		return false
	}

	// 检查network服务是否存在且启用（CentOS 7.x系统）
	cmd = exec.CommandContext(ctx2, "systemctl", "is-enabled", "network")
	if cmd.Run() == nil {
		utils.Info("bond", "CentOS traditional bond network scripts detected and verified")
		return true
	}

	// 如果没有其他网络管理器在运行，且传统工具存在，则认为可用
	utils.Info("bond", "CentOS bond ifupdown tools available as fallback")
	return true
}

func (cbif *CentOSBondIfupdown) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := cbif.ConfigureWithCheck(ctx, bondConfig)
	return err
}

func (cbif *CentOSBondIfupdown) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	// 验证Bond名称不能为空
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 准备模板数据，解析 CIDR 格式的地址
	templateData := CentOSBondIfcfgData{
		BondConfig: bondConfig,
	}

	// 解析 IPv4 地址和子网掩码
	if bondConfig.Network.IP != "" {
		if strings.Contains(bondConfig.Network.IP, "/") {
			// CIDR 格式：192.168.1.100/24
			ip, ipNet, err := net.ParseCIDR(bondConfig.Network.IP)
			if err != nil {
				return false, fmt.Errorf("failed to parse IPv4 CIDR %s: %v", bondConfig.Network.IP, err)
			}
			templateData.IPv4IP = ip.String()
			templateData.IPv4Netmask = cbif.cidrToNetmask(ipNet)
		} else {
			// 纯 IP 地址格式
			templateData.IPv4IP = bondConfig.Network.IP
		}
	}

	// 获取模板内容，优先使用外部模板
	templateContent, err := utils.GetTemplateContent("centos_bond_ifcfg.tpl", centosBondIfcfgTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to get CentOS bond ifcfg template: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("centos_bond_ifcfg").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse CentOS bond ifcfg template: %v", err)
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return false, fmt.Errorf("failed to execute CentOS bond ifcfg template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/sysconfig/network-scripts/ifcfg-%s", bondConfig.Name)
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Debugf("bond", "CentOS bond ifcfg config for bond %s unchanged, skipping write", bondConfig.Name)
			return false, nil // 配置未变更
		}
	}

	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return false, fmt.Errorf("failed to create directory %s: %v", filepath.Dir(configPath), err)
	}

	// 写入配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write CentOS bond ifcfg config: %v", err)
	}

	// 如果配置了DNS，更新 /etc/resolv.conf
	if len(bondConfig.Network.DNSServers) > 0 {
		if err := cbif.updateResolvConf(bondConfig.Network.DNSServers); err != nil {
			utils.Warnf("bond", "Failed to update resolv.conf: %v", err)
		}
	}

	utils.Infof("bond", "CentOS bond ifcfg configuration written for bond %s", bondConfig.Name)
	return true, nil // 配置已变更
}

func (cbif *CentOSBondIfupdown) ReloadIfy(ctx context.Context) error {
	// CentOS 7.2-7.9 特定的Bond网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 尝试多种重启网络的方法，按优先级排序
	reloadMethods := []struct {
		name string
		cmd  []string
	}{
		{"systemctl restart network", []string{"systemctl", "restart", "network"}},
		{"service network restart", []string{"service", "network", "restart"}},
	}

	for _, method := range reloadMethods {
		utils.Infof("bond", "Trying to reload CentOS bond network using: %s", method.name)
		cmd := exec.CommandContext(ctx, method.cmd[0], method.cmd[1:]...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			utils.Infof("bond", "CentOS bond network reloaded successfully using: %s", method.name)
			return nil
		}
		utils.Warnf("bond", "Failed to reload bond network using %s: %v, output: %s", method.name, err, string(output))
	}

	return fmt.Errorf("all CentOS bond network reload methods failed")
}

// cidrToNetmask 将CIDR转换为子网掩码
func (cbif *CentOSBondIfupdown) cidrToNetmask(ipNet *net.IPNet) string {
	mask := ipNet.Mask
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}

// updateResolvConf 更新 /etc/resolv.conf 文件
func (cbif *CentOSBondIfupdown) updateResolvConf(nameservers []string) error {
	if len(nameservers) == 0 {
		return nil
	}

	// 构建 resolv.conf 内容
	var content strings.Builder
	content.WriteString("# Generated by nix-operator bond configuration\n")
	for _, ns := range nameservers {
		content.WriteString(fmt.Sprintf("nameserver %s\n", ns))
	}

	// 写入 /etc/resolv.conf
	if err := utils.AtomicWriteFile([]byte(content.String()), "/etc/resolv.conf", 0644); err != nil {
		return fmt.Errorf("failed to write /etc/resolv.conf: %v", err)
	}

	return nil
}
