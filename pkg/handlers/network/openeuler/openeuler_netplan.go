package openeuler

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// OpenEulerNetplan openEuler 专用的 Netplan 实现
type OpenEulerNetplan struct {
	osInfo   *controller.OSInfo
	testMode bool // 测试模式标志
}

//go:embed openeuler_netplan.tpl
var openeulerNetplanTemplate string

// NewOpenEulerNetplan 创建 openEuler Netplan 实例
func NewOpenEulerNetplan(osInfo *controller.OSInfo) *OpenEulerNetplan {
	return &OpenEulerNetplan{
		osInfo:   osInfo,
		testMode: false,
	}
}

// NewOpenEulerNetplanForTest 创建测试用的 openEuler Netplan 实例
func NewOpenEulerNetplanForTest(osInfo *controller.OSInfo) *OpenEulerNetplan {
	return &OpenEulerNetplan{
		osInfo:   osInfo,
		testMode: true,
	}
}

func (onp *OpenEulerNetplan) IsInstall(ctx context.Context) bool {
	// 测试模式下模拟检测逻辑
	if onp.testMode {
		return onp.mockIsInstall()
	}

	// 检查netplan是否安装
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "which", "netplan")
	if err := cmd.Run(); err != nil {
		utils.Debug("network", "netplan command not found")
		return false
	}

	// openEuler 特定检查：验证 netplan 版本兼容性
	if onp.osInfo != nil {
		if onp.osInfo.ID == "openeuler" && onp.osInfo.VersionID != "" {
			// 检查 netplan 版本
			cmd = exec.CommandContext(ctx, "netplan", "--version")
			output, err := cmd.Output()
			if err != nil {
				utils.Warnf("network", "Failed to get netplan version: %v", err)
				return false
			}
			version := strings.TrimSpace(string(output))
			utils.Infof("network", "openEuler netplan detected, version: %s", version)
		}
	}

	// 检查netplan配置目录是否存在
	if _, err := os.Stat("/etc/netplan"); os.IsNotExist(err) {
		utils.Debug("network", "/etc/netplan directory does not exist")
		return false
	}

	return true
}

func (onp *OpenEulerNetplan) Configure(ctx context.Context, iface types.Interface) error {
	_, err := onp.ConfigureWithCheck(ctx, iface)
	return err
}

func (onp *OpenEulerNetplan) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	// 测试模式下模拟配置逻辑
	if onp.testMode {
		return onp.mockConfigureWithCheck(iface)
	}

	// 验证接口名称不能为空
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}

	// 获取模板内容
	templateContent, err := utils.GetTemplateContent("openeuler_netplan.tpl", openeulerNetplanTemplate)
	if err != nil {
		return false, err
	}

	// 解析模板
	tmpl, err := template.New("openeuler_netplan").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse openEuler netplan template: %v", err)
	}

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

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return false, fmt.Errorf("failed to execute openEuler netplan template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/netplan/50-nix-operator-%s.yaml", iface.Name)
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Debugf("network", "openEuler netplan config for interface %s unchanged, skipping write", iface.Name)
			return false, nil // 配置未变更
		}
	}

	// 确保netplan目录存在
	if err := os.MkdirAll("/etc/netplan", 0755); err != nil {
		return false, fmt.Errorf("failed to create netplan directory: %v", err)
	}

	// 写入配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write openEuler netplan config: %v", err)
	}

	utils.Infof("network", "openEuler netplan configuration written for interface %s", iface.Name)
	return true, nil // 配置已变更
}

func (onp *OpenEulerNetplan) ReloadIfy(ctx context.Context) error {
	// 测试模式下模拟重载逻辑
	if onp.testMode {
		return onp.mockReloadIfy()
	}

	// openEuler 特定的 netplan 重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 1. 验证配置语法
	cmd := exec.CommandContext(ctx, "netplan", "try", "--timeout=30")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("openEuler netplan configuration validation failed: %v, output: %s", err, string(output))
	}

	// 2. 应用配置
	cmd = exec.CommandContext(ctx, "netplan", "apply")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply openEuler netplan config: %v, output: %s", err, string(output))
	}

	// 3. openEuler 特定：等待网络稳定
	time.Sleep(3 * time.Second)

	utils.Infof("network", "openEuler netplan configuration applied successfully")
	return nil
}

// 测试模式相关方法
func (onp *OpenEulerNetplan) mockIsInstall() bool {
	// 模拟检测逻辑：假设Netplan不可用（优先级最低）
	utils.Info("network", "[TEST MODE] openEuler Netplan not detected (simulated)")
	return false
}

func (onp *OpenEulerNetplan) mockConfigureWithCheck(iface types.Interface) (bool, error) {
	// 模拟配置逻辑：总是返回配置已变更
	utils.Infof("network", "[TEST MODE] openEuler Netplan configuration simulated for interface %s", iface.Name)
	return true, nil
}

func (onp *OpenEulerNetplan) mockReloadIfy() error {
	// 模拟重载逻辑：总是成功
	utils.Info("network", "[TEST MODE] openEuler Netplan reload simulated")
	return nil
}
