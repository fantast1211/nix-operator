package network

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/utils"
)

type NetworkManager struct{}

//go:embed nmconnection.tpl
var nmConnectionTemplate string

func (nm *NetworkManager) IsInstall(ctx context.Context) bool {
	_, err := os.Stat("/usr/sbin/NetworkManager")
	return err == nil
}

func (nm *NetworkManager) Configure(ctx context.Context, iface Interface) error {
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/%s.nmconnection", iface.Name)

	// 创建模板并添加自定义函数
	tmpl := template.New("nmconnection").Funcs(template.FuncMap{
		"join": strings.Join,
	})

	tmpl, err := tmpl.Parse(nmConnectionTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}

	// 准备模板数据
	data := struct {
		CommentHeader string
		Interface     Interface
	}{
		CommentHeader: config.CommentHeader,
		Interface:     iface,
	}

	// 渲染模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	desired := buf.String()

	// 读取现有配置
	current, err := os.ReadFile(configPath)
	if err == nil && string(current) == desired {
		return nil // 配置相同，无需更新
	}

	// 写入新配置
	return utils.AtomicWriteFile([]byte(desired), configPath, 0600)
}

func (nm *NetworkManager) ReloadIfy(ctx context.Context) error {
	if !isServiceActive(ctx, "NetworkManager") {
		return nil
	}
	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to reload NetworkManager: %v, output: %s", err, output)
	}
	return nil
}

// func (nm *NetworkManager) ReloadIfy(ctx context.Context) error {
// 	if !isServiceActive(ctx, "NetworkManager") {
// 		return nil
// 	}

// 	// 使用 D-Bus API 替代命令行调用
// 	// 获取Settings对象而不是NetworkManager对象
// 	settings, err := gonetworkmanager.NewSettings()
// 	if err != nil {
// 		return fmt.Errorf("failed to connect to NetworkManager Settings: %v", err)
// 	}

// 	// 调用Settings对象的ReloadConnections方法
// 	err = settings.ReloadConnections()
// 	if err != nil {
// 		return fmt.Errorf("failed to reload NetworkManager connections: %v", err)
// 	}

// 	return nil
// }
