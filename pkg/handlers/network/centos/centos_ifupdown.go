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
	"go.xbrother.com/nix-operator/pkg/handlers/network/common"
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
	// 验证接口名称不能为空
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}

	// 准备模板数据，解析 CIDR 格式的地址
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
		if strings.Contains(iface.IPv4Address, "/") {
			// CIDR 格式：192.168.1.100/24
			ip, ipNet, err := net.ParseCIDR(iface.IPv4Address)
			if err != nil {
				return false, fmt.Errorf("failed to parse IPv4 CIDR %s: %v", iface.IPv4Address, err)
			}
			templateData.IPv4IP = ip.String()
			templateData.IPv4Netmask = cif.cidrToNetmask(ipNet)
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
	templateContent, err := common.GetTemplateContent("centos_ifcfg.tpl", centosIfcfgTemplate)
	if err != nil {
		return false, err
	}

	// 解析模板
	tmpl, err := template.New("centos_ifcfg").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse CentOS ifcfg template: %v", err)
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return false, fmt.Errorf("failed to execute CentOS ifcfg template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/sysconfig/network-scripts/ifcfg-%s", iface.Name)
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Debugf("network", "CentOS ifcfg config for interface %s unchanged, skipping write", iface.Name)
			return false, nil // 配置未变更
		}
	}

	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return false, fmt.Errorf("failed to create directory %s: %v", filepath.Dir(configPath), err)
	}

	// 写入配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write CentOS ifcfg config: %v", err)
	}

	// 如果配置了DNS，更新 /etc/resolv.conf
	if len(iface.Nameservers) > 0 {
		if err := cif.updateResolvConf(iface.Nameservers); err != nil {
			utils.Warnf("network", "Failed to update resolv.conf: %v", err)
		}
	}

	utils.Infof("network", "CentOS ifcfg configuration written for interface %s", iface.Name)
	return true, nil // 配置已变更
}

func (cif *CentOSIfupdown) ReloadIfy(ctx context.Context) error {
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
		utils.Debugf("network", "Trying reload method: %s", method.name)
		cmd := exec.CommandContext(ctx, method.cmd[0], method.cmd[1:]...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			utils.Infof("network", "Successfully reloaded network using: %s", method.name)
			// 等待网络稳定
			time.Sleep(3 * time.Second)
			return nil
		}
		utils.Debugf("network", "Method %s failed: %v, output: %s", method.name, err, string(output))
	}

	return fmt.Errorf("all network reload methods failed")
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
