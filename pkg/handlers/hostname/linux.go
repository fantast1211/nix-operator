package hostname

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed hostname.tpl
var hostnameTemplate string

// 外部模板目录，用于高优先级覆盖
const externalTemplateDir = "/etc/nix-operator/templates"

// HostnameSpec 定义了主机名配置的结构
type HostnameSpec struct {
	Hostname string `json:"hostname"`
}

func init() {
	controller.RegisterHandler("HostnameConfiguration", &LinuxHostnameHandler{})
}

// LinuxHostnameHandler 是Linux系统的主机名处理器
type LinuxHostnameHandler struct{}

// Match 检查是否支持该操作系统
func (h *LinuxHostnameHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

// Reconcile 处理主机名配置
func (h *LinuxHostnameHandler) Reconcile(ctx context.Context, cfg *config.ResourceConfig) (*controller.ReconcileResult, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 解析配置
	var spec HostnameSpec
	if err := json.Unmarshal(cfg.Spec, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hostname spec: %v", err)
	}

	// 验证主机名
	if spec.Hostname == "" {
		return nil, fmt.Errorf("hostname cannot be empty")
	}

	// 获取当前主机名
	currentHostname, err := h.getCurrentHostname(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current hostname: %v", err)
	}

	// 如果主机名没有变化，则不需要更新
	if currentHostname == spec.Hostname {
		fmt.Print("111111")
		return &controller.ReconcileResult{
			Effective: cfg,
			Status: &config.ResourceStatus{
				Phase:             "Ready",
				Reason:            "NoChangeNeeded",
				Message:           "Hostname is already up to date",
				LastReconcileTime: time.Now().Format(time.RFC3339),
			},
		}, nil
	}

	// 更新主机名
	if err := h.updateHostname(ctx, spec.Hostname); err != nil {
		return nil, fmt.Errorf("failed to update hostname: %v", err)
	}

	// 准备模板数据
	templateData := struct {
		CommentHeader string
		Hostname      string
	}{
		CommentHeader: config.CommentHeader,
		Hostname:      spec.Hostname,
	}

	// 渲染模板
	content, err := h.renderHostnameTemplate(ctx, templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to render hostname template: %v", err)
	}

	// 打印调试信息
	fmt.Printf("Rendered hostname template: %q\n", content)

	// 更新 /etc/hostname 文件
	if err := utils.AtomicWriteFile([]byte(content), "/etc/hostname", 0644); err != nil {
		return nil, fmt.Errorf("failed to write hostname file: %v", err)
	}

	// 更新 /etc/hosts 文件中的本地主机名条目
	if err := h.updateHostsFile(ctx, spec.Hostname); err != nil {
		return nil, fmt.Errorf("failed to update hosts file: %v", err)
	}

	// 构造成功的ReconcileResult
	result := &controller.ReconcileResult{
		Effective: cfg,
		Status: &config.ResourceStatus{
			Phase:             "Ready",
			Reason:            "HostnameUpdated",
			Message:           fmt.Sprintf("Successfully updated hostname to %s", spec.Hostname),
			LastReconcileTime: time.Now().Format(time.RFC3339),
		},
	}

	return result, nil
}

// renderHostnameTemplate 渲染主机名模板
func (h *LinuxHostnameHandler) renderHostnameTemplate(ctx context.Context, data interface{}) (string, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// 首先检查是否有外部模板
	externalTemplatePath := filepath.Join(externalTemplateDir, "hostname.tpl")
	var tmplContent string

	// 尝试读取外部模板
	if _, err := os.Stat(externalTemplatePath); err == nil {
		externalTemplateBytes, err := os.ReadFile(externalTemplatePath)
		if err == nil {
			tmplContent = string(externalTemplateBytes)
			fmt.Printf("Using external template: %q\n", tmplContent)
		}
	}

	// 如果没有外部模板，使用嵌入的模板
	if tmplContent == "" {
		tmplContent = hostnameTemplate
		fmt.Printf("Using embedded template: %q\n", tmplContent)
	}

	// 创建模板
	tmpl, err := template.New("hostname").Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	// 渲染模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	result := buf.String()
	fmt.Printf("Rendered hostname template (hex): %x\n", result)
	return result, nil
}

// getCurrentHostname 获取当前系统主机名
func (h *LinuxHostnameHandler) getCurrentHostname(ctx context.Context) (string, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// 使用标准库获取主机名
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %v", err)
	}

	return hostname, nil
}

// updateHostname 使用系统调用更新主机名
func (h *LinuxHostnameHandler) updateHostname(ctx context.Context, hostname string) error {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return err
	}

	// 使用syscall设置主机名
	if err := syscall.Sethostname([]byte(hostname)); err != nil {
		return fmt.Errorf("failed to set hostname: %v", err)
	}

	return nil
}

// updateHostsFile 更新 /etc/hosts 文件中的本地主机名条目
func (h *LinuxHostnameHandler) updateHostsFile(ctx context.Context, hostname string) error {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return err
	}

	// 读取当前的 hosts 文件
	hostsContent, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return fmt.Errorf("failed to read hosts file: %v", err)
	}

	// 处理 hosts 文件内容
	var newLines []string
	lines := strings.Split(string(hostsContent), "\n")
	localHostUpdated := false

	for _, line := range lines {
		// 检查上下文是否已取消
		if err := ctx.Err(); err != nil {
			return err
		}

		// 跳过空行
		if strings.TrimSpace(line) == "" {
			newLines = append(newLines, line)
			continue
		}

		// 跳过注释行
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			newLines = append(newLines, line)
			continue
		}

		// 解析行
		fields := strings.Fields(line)
		if len(fields) < 2 {
			newLines = append(newLines, line)
			continue
		}

		// 检查是否是本地主机条目 (127.0.0.1 或 ::1)
		if fields[0] == "127.0.0.1" || fields[0] == "::1" {
			// 保留原始的主机名列表，但替换旧的主机名
			hostnames := []string{"localhost"}
			for _, h := range fields[1:] {
				// 跳过旧的主机名和localhost
				if h != hostname && h != "localhost" {
					hostnames = append(hostnames, h)
				}
			}

			// 添加新的主机名（如果是127.0.0.1条目）
			if fields[0] == "127.0.0.1" {
				hostnames = append(hostnames, hostname)
				localHostUpdated = true
			}

			// 构建新行
			newLine := fields[0] + "\t" + strings.Join(hostnames, " ")
			newLines = append(newLines, newLine)
		} else {
			newLines = append(newLines, line)
		}
	}

	// 如果没有找到127.0.0.1条目，添加一个
	if !localHostUpdated {
		newLines = append(newLines, fmt.Sprintf("127.0.0.1\tlocalhost %s", hostname))
	}

	// 准备新的文件内容
	newContent := []byte(strings.Join(newLines, "\n"))
	// 确保文件以换行符结尾
	if !bytes.HasSuffix(newContent, []byte{"\n"[0]}) {
		newContent = append(newContent, "\n"[0])
	}

	// 原子性写入文件
	if err := utils.AtomicWriteFile(newContent, "/etc/hosts", 0644); err != nil {
		return fmt.Errorf("failed to write hosts file: %v", err)
	}

	return nil
}
