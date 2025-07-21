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
				utils.Warnf("network", "nmcli command not available")
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

func (onm *OpenEulerNetworkManager) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	// 测试模式下模拟配置逻辑
	if onm.testMode {
		return onm.mockConfigureWithCheck(iface)
	}

	// 验证接口名称不能为空
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}

	// 获取模板内容
	templateContent, err := utils.GetTemplateContent("openeuler_nmconnection.tpl", openeulerNmConnectionTemplate)
	if err != nil {
		return false, err
	}

	// 解析模板
	tmpl, err := template.New("openeuler_nmconnection").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse openEuler NetworkManager template: %v", err)
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
		return false, fmt.Errorf("failed to execute openEuler NetworkManager template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/nix-operator-%s.nmconnection", iface.Name)
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Debugf("network", "openEuler NetworkManager config for interface %s unchanged, skipping write", iface.Name)
			return false, nil // 配置未变更
		}
	}

	// 写入连接配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0600); err != nil {
		return false, fmt.Errorf("failed to write openEuler NetworkManager config: %v", err)
	}

	utils.Infof("network", "openEuler NetworkManager configuration written for interface %s", iface.Name)
	return true, nil // 配置已变更
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
				utils.Warnf("network", "Failed to activate connection %s: %v, output: %s", conn, err, string(output))
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
