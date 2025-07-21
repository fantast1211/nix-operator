package openeuler

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

// OpenEulerBondIfupdown openEuler系统Bond ifupdown管理器
// 专注于Bond虚拟网卡配置，只负责主接口配置
type OpenEulerBondIfupdown struct {
	osInfo   *controller.OSInfo
	testMode bool // 测试模式标志
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
		osInfo:   osInfo,
		testMode: false,
	}
}

// NewOpenEulerBondIfupdownForTest 创建测试用的openEuler Bond Ifupdown实例
func NewOpenEulerBondIfupdownForTest(osInfo *controller.OSInfo) *OpenEulerBondIfupdown {
	return &OpenEulerBondIfupdown{
		osInfo:   osInfo,
		testMode: true,
	}
}

func (obi *OpenEulerBondIfupdown) IsInstall(ctx context.Context) bool {
	// 测试模式下模拟检测逻辑
	if obi.testMode {
		return obi.mockIsInstall()
	}

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
	// 测试模式下模拟配置逻辑
	if obi.testMode {
		return obi.mockConfigureWithCheck(bondConfig)
	}

	// 验证Bond名称不能为空
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 只配置Bond主接口，从接口配置由network模块处理
	bondChanged, err := obi.configureBondInterface(bondConfig)
	if err != nil {
		return false, fmt.Errorf("failed to configure bond interface: %v", err)
	}

	if bondChanged {
		utils.Infof("bond", "OpenEuler Bond Ifupdown configuration written for bond %s", bondConfig.Name)
	} else {
		utils.Debugf("bond", "OpenEuler Bond Ifupdown config for bond %s unchanged", bondConfig.Name)
	}

	return bondChanged, nil
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
	// 测试模式下模拟重载逻辑
	if obi.testMode {
		return obi.mockReloadIfy()
	}

	// openEuler特定的Bond网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 1. 加载bonding内核模块
	cmd := exec.CommandContext(ctx, "modprobe", "bonding")
	if output, err := cmd.CombinedOutput(); err != nil {
		utils.Warnf("bond", "Failed to load bonding module: %v, output: %s", err, string(output))
	}

	// 2. 重启网络服务
	cmd = exec.CommandContext(ctx, "systemctl", "restart", "network")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 如果network服务不可用，尝试使用NetworkManager
		utils.Warnf("bond", "Failed to restart network service: %v, trying NetworkManager", err)
		cmd = exec.CommandContext(ctx, "systemctl", "restart", "NetworkManager")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to restart network services: %v, output: %s", err, string(output))
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

// 测试模式相关方法
func (obi *OpenEulerBondIfupdown) mockIsInstall() bool {
	// 模拟检测逻辑：假设bond ifupdown总是可用
	utils.Info("bond", "[TEST MODE] openEuler bond ifupdown tools detected")
	return true
}

func (obi *OpenEulerBondIfupdown) mockConfigureWithCheck(bondConfig types.BondConfig) (bool, error) {
	// 验证Bond名称
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 验证IPv4地址格式
	if bondConfig.Network.IP != "" {
		if _, _, err := net.ParseCIDR(bondConfig.Network.IP); err != nil {
			return false, fmt.Errorf("invalid IPv4 CIDR format: %s", bondConfig.Network.IP)
		}
	}

	// 模拟配置逻辑：总是返回配置已变更
	utils.Infof("bond", "[TEST MODE] openEuler bond ifcfg configuration simulated for bond %s", bondConfig.Name)
	return true, nil
}

func (obi *OpenEulerBondIfupdown) mockReloadIfy() error {
	// 模拟重载逻辑：总是成功
	utils.Info("bond", "[TEST MODE] openEuler bond network service restart simulated")
	return nil
}