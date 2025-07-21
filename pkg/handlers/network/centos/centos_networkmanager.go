package centos

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

// CentOSNetworkManager CentOS 7.2-7.9 专用的 NetworkManager 实现
type CentOSNetworkManager struct {
	osInfo *controller.OSInfo
}

//go:embed centos_nmconnection.tpl
var centosNmConnectionTemplate string

// NewCentOSNetworkManager 创建 CentOS NetworkManager 实例
func NewCentOSNetworkManager(osInfo *controller.OSInfo) *CentOSNetworkManager {
	return &CentOSNetworkManager{
		osInfo: osInfo,
	}
}

func (cnm *CentOSNetworkManager) IsInstall(ctx context.Context) bool {
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

	// CentOS 7.2-7.9 特定检查：验证 NetworkManager 版本兼容性
	if cnm.osInfo != nil {
		if cnm.osInfo.ID == "centos" && cnm.osInfo.VersionID != "" {
			// 检查 nmcli 版本，确保支持所需功能
			cmd = exec.CommandContext(ctx, "nmcli", "--version")
			if cmd.Run() != nil {
				utils.Warnf("network", "nmcli command not available")
				return false
			}
			utils.Infof("network", "CentOS NetworkManager detected and verified")
		}
	}

	return true
}

func (cnm *CentOSNetworkManager) Configure(ctx context.Context, iface types.Interface) error {
	_, err := cnm.ConfigureWithCheck(ctx, iface)
	return err
}

func (cnm *CentOSNetworkManager) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	// 验证接口名称不能为空
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}

	// 获取模板内容
	templateContent, err := utils.GetTemplateContent("centos_nmconnection.tpl", centosNmConnectionTemplate)
	if err != nil {
		return false, err
	}

	// 解析模板
	tmpl, err := template.New("centos_nmconnection").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse CentOS NetworkManager template: %v", err)
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, iface); err != nil {
		return false, fmt.Errorf("failed to execute CentOS NetworkManager template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/nix-operator-%s.nmconnection", iface.Name)
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Debugf("network", "CentOS NetworkManager config for interface %s unchanged, skipping write", iface.Name)
			return false, nil // 配置未变更
		}
	}

	// 写入连接配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0600); err != nil {
		return false, fmt.Errorf("failed to write CentOS NetworkManager config: %v", err)
	}

	utils.Infof("network", "CentOS NetworkManager configuration written for interface %s", iface.Name)
	return true, nil // 配置已变更
}

func (cnm *CentOSNetworkManager) ReloadIfy(ctx context.Context) error {
	// CentOS 7.2-7.9 特定的网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. 重新加载配置
	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload CentOS NetworkManager config: %v, output: %s", err, string(output))
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
			// CentOS 7.x 可能需要更长的等待时间
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

	// 4. CentOS 7.x 特定：等待网络稳定
	time.Sleep(2 * time.Second)

	utils.Infof("network", "CentOS NetworkManager configuration reloaded successfully")
	return nil
}
