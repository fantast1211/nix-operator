package network

import (
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"

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

func (nm *NetworkManager) Configure(ctx context.Context, iface Interface) error {
	// 获取模板内容
	templateContent, err := getTemplateContent("nmconnection.tpl", nmConnectionTemplate)
	if err != nil {
		return err
	}

	// 解析模板
	tmpl, err := template.New("nmconnection").Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse NetworkManager template: %v", err)
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, iface); err != nil {
		return fmt.Errorf("failed to execute NetworkManager template: %v", err)
	}

	// 写入连接配置文件
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/nix-operator-%s.nmconnection", iface.Name)
	if err := utils.AtomicWriteFile([]byte(content.String()), configPath, 0600); err != nil {
		return fmt.Errorf("failed to write NetworkManager config: %v", err)
	}

	return nil
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
