package network

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/utils"
)

type Ifupdown struct{}

//go:embed ifupdown.tpl
var ifupdownTemplate string

func (ifd *Ifupdown) IsInstall(ctx context.Context) bool {
	_, err := os.Stat("/sbin/ifup")
	return err == nil
}

func (ifd *Ifupdown) findConfig(iface Interface) (string, error) {
	// 检查主配置文件
	mainConfig, err := os.ReadFile("/etc/network/interfaces")
	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(mainConfig)))
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), fmt.Sprintf("iface %s ", iface.Name)) {
				return "/etc/network/interfaces", nil
			}
		}
	}

	// 检查 interfaces.d 目录
	files, err := os.ReadDir("/etc/network/interfaces.d")
	if err != nil {
		return "", fmt.Errorf("failed to read interfaces.d directory: %v", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		path := filepath.Join("/etc/network/interfaces.d", file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		if strings.Contains(string(data), fmt.Sprintf("iface %s ", iface.Name)) {
			return path, nil
		}
	}

	return fmt.Sprintf("/etc/network/interfaces.d/%s", iface.Name), nil
}

func (ifd *Ifupdown) Configure(ctx context.Context, iface Interface) error {
	configPath, err := ifd.findConfig(iface)
	if err != nil {
		return err
	}

	// 创建模板并添加自定义函数
	tmpl := template.New("ifupdown").Funcs(template.FuncMap{
		"join": strings.Join,
	})

	tmpl, err = tmpl.Parse(ifupdownTemplate)
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

	return utils.AtomicWriteFile([]byte(desired), configPath, 0644)
}

func (ifd *Ifupdown) ReloadIfy(ctx context.Context) error {
	if !isServiceActive(ctx, "networking") {
		return nil
	}
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "networking")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restart networking: %v, output: %s", err, output)
	}
	return nil
}
