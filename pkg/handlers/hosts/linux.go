package hosts

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed hosts.tpl
var hostsTemplate string

// 外部模板目录，用于高优先级覆盖
const externalTemplateDir = "/etc/nix-operator/templates"

type Config struct {
	Hosts []hostEntry `json:"hosts"`
}

type HostEntry struct {
	IP        string   `json:"ip"`
	Hostnames []string `json:"hostnames"`
}

type HostsSpec struct {
	Hosts []HostEntry `json:"hosts"`
}

func init() {
	controller.RegisterHandler("HostsConfiguration", &LinuxHostsHandler{})
}

type LinuxHostsHandler struct{}

type hostEntry struct {
	IP        string
	Hostnames []string
}

func (h *LinuxHostsHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

func (h *LinuxHostsHandler) Reconcile(ctx context.Context, cfg *config.ResourceConfig) (*controller.ReconcileResult, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 读取现有的 hosts 文件
	currentEntries, err := h.getCurrentHosts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read current hosts: %v", err)
	}

	var spec HostsSpec
	if err := json.Unmarshal(cfg.Spec, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hosts spec: %v", err)
	}
	// 转换期望的配置
	desiredEntries := make([]hostEntry, len(spec.Hosts))
	for i, host := range spec.Hosts {
		desiredEntries[i] = hostEntry{
			IP:        host.IP,
			Hostnames: host.Hostnames,
		}
	}

	// 比较现有配置和期望配置
	if h.areHostsEqual(currentEntries, desiredEntries) {
		// 配置一致，无需更新
		return &controller.ReconcileResult{
			Effective: cfg,
			Status: &config.ResourceStatus{
				Phase:             "Ready",
				Reason:            "NoChangeNeeded",
				Message:           "Hosts configuration is already up to date",
				LastReconcileTime: time.Now().Format(time.RFC3339),
			},
		}, nil
	}

	// 准备模板数据
	templateData := struct {
		CommentHeader string
		Hosts         []hostEntry
	}{
		CommentHeader: config.CommentHeader,
		Hosts:         desiredEntries,
	}

	// 渲染模板
	content, err := h.renderHostsTemplate(ctx, templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to render hosts template: %v", err)
	}

	// 原子性写入文件
	if err := utils.AtomicWriteFile([]byte(content), "/etc/hosts", 0644); err != nil {
		return nil, fmt.Errorf("failed to write hosts file: %v", err)
	}

	// 构造成功的ReconcileResult
	result := &controller.ReconcileResult{
		Effective: cfg,
		Status: &config.ResourceStatus{
			Phase:             "Ready",
			Reason:            "HostsUpdated",
			Message:           "Successfully updated /etc/hosts file",
			LastReconcileTime: time.Now().Format(time.RFC3339),
		},
	}

	return result, nil
}

func (h *LinuxHostsHandler) renderHostsTemplate(ctx context.Context, data interface{}) (string, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// 首先检查是否有外部模板
	externalTemplatePath := filepath.Join(externalTemplateDir, "hosts.tpl")
	var tmplContent string

	// 尝试读取外部模板
	if _, err := os.Stat(externalTemplatePath); err == nil {
		externalTemplateBytes, err := os.ReadFile(externalTemplatePath)
		if err == nil {
			tmplContent = string(externalTemplateBytes)
		}
	}

	// 如果没有外部模板，使用嵌入的模板
	if tmplContent == "" {
		tmplContent = hostsTemplate
	}

	// 创建模板并添加自定义函数
	tmpl := template.New("hosts").Funcs(template.FuncMap{
		"join": strings.Join,
	})

	// 解析模板
	tmpl, err := tmpl.Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	// 渲染模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return buf.String(), nil
}

func (h *LinuxHostsHandler) getCurrentHosts(ctx context.Context) ([]hostEntry, error) {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	file, err := os.Open("/etc/hosts")
	if err != nil {
		if os.IsNotExist(err) {
			return []hostEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var entries []hostEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// 检查上下文是否已取消
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 {
			entries = append(entries, hostEntry{
				IP:        fields[0],
				Hostnames: fields[1:],
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (h *LinuxHostsHandler) areHostsEqual(current, desired []hostEntry) bool {
	if len(current) != len(desired) {
		return false
	}

	// 复制切片以避免修改原始数据
	currentCopy := make([]hostEntry, len(current))
	desiredCopy := make([]hostEntry, len(desired))
	copy(currentCopy, current)
	copy(desiredCopy, desired)

	// 对每个条目的主机名进行排序
	for i := range currentCopy {
		slices.Sort(currentCopy[i].Hostnames)
	}
	for i := range desiredCopy {
		slices.Sort(desiredCopy[i].Hostnames)
	}

	// 对条目进行排序（按IP地址）
	slices.SortFunc(currentCopy, func(a, b hostEntry) int {
		return strings.Compare(a.IP, b.IP)
	})
	slices.SortFunc(desiredCopy, func(a, b hostEntry) int {
		return strings.Compare(a.IP, b.IP)
	})

	// 比较每个条目
	for i := range currentCopy {
		if currentCopy[i].IP != desiredCopy[i].IP {
			return false
		}
		if !slices.Equal(currentCopy[i].Hostnames, desiredCopy[i].Hostnames) {
			return false
		}
	}

	return true
}
