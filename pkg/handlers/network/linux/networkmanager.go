package linux

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

	"go.xbrother.com/nix-operator/pkg/handlers/network/common"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

type NetworkManager struct{}

//go:embed nmconnection.tpl
var nmConnectionTemplate string

func (nm *NetworkManager) IsInstall(ctx context.Context) bool {
	// 检查NetworkManager服务是否运行
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "NetworkManager")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == "active"
}

func (nm *NetworkManager) Configure(ctx context.Context, iface types.Interface) error {
	_, err := nm.ConfigureWithCheck(ctx, iface)
	return err
}

func (nm *NetworkManager) ConfigureWithCheck(ctx context.Context, iface types.Interface) (bool, error) {
	// 验证接口名称不能为空
	if iface.Name == "" {
		return false, fmt.Errorf("interface name cannot be empty")
	}

	// 获取模板内容
	templateContent, err := common.GetTemplateContent("nmconnection.tpl", nmConnectionTemplate)
	if err != nil {
		return false, err
	}

	// 解析模板
	tmpl, err := template.New("nmconnection").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse NetworkManager template: %v", err)
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, iface); err != nil {
		return false, fmt.Errorf("failed to execute NetworkManager template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 检查配置文件是否存在以及内容是否相同
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/nix-operator-%s.nmconnection", iface.Name)
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Debugf("network", "NetworkManager config for interface %s unchanged, skipping write", iface.Name)
			return false, nil // 配置未变更
		}
	}

	// 写入连接配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0600); err != nil {
		return false, fmt.Errorf("failed to write NetworkManager config: %v", err)
	}

	utils.Infof("network", "NetworkManager config for interface %s updated", iface.Name)
	return true, nil // 配置已变更
}

func (nm *NetworkManager) ReloadIfy(ctx context.Context) error {
	// 重新加载NetworkManager配置
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. 重新加载配置
	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload NetworkManager config: %v, output: %s", err, string(output))
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
			cmd = exec.CommandContext(ctx, "nmcli", "connection", "up", conn)
			if output, err := cmd.CombinedOutput(); err != nil {
				// 记录警告但继续处理其他连接
				fmt.Printf("Warning: failed to activate connection %s: %v, output: %s\n", conn, err, string(output))
			}
		}
	}

	return nil
}
