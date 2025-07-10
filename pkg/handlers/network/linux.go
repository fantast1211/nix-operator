package network

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

type Interface struct {
	Name        string   `json:"name" yaml:"name,omitempty"`
	IPv4Address string   `json:"ipAddress,omitempty" yaml:"addresses,omitempty"`  // IPv4 地址
	IPv6Address string   `json:"ipv6Address,omitempty" yaml:"-"`                  // IPv6 地址
	IPv4Gateway string   `json:"gateway,omitempty" yaml:"gateway4,omitempty"`     // IPv4 网关
	IPv6Gateway string   `json:"ipv6Gateway,omitempty" yaml:"gateway6,omitempty"` // IPv6 网关
	MTU         int      `json:"mtu,omitempty" yaml:"mtu,omitempty"`
	Nameservers []string `json:"nameservers,omitempty" yaml:"-"`
}

// 外部模板目录，用于高优先级覆盖
const externalTemplateDir = "/etc/nix-operator/templates"

func init() {
	controller.RegisterHandler("NetworkConfiguration", &LinuxNetworkHandler{})
}

type LinuxNetworkHandler struct{}

func (h *LinuxNetworkHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

func (h *LinuxNetworkHandler) Reconcile(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
	var results []*domain.ReconcileResult
	var matchedConfig *systemv1.ResourceConfig
	var fallbackConfig *systemv1.ResourceConfig
	configStatusMap := make(map[string]*domain.ReconcileResult)

	// 遍历所有配置，查找匹配的配置和fallback配置
	for _, config := range configs {
		// 解析配置规格
		networkSpec, err := utils.UnmarshalSpec[*systemv1.NetworkConfigurationSpec](config.Spec)
		if err != nil {
			result, _ := status.ReconcileError(config, status.ReasonSpecError, fmt.Errorf("failed to unmarshal spec: %v", err))
			configStatusMap[config.Metadata.Name] = result
			continue
		}

		// 检查nodeSelector匹配
		if networkSpec.NodeSelector != nil && h.hasValidNodeSelector(networkSpec.NodeSelector) {
			// 有有效的nodeSelector，检查是否匹配
			matched, err := utils.MatchNodeSelector(networkSpec.NodeSelector)
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
		networkSpec, err := utils.UnmarshalSpec[*systemv1.NetworkConfigurationSpec](configToApply.Spec)
		if err != nil {
			result, _ := status.ReconcileError(configToApply, status.ReasonSpecError, fmt.Errorf("failed to unmarshal effective config spec: %v", err))
			configStatusMap[configToApply.Metadata.Name] = result
		} else {
			// 应用配置
			applyResult, err := h.applyConfiguration(ctx, configToApply, networkSpec)
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

// hasValidNodeSelector 检查是否有有效的 nodeSelector
func (h *LinuxNetworkHandler) hasValidNodeSelector(selector *systemv1.NodeSelector) bool {
	if selector == nil {
		return false
	}
	return selector.MachineId != "" || selector.Ip != ""
}

// getTemplateContent 获取模板内容，优先使用外部模板
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

// applyConfiguration 应用网络配置
func (h *LinuxNetworkHandler) applyConfiguration(ctx context.Context, configToApply *systemv1.ResourceConfig, networkSpec *systemv1.NetworkConfigurationSpec) (*domain.ReconcileResult, error) {
	// 转换为内部接口结构
	iface := Interface{
		Name:        networkSpec.Name,
		IPv4Address: networkSpec.Ipv4Address,
		IPv6Address: networkSpec.Ipv6Address,
		IPv4Gateway: networkSpec.Ipv4Gateway,
		IPv6Gateway: networkSpec.Ipv6Gateway,
		MTU:         int(networkSpec.Mtu),
		Nameservers: networkSpec.Nameservers,
	}

	// 检测并选择合适的网络管理器
	manager, err := h.detectNetworkManager(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect network manager: %v", err)
	}

	// 配置网络接口
	if err := manager.Configure(ctx, iface); err != nil {
		return nil, fmt.Errorf("failed to configure network interface: %v", err)
	}

	// 重新加载网络配置
	if err := manager.ReloadIfy(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload network configuration: %v", err)
	}

	return status.ReconcileReady(configToApply, "Configured", "Network configuration applied successfully")
}

// detectNetworkManager 检测系统中可用的网络管理器
func (h *LinuxNetworkHandler) detectNetworkManager(ctx context.Context) (INetworkManager, error) {
	// 按优先级检测网络管理器
	managers := []INetworkManager{
		// &Ifupdown{}, // ifupdown 广泛支持，优先级最高
		&Netplan{},        // Netplan 次之
		&NetworkManager{}, // 优先级最低
	}

	for _, manager := range managers {
		if manager.IsInstall(ctx) {
			return manager, nil
		}
	}

	return nil, fmt.Errorf("no supported network manager found")
}
