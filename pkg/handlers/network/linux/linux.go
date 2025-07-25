package linux

import (
	"context"
	"fmt"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

type LinuxNetworkHandler struct {
	osInfo controller.OSInfo
}

func (h *LinuxNetworkHandler) Match(osInfo controller.OSInfo) bool {
	if osInfo.KernelName == "Linux" {
		h.osInfo = osInfo
		return true
	}
	return false
}

func (h *LinuxNetworkHandler) Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*status.ReconcileResult, error) {
	utils.Infof("network", "Starting network configuration reconciliation for config: %s", config.Metadata.Name)

	// 解析配置规格
	networkSpec, err := utils.UnmarshalSpec[*systemv1.NetworkConfigurationSpec](config.Spec)
	if err != nil {
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "SpecError",
				Message: fmt.Sprintf("failed to unmarshal spec: %v", err),
			},
			Error: err,
		}, nil
	}

	// 应用配置
	result, err := h.applyConfiguration(ctx, config, networkSpec)
	if err != nil {
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "ReconcileError",
				Message: fmt.Sprintf("failed to apply configuration: %v", err),
			},
			Error: err,
		}, nil
	}

	return result, nil
}

// applyConfiguration 应用网络配置
func (h *LinuxNetworkHandler) applyConfiguration(ctx context.Context, config *systemv1.ResourceConfig, networkSpec *systemv1.NetworkConfigurationSpec) (*status.ReconcileResult, error) {
	utils.Info("network", "Starting Linux network configuration application")
	utils.Infof("network", "Configuration contains %d network interfaces", len(networkSpec.Interfaces))

	// 检测并选择合适的网络管理器
	utils.Info("network", "Detecting available network manager")
	manager, err := h.detectNetworkManager(ctx)
	if err != nil {
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "DetectManagerError",
				Message: fmt.Sprintf("failed to detect network manager: %v", err),
			},
			Error: err,
		}, nil
	}

	// 跟踪是否有任何配置变更
	hasChanges := false

	// 遍历并配置所有网络接口
	utils.Info("network", "Starting configuration of network interfaces")
	for i, interfaceSpec := range networkSpec.Interfaces {
		utils.Infof("network", "Configuring interface %d/%d: %s", i+1, len(networkSpec.Interfaces), interfaceSpec.Name)

		// 转换为内部接口结构
		iface := types.Interface{
			Name:        interfaceSpec.Name,
			IPv4Address: interfaceSpec.Ipv4Address,
			IPv6Address: interfaceSpec.Ipv6Address,
			IPv4Gateway: interfaceSpec.Ipv4Gateway,
			IPv6Gateway: interfaceSpec.Ipv6Gateway,
			MTU:         int(interfaceSpec.Mtu),
			Nameservers: interfaceSpec.Nameservers,
		}
		utils.Infof("network", "Interface %s configuration: IPv4=%s, IPv6=%s, MTU=%d", iface.Name, iface.IPv4Address, iface.IPv6Address, iface.MTU)

		// 使用ConfigureWithCheck检查配置是否有变更
		utils.Infof("network", "Applying configuration for interface %s", iface.Name)
		changed, err := manager.ConfigureWithCheck(ctx, iface)
		if err != nil {
			return &status.ReconcileResult{
				Config: config,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  "ConfigureInterfaceError",
					Message: fmt.Sprintf("failed to configure network interface %s: %v", interfaceSpec.Name, err),
				},
				Error: err,
			}, nil
		}
		if changed {
			utils.Infof("network", "Interface %s configuration changed", iface.Name)
			hasChanges = true
		} else {
			utils.Infof("network", "Interface %s configuration unchanged", iface.Name)
		}
	}

	// 只有在有配置变更时才重新加载网络配置
	utils.Infof("network", "Configuration summary: %d interfaces processed, changes detected: %t", len(networkSpec.Interfaces), hasChanges)
	if hasChanges {
		utils.Info("network", "Network configuration changed, reloading network services")
		if err := manager.ReloadIfy(ctx); err != nil {
			return &status.ReconcileResult{
				Config: config,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  "ReloadError",
					Message: fmt.Sprintf("failed to reload network configuration: %v", err),
				},
				Error: err,
			}, nil
		}
		utils.Info("network", "Network services reloaded successfully")
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  "Configured",
				Message: "Network configuration applied and reloaded successfully",
			},
		}, nil
	} else {
		utils.Info("network", "Network configuration unchanged, skipping network service reload")
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  "Configured",
				Message: "Network configuration verified, no changes needed",
			},
		}, nil
	}
}

// detectNetworkManager 检测系统中可用的网络管理器
func (h *LinuxNetworkHandler) detectNetworkManager(ctx context.Context) (types.INetworkManager, error) {
	// 按优先级检测网络管理器：NetworkManager > Netplan > ifupdown
	managers := []types.INetworkManager{
		&NetworkManager{}, // NetworkManager 优先级最高，现代Linux发行版首选
		// &Netplan{},        // Netplan 次之，Ubuntu 18.04+默认
		&Ifupdown{}, // ifupdown 兜底，传统Debian/Ubuntu系统
	}

	for _, manager := range managers {
		if manager.IsInstall(ctx) {
			utils.Info("network", fmt.Sprintf("Selected network manager: %T on %s %s", manager, h.osInfo.ID, h.osInfo.VersionID))
			return manager, nil
		}
	}

	return nil, fmt.Errorf("no supported network manager found on %s %s", h.osInfo.ID, h.osInfo.VersionID)
}
