package hosts

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed hosts.tpl
var hostsTemplate string

//go:embed hostname.tpl
var hostnameTemplate string

// 外部模板目录，用于高优先级覆盖
const externalTemplateDir = "/etc/nix-operator/templates"

type HostEntry struct {
	IP        string   `json:"ip"`
	Hostnames []string `json:"hostnames"`
}

type HostsSpec struct {
	Configurations []HostsConfigurationItem `json:"configurations"`
}

type HostsConfigurationItem struct {
	NodeSelector utils.NodeSelector `json:"nodeSelector"`
	Hostname     string             `json:"hostname"`
	Hosts        []HostEntry        `json:"hosts"`
}

func init() {
	controller.RegisterHandler("HostsConfiguration", &LinuxHostsHandler{})
}

type LinuxHostsHandler struct{}

func (h *LinuxHostsHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

// 获取模板内容，优先使用外部模板
func getTemplateContent(templateName, defaultContent string) (string, error) {
	externalPath := filepath.Join(externalTemplateDir, templateName)
	if _, err := os.Stat(externalPath); err == nil {
		content, err := os.ReadFile(externalPath)
		if err != nil {
			return "", fmt.Errorf("failed to read external template %s: %v", externalPath, err)
		}
		return string(content), nil
	}
	return defaultContent, nil
}

func (h *LinuxHostsHandler) Reconcile(ctx context.Context, cfg *config.ResourceConfig) (*controller.ReconcileResult, error) {
	// 解析hosts配置
	var hostsSpec struct {
		Configurations []HostsConfigurationItem `json:"configurations"`
	}

	// 将Spec转换为hosts配置
	specBytes, err := json.Marshal(cfg.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %v", err)
	}

	if err := json.Unmarshal(specBytes, &hostsSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hosts spec: %v", err)
	}

	// 第一阶段：优先匹配有 nodeSelector 的配置项

	for _, configItem := range hostsSpec.Configurations {
		// 检查是否有有效的 nodeSelector
		if h.hasValidNodeSelector(configItem.NodeSelector) {
			match, err := utils.MatchNodeSelector(configItem.NodeSelector)
			if err != nil {
				return nil, fmt.Errorf("failed to check node selector: %v", err)
			}
			if match {
				// 找到匹配的特定配置，应用并退出
				return h.applyConfiguration(ctx, configItem)
			}
		}
	}

	// 第二阶段：如果没有匹配的特定配置，使用通用配置
	for _, configItem := range hostsSpec.Configurations {
		if !h.hasValidNodeSelector(configItem.NodeSelector) {
			// 应用通用配置并退出
			return h.applyConfiguration(ctx, configItem)
		}
	}

	// 没有找到任何匹配的配置
	return &controller.ReconcileResult{
		Status: &config.ResourceStatus{
			Phase:   "Skipped",
			Reason:  "NoMatchingConfiguration",
			Message: "No matching configuration found for this host",
		},
	}, nil
}

func (h *LinuxHostsHandler) configureHostname(ctx context.Context, hostname string) error {
	// 获取hostname模板
	templateContent, err := getTemplateContent("hostname.tpl", hostnameTemplate)
	if err != nil {
		return err
	}

	// 解析模板
	tmpl, err := template.New("hostname").Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse hostname template: %v", err)
	}

	// 准备模板数据
	data := struct {
		Hostname string
	}{
		Hostname: hostname,
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, data); err != nil {
		return fmt.Errorf("failed to execute hostname template: %v", err)
	}

	desiredContent := content.String()

	// 读取现有配置
	currentContent, err := os.ReadFile("/etc/hostname")
	if err == nil && string(currentContent) == desiredContent {
		return nil // 配置相同，无需更新
	}

	// 使用工具函数原子性写入文件
	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/hostname", 0644); err != nil {
		return fmt.Errorf("failed to write hostname file: %v", err)
	}

	// 使用系统调用设置主机名
	return h.setHostname(ctx, hostname)
}

func (h *LinuxHostsHandler) setHostname(ctx context.Context, hostname string) error {
	// 使用系统调用设置主机名
	return syscall.Sethostname([]byte(hostname))
}

func (h *LinuxHostsHandler) configureHosts(ctx context.Context, hosts []HostEntry) error {
	// 获取hosts模板
	templateContent, err := getTemplateContent("hosts.tpl", hostsTemplate)
	if err != nil {
		return err
	}

	// 解析模板
	tmpl, err := template.New("hosts").Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse hosts template: %v", err)
	}

	// 准备模板数据
	data := struct {
		Hosts []HostEntry
	}{
		Hosts: hosts,
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, data); err != nil {
		return fmt.Errorf("failed to execute hosts template: %v", err)
	}

	desiredContent := content.String()

	// 读取现有配置
	currentContent, err := os.ReadFile("/etc/hosts")
	if err == nil && string(currentContent) == desiredContent {
		return nil // 配置相同，无需更新
	}

	// 使用工具函数原子性写入文件
	return utils.AtomicWriteFile([]byte(desiredContent), "/etc/hosts", 0644)
}

// hasValidNodeSelector 检查是否有有效的 nodeSelector
func (h *LinuxHostsHandler) hasValidNodeSelector(selector utils.NodeSelector) bool {
	return selector.MachineID != "" || selector.IP != ""
}

// applyConfiguration 应用配置项
func (h *LinuxHostsHandler) applyConfiguration(ctx context.Context, configItem struct {
	NodeSelector utils.NodeSelector `json:"nodeSelector"`
	Hostname     string             `json:"hostname"`
	Hosts        []HostEntry        `json:"hosts"`
}) (*controller.ReconcileResult, error) {
	// 这里传参没有换成HostsConfigurationItem，考虑语义上有歧义，也怕耦合，未来变更影响到这个方法
	// 处理hostname配置
	if configItem.Hostname != "" {
		if err := h.configureHostname(ctx, configItem.Hostname); err != nil {
			return nil, fmt.Errorf("failed to configure hostname: %v", err)
		}
	}

	// 处理hosts配置
	if len(configItem.Hosts) > 0 {
		if err := h.configureHosts(ctx, configItem.Hosts); err != nil {
			return nil, fmt.Errorf("failed to configure hosts: %v", err)
		}
	}

	return &controller.ReconcileResult{
		Status: &config.ResourceStatus{
			Phase:   "Ready",
			Reason:  "Configured",
			Message: "Hosts configuration applied successfully",
		},
	}, nil
}
