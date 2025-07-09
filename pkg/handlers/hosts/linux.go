package hosts

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed hosts.tpl
var hostsTemplate string

//go:embed hostname.tpl
var hostnameTemplate string

// 外部模板目录，用于高优先级覆盖
const externalTemplateDir = "/etc/nix-operator/templates"

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

func (h *LinuxHostsHandler) Reconcile(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
	var results []*domain.ReconcileResult
	var matchedConfig *systemv1.ResourceConfig
	var fallbackConfig *systemv1.ResourceConfig
	configStatusMap := make(map[string]*domain.ReconcileResult)

	// 遍历所有配置，查找匹配的配置和fallback配置
	for _, config := range configs {
		// 解析配置规格
		hostsSpec, err := utils.UnmarshalSpec[*systemv1.HostsConfigurationSpec](config.Spec)
		if err != nil {
			result, _ := status.ReconcileError(config, status.ReasonSpecError, fmt.Errorf("failed to unmarshal spec: %v", err))
			configStatusMap[config.Metadata.Name] = result
			continue
		}

		// 检查nodeSelector匹配
		if hostsSpec.NodeSelector != nil && h.hasValidNodeSelector(hostsSpec.NodeSelector) {
			// 有有效的nodeSelector，检查是否匹配
			matched, err := utils.MatchNodeSelector(hostsSpec.NodeSelector)
			if err != nil {
				result, _ := status.ReconcileError(config, status.ReasonNodeSelectorError, fmt.Errorf("failed to match nodeSelector: %v", err))
				configStatusMap[config.Metadata.Name] = result
				continue
			}
			if matched {
				matchedConfig = config
				// 暂时不设置状态，等待应用后再设置
			} else {
				result, _ := status.ReconcileSkipped(config, status.ReasonNotMatched, "Configuration did not match nodeSelector")
				configStatusMap[config.Metadata.Name] = result
			}
		} else {
			// 没有有效的nodeSelector，作为fallback配置
			if fallbackConfig == nil {
				fallbackConfig = config
				// 暂时不设置状态，等待应用后再设置
			} else {
				// 如果已经有fallback配置，则跳过这个
				result, _ := status.ReconcileSkipped(config, status.ReasonFallback, "Another fallback configuration already exists")
				configStatusMap[config.Metadata.Name] = result
			}
		}
	}

	// 确定要应用的配置
	var configToApply *systemv1.ResourceConfig
	if matchedConfig != nil {
		configToApply = matchedConfig
	} else if fallbackConfig != nil {
		configToApply = fallbackConfig
	}

	// 如果有配置要应用，则应用它
	if configToApply != nil {
		hostsSpec, err := utils.UnmarshalSpec[*systemv1.HostsConfigurationSpec](configToApply.Spec)
		if err != nil {
			result, _ := status.ReconcileError(configToApply, status.ReasonSpecError, fmt.Errorf("failed to unmarshal effective config spec: %v", err))
			configStatusMap[configToApply.Metadata.Name] = result
		} else {
			// 应用配置
			applyResult, err := h.applyConfiguration(ctx, configToApply, hostsSpec)
			if err != nil {
				result, _ := status.ReconcileError(configToApply, status.ReasonReconcileError, fmt.Errorf("failed to apply configuration: %v", err))
				configStatusMap[configToApply.Metadata.Name] = result
			} else {
				// 应用成功
				configStatusMap[configToApply.Metadata.Name] = applyResult
			}
		}
		
		// 为其他未应用的配置设置跳过状态
		for _, config := range configs {
			if _, exists := configStatusMap[config.Metadata.Name]; !exists {
				if config == configToApply {
					// 这种情况不应该发生，但为了安全起见
					continue
				}
				var reason, message string
				if config == fallbackConfig {
					reason = status.ReasonFallback
					message = "Configuration used as fallback but not applied due to matched configuration"
				} else {
					reason = status.ReasonNotMatched
					message = "Configuration not applied"
				}
				result, _ := status.ReconcileSkipped(config, reason, message)
				configStatusMap[config.Metadata.Name] = result
			}
		}
	} else {
		// 没有配置可以应用，所有配置都标记为跳过
		for _, config := range configs {
			if _, exists := configStatusMap[config.Metadata.Name]; !exists {
				result, _ := status.ReconcileSkipped(config, status.ReasonNotMatched, "No applicable configuration found")
				configStatusMap[config.Metadata.Name] = result
			}
		}
	}

	// 按照原始配置顺序构建结果
	for _, config := range configs {
		if result, exists := configStatusMap[config.Metadata.Name]; exists {
			results = append(results, result)
		}
	}

	return results, nil
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

func (h *LinuxHostsHandler) configureHosts(ctx context.Context, hosts []*systemv1.HostEntry) error {
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
		Hosts []*systemv1.HostEntry
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
func (h *LinuxHostsHandler) hasValidNodeSelector(selector *systemv1.NodeSelector) bool {
	if selector == nil {
		return false
	}
	return selector.MachineId != "" || selector.Ip != ""
}

// applyConfiguration 应用配置项
func (h *LinuxHostsHandler) applyConfiguration(ctx context.Context, configToApply *systemv1.ResourceConfig, hostsSpec *systemv1.HostsConfigurationSpec) (*domain.ReconcileResult, error) {
	// 处理hostname配置
	if hostsSpec.Hostname != "" {
		if err := h.configureHostname(ctx, hostsSpec.Hostname); err != nil {
			return nil, fmt.Errorf("failed to configure hostname: %v", err)
		}
	}

	// 处理hosts配置
	if len(hostsSpec.Hosts) > 0 {
		if err := h.configureHosts(ctx, hostsSpec.Hosts); err != nil {
			return nil, fmt.Errorf("failed to configure hosts: %v", err)
		}
	}

	return status.ReconcileReady(configToApply, "Configured", "Hosts configuration applied successfully")
}
