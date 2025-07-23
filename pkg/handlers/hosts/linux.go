package hosts

import (
	"context"
	_ "embed"
	"fmt"
	"os"

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


func init() {
	controller.RegisterHandler("HostsConfiguration", &LinuxHostsHandler{})
}

type LinuxHostsHandler struct{}

func (h *LinuxHostsHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}



func (h *LinuxHostsHandler) Reconcile(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
	utils.Infof("hosts", "Starting hosts configuration reconciliation with %d configs", len(configs))
	var results []*domain.ReconcileResult
	var matchedConfig *systemv1.ResourceConfig
	var fallbackConfig *systemv1.ResourceConfig
	configStatusMap := make(map[string]*domain.ReconcileResult)

	// 遍历所有配置，查找匹配的配置和fallback配置
	for _, config := range configs {
		utils.Debugf("hosts", "Processing config: %s", config.Metadata.Name)
		// 解析配置规格
		hostsSpec, err := utils.UnmarshalSpec[*systemv1.HostsConfigurationSpec](config.Spec)
		if err != nil {
			utils.Errorf("hosts", "Failed to unmarshal spec for config %s: %v", config.Metadata.Name, err)
			result, _ := status.ReconcileError(config, status.ReasonSpecError, fmt.Errorf("failed to unmarshal spec: %v", err))
			configStatusMap[config.Metadata.Name] = result
			continue
		}

		// 检查nodeSelector匹配
		if hostsSpec.NodeSelector != nil && h.hasValidNodeSelector(hostsSpec.NodeSelector) {
			utils.Debugf("hosts", "Config %s has valid nodeSelector, checking match", config.Metadata.Name)
			// 有有效的nodeSelector，检查是否匹配
			matched, err := utils.MatchNodeSelector(hostsSpec.NodeSelector)
			if err != nil {
				utils.Errorf("hosts", "Failed to match nodeSelector for config %s: %v", config.Metadata.Name, err)
				result, _ := status.ReconcileError(config, status.ReasonNodeSelectorError, fmt.Errorf("failed to match nodeSelector: %v", err))
				configStatusMap[config.Metadata.Name] = result
				continue
			}
			if matched {
				utils.Infof("hosts", "Config %s matched nodeSelector, will be applied", config.Metadata.Name)
				matchedConfig = config
				// 暂时不设置状态，等待应用后再设置
			} else {
				utils.Debugf("hosts", "Config %s did not match nodeSelector, skipping", config.Metadata.Name)
				result, _ := status.ReconcileSkipped(config, status.ReasonNotMatched, "Configuration did not match nodeSelector")
				configStatusMap[config.Metadata.Name] = result
			}
		} else {
			// 没有有效的nodeSelector，作为fallback配置
			utils.Debugf("hosts", "Config %s has no valid nodeSelector, considering as fallback", config.Metadata.Name)
			if fallbackConfig == nil {
				utils.Debugf("hosts", "Config %s set as fallback configuration", config.Metadata.Name)
				fallbackConfig = config
				// 暂时不设置状态，等待应用后再设置
			} else {
				// 如果已经有fallback配置，则跳过这个
				utils.Debugf("hosts", "Config %s skipped, fallback already exists: %s", config.Metadata.Name, fallbackConfig.Metadata.Name)
				result, _ := status.ReconcileSkipped(config, status.ReasonFallback, "Another fallback configuration already exists")
				configStatusMap[config.Metadata.Name] = result
			}
		}
	}

	// 确定要应用的配置
	var configToApply *systemv1.ResourceConfig
	if matchedConfig != nil {
		utils.Infof("hosts", "Using matched config: %s", matchedConfig.Metadata.Name)
		configToApply = matchedConfig
	} else if fallbackConfig != nil {
		utils.Infof("hosts", "Using fallback config: %s", fallbackConfig.Metadata.Name)
		configToApply = fallbackConfig
	} else {
		utils.Warnf("hosts", "No applicable configuration found")
	}

	// 如果有配置要应用，则应用它
	if configToApply != nil {
		utils.Infof("hosts", "Applying configuration: %s", configToApply.Metadata.Name)
		hostsSpec, err := utils.UnmarshalSpec[*systemv1.HostsConfigurationSpec](configToApply.Spec)
		if err != nil {
			utils.Errorf("hosts", "Failed to unmarshal effective config spec for %s: %v", configToApply.Metadata.Name, err)
			result, _ := status.ReconcileError(configToApply, status.ReasonSpecError, fmt.Errorf("failed to unmarshal effective config spec: %v", err))
			configStatusMap[configToApply.Metadata.Name] = result
		} else {
			// 应用配置
			utils.Debugf("hosts", "Starting to apply hosts configuration for %s", configToApply.Metadata.Name)
			applyResult, err := h.applyConfiguration(ctx, configToApply, hostsSpec)
			if err != nil {
				utils.Errorf("hosts", "Failed to apply configuration %s: %v", configToApply.Metadata.Name, err)
				result, _ := status.ReconcileError(configToApply, status.ReasonReconcileError, fmt.Errorf("failed to apply configuration: %v", err))
				configStatusMap[configToApply.Metadata.Name] = result
			} else {
				// 应用成功
				utils.Infof("hosts", "Successfully applied configuration: %s", configToApply.Metadata.Name)
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

	utils.Infof("hosts", "Hosts configuration reconciliation completed with %d results", len(results))
	return results, nil
}

func (h *LinuxHostsHandler) configureHostname(ctx context.Context, hostname string) error {
	utils.Infof("hosts", "Configuring hostname: %s", hostname)
	// 获取hostname模板
	templateContent, err := utils.GetTemplateContent("hostname.tpl", hostnameTemplate)
	if err != nil {
		utils.Errorf("hosts", "Failed to get hostname template: %v", err)
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
		utils.Debugf("hosts", "Hostname configuration is already up to date: %s", hostname)
		return nil // 配置相同，无需更新
	}
	utils.Debugf("hosts", "Hostname needs to be updated from current state")

	// 使用工具函数原子性写入文件
	utils.Debugf("hosts", "Writing hostname to /etc/hostname")
	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/hostname", 0644); err != nil {
		utils.Errorf("hosts", "Failed to write hostname file: %v", err)
		return fmt.Errorf("failed to write hostname file: %v", err)
	}

	// 使用系统调用设置主机名
	utils.Debugf("hosts", "Setting system hostname via syscall")
	return h.setHostname(ctx, hostname)
}

func (h *LinuxHostsHandler) setHostname(ctx context.Context, hostname string) error {
	// 使用系统调用设置主机名
	utils.Debugf("hosts", "Executing sethostname syscall for: %s", hostname)
	err := syscall.Sethostname([]byte(hostname))
	if err != nil {
		utils.Errorf("hosts", "Failed to set hostname via syscall: %v", err)
		return err
	}
	utils.Infof("hosts", "Successfully set hostname to: %s", hostname)
	return nil
}

func (h *LinuxHostsHandler) configureHosts(ctx context.Context, hosts []*systemv1.HostEntry) error {
	utils.Infof("hosts", "Configuring hosts file with %d entries", len(hosts))
	// 获取hosts模板
	templateContent, err := utils.GetTemplateContent("hosts.tpl", hostsTemplate)
	if err != nil {
		utils.Errorf("hosts", "Failed to get hosts template: %v", err)
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
		utils.Debugf("hosts", "Hosts file configuration is already up to date")
		return nil // 配置相同，无需更新
	}
	utils.Debugf("hosts", "Hosts file needs to be updated")

	// 使用工具函数原子性写入文件
	utils.Debugf("hosts", "Writing hosts configuration to /etc/hosts")
	err = utils.AtomicWriteFile([]byte(desiredContent), "/etc/hosts", 0644)
	if err != nil {
		utils.Errorf("hosts", "Failed to write hosts file: %v", err)
		return err
	}
	utils.Infof("hosts", "Successfully updated hosts file with %d entries", len(hosts))
	return nil
}

// hasValidNodeSelector 检查是否有有效的 nodeSelector
func (h *LinuxHostsHandler) hasValidNodeSelector(selector *systemv1.NodeSelector) bool {
	if selector == nil {
		return false
	}
	return selector.MachineId != ""
}

// applyConfiguration 应用配置项
func (h *LinuxHostsHandler) applyConfiguration(ctx context.Context, configToApply *systemv1.ResourceConfig, hostsSpec *systemv1.HostsConfigurationSpec) (*domain.ReconcileResult, error) {
	utils.Debugf("hosts", "Applying hosts configuration - hostname: %s, hosts entries: %d", hostsSpec.Hostname, len(hostsSpec.Hosts))

	// 处理hostname配置
	if hostsSpec.Hostname != "" {
		utils.Debugf("hosts", "Processing hostname configuration")
		if err := h.configureHostname(ctx, hostsSpec.Hostname); err != nil {
			utils.Errorf("hosts", "Failed to configure hostname: %v", err)
			return nil, fmt.Errorf("failed to configure hostname: %v", err)
		}
		utils.Infof("hosts", "Hostname configuration completed successfully")
	} else {
		utils.Debugf("hosts", "No hostname configuration specified")
	}

	// 处理hosts配置
	if len(hostsSpec.Hosts) > 0 {
		utils.Debugf("hosts", "Processing hosts entries configuration")
		if err := h.configureHosts(ctx, hostsSpec.Hosts); err != nil {
			utils.Errorf("hosts", "Failed to configure hosts: %v", err)
			return nil, fmt.Errorf("failed to configure hosts: %v", err)
		}
		utils.Infof("hosts", "Hosts entries configuration completed successfully")
	} else {
		utils.Debugf("hosts", "No hosts entries specified")
	}

	utils.Infof("hosts", "All hosts configuration applied successfully")
	return status.ReconcileReady(configToApply, "Configured", "Hosts configuration applied successfully")
}
