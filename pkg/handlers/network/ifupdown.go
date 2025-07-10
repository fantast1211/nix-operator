package network

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/utils"
)

type Ifupdown struct{}

//go:embed ifupdown.tpl
var ifupdownTemplate string

func (ifd *Ifupdown) IsInstall(ctx context.Context) bool {
	// 检查/etc/network/interfaces文件是否存在
	if _, err := os.Stat("/etc/network/interfaces"); err != nil {
		return false
	}
	
	// 检查ifup命令是否存在
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "which", "ifup")
	return cmd.Run() == nil
}

func (ifd *Ifupdown) Configure(ctx context.Context, iface Interface) error {
	// 获取模板内容
	templateContent, err := getTemplateContent("ifupdown.tpl", ifupdownTemplate)
	if err != nil {
		return err
	}

	// 解析模板
	tmpl, err := template.New("ifupdown").Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse ifupdown template: %v", err)
	}

	// 准备模板数据
	data := struct {
		Interfaces map[string]Interface
	}{
		Interfaces: map[string]Interface{
			iface.Name: iface,
		},
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, data); err != nil {
		return fmt.Errorf("failed to execute ifupdown template: %v", err)
	}

	// 写入配置文件
	if err := utils.AtomicWriteFile([]byte(content.String()), "/etc/network/interfaces", 0644); err != nil {
		return fmt.Errorf("failed to write ifupdown config: %v", err)
	}

	return nil
}

func (ifd *Ifupdown) ReloadIfy(ctx context.Context) error {
	// 重启网络服务
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 先尝试使用systemctl重启networking服务
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "networking")
	if err := cmd.Run(); err == nil {
		return nil
	}

	// 如果systemctl失败，尝试使用service命令
	cmd = exec.CommandContext(ctx, "service", "networking", "restart")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart networking service: %v, output: %s", err, string(output))
	}

	return nil
}
