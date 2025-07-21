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
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Debugf("bond", "CentOS Bond NetworkManager config for bond %s unchanged, skipping write", bondConfig.Name)
			return false, nil // 配置未变更
		}
	}

	// 写入连接配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0600); err != nil {
		return false, fmt.Errorf("failed to write CentOS Bond NetworkManager config: %v", err)
	}

	utils.Infof("bond", "CentOS Bond NetworkManager configuration written for bond %s", bondConfig.Name)
	return true, nil // 配置已变更
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
	for _, conn := range connections {
		if strings.HasPrefix(conn, "nix-operator-bond-") {
			// CentOS 7.x 可能需要更长的等待时间
			ctx2, cancel2 := context.WithTimeout(ctx, 15*time.Second)
			cmd = exec.CommandContext(ctx2, "nmcli", "connection", "up", conn)
			if output, err := cmd.CombinedOutput(); err != nil {
				// 记录警告但继续处理其他连接
				utils.Warnf("bond", "Failed to activate bond connection %s: %v, output: %s", conn, err, string(output))
			} else {
				utils.Infof("bond", "Successfully activated bond connection %s", conn)
			}
			cancel2()
		}
	}

	// 4. CentOS 7.x 特定：等待网络稳定
	time.Sleep(2 * time.Second)

	utils.Infof("bond", "CentOS Bond NetworkManager configuration reloaded successfully")
	return nil
}
