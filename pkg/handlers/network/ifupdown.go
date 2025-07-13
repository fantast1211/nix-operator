package network

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/utils"
)

type Ifupdown struct{}

//go:embed ifupdown.tpl
var ifupdownTemplate string

func (ifd *Ifupdown) IsInstall(ctx context.Context) bool {
	// 检查ifup和ifdown命令是否存在
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 检查ifup命令
	cmd := exec.CommandContext(ctx, "which", "ifup")
	if cmd.Run() != nil {
		utils.Warn("network", "ifup not found")
		return false
	}

	// 检查ifdown命令
	cmd = exec.CommandContext(ctx, "which", "ifdown")
	if cmd.Run() != nil {
		utils.Warn("network", "ifdown not found")
		return false
	}

	// 注意：不检查/etc/network/interfaces文件是否存在
	// 因为Configure方法会在需要时创建该文件

	// 检查是否有其他网络管理器在运行（如NetworkManager或systemd-networkd）
	// 如果有，则ifupdown可能不是主要的网络管理器
	ctx2, cancel2 := context.WithTimeout(ctx, 3*time.Second)
	defer cancel2()

	// 检查NetworkManager是否在运行
	cmd = exec.CommandContext(ctx2, "systemctl", "is-active", "NetworkManager")
	if cmd.Run() == nil {
		// NetworkManager正在运行，ifupdown可能不是主要管理器
		utils.Warn("network", "The NetworkManager service is running, which implies that the traditional ifupdown utility might not be the primary tool for managing network interfaces.")
		return false
	}

	// 检查systemd-networkd是否在运行
	cmd = exec.CommandContext(ctx2, "systemctl", "is-active", "systemd-networkd")
	if cmd.Run() == nil {
		// systemd-networkd正在运行，ifupdown可能不是主要管理器
		utils.Warn("network", "The systemd-networkd service is running, which implies that the traditional ifupdown utility might not be the primary tool for managing network interfaces.")
		return false
	}

	// 检查networking服务是否存在且启用（Debian/Ubuntu系统）
	cmd = exec.CommandContext(ctx2, "systemctl", "is-enabled", "networking")
	if cmd.Run() == nil {
		return true
	}

	// 如果没有其他网络管理器在运行，且ifupdown工具存在，则认为ifupdown可用
	return true
}

func (ifd *Ifupdown) Configure(ctx context.Context, iface Interface) error {
	// 验证接口名称不能为空
	if iface.Name == "" {
		return fmt.Errorf("interface name cannot be empty")
	}

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

	// 确保目标目录存在
	configPath := "/etc/network/interfaces"
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", filepath.Dir(configPath), err)
	}

	// 写入配置文件
	if err := utils.AtomicWriteFile([]byte(content.String()), configPath, 0644); err != nil {
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
