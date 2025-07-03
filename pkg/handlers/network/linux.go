package network

import (
	"context"
	"encoding/json"
	"fmt"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/utils"
)

type Config struct {
	Interfaces []Interface `json:"interfaces"`
}

type Interface struct {
	NodeSelector utils.NodeSelector `json:"nodeSelector"`
	Name         string             `json:"name"`
	IPAddress    string             `json:"ipAddress"`   // IPv4 地址
	IPv6Address  string             `json:"ipv6Address"` // IPv6 地址
	Gateway      string             `json:"gateway"`     // IPv4 网关
	IPv6Gateway  string             `json:"ipv6Gateway"` // IPv6 网关
	MTU          int                `json:"mtu"`
	MACAddress   string             `json:"macAddress"`
	Nameservers  []string           `json:"nameservers"`
}

func init() {
	handler := &LinuxNetworkHandler{
		managers: []INetworkManager{
			&NetworkManager{},
			// &Netplan{},
			// &Ifupdown{},
		},
	}
	controller.RegisterHandler("NetworkConfiguration", handler)
}

type LinuxNetworkHandler struct {
	managers []INetworkManager
}

func (h *LinuxNetworkHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

func (h *LinuxNetworkHandler) Reconcile(ctx context.Context, cfg *config.ResourceConfig) (*controller.ReconcileResult, error) {

	// 解析网络配置
	var networkSpec struct {
		Interfaces []Interface `yaml:"interfaces" json:"interfaces"`
	}

	// 将Spec转换为网络配置
	specBytes, err := json.Marshal(cfg.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %v", err)
	}

	if err := json.Unmarshal(specBytes, &networkSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal network spec: %v", err)
	}

	// 添加一个标志来跟踪是否至少有一个管理器被配置
	atLeastOneManagerConfigured := false

	for _, iface := range networkSpec.Interfaces {
		match, err := utils.MatchNodeSelector(iface.NodeSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to check node selector: %v", err)
		}
		if !match {
			continue
		}
		// 为每个已安装的网络管理器生成配置
		for _, manager := range h.managers {
			if !manager.IsInstall(ctx) {
				continue
			}
			atLeastOneManagerConfigured = true
			if err := manager.Configure(ctx, iface); err != nil {
				return nil, fmt.Errorf("failed to configure interface %s: %v", iface.Name, err)
			}
			if err := manager.ReloadIfy(ctx); err != nil {
				return nil, fmt.Errorf("failed to reload network configuration: %v", err)
			}
		}
	}

	// 检查是否至少有一个管理器被配置
	if !atLeastOneManagerConfigured {
		return nil, fmt.Errorf("no network manager is installed on this system")
	}

	// 如果成功完成，设置状态为成功
	// 创建结果对象
	result := &controller.ReconcileResult{
		Effective: cfg,
		Status: &config.ResourceStatus{
			Phase:   controller.PhaseReady,
			Message: "Network configuration applied successfully",
		},
	}
	return result, nil
}
