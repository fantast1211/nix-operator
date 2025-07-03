package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
)

// ListNetworkInterfaces 获取所有网络接口
func (s *SystemServiceServer) ListNetworkInterfaces(ctx context.Context, req *v1.ListNetworkInterfacesRequest) (*v1.ListNetworkInterfacesResponse, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 从控制器获取资源配置
	resources, err := s.controller.ListResourceConfigs(ctx, "NetworkConfiguration")
	if err != nil {
		return nil, err
	}

	// 转换为API响应格式
	response := &v1.ListNetworkInterfacesResponse{
		Interfaces: make([]*v1.NetworkInterface, 0, len(resources)),
	}

	for _, res := range resources {
		networkInterfaces, err := convertToNetworkInterfaces(res)
		if err != nil {
			continue
		}
		response.Interfaces = append(response.Interfaces, networkInterfaces...)
	}

	return response, nil
}

// GetNetworkInterface 获取单个网络接口
func (s *SystemServiceServer) GetNetworkInterface(ctx context.Context, req *v1.GetNetworkInterfaceRequest) (*v1.GetNetworkInterfaceResponse, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 从控制器获取资源配置
	resources, err := s.controller.ListResourceConfigs(ctx, "NetworkConfiguration")
	if err != nil {
		return nil, err
	}

	// 查找指定名称的网络接口
	for _, res := range resources {
		networkInterfaces, err := convertToNetworkInterfaces(res)
		if err != nil {
			continue
		}

		for _, iface := range networkInterfaces {
			if iface.Name == req.Name {
				return &v1.GetNetworkInterfaceResponse{
					Interface: iface,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("network interface not found: %s", req.Name)
}

// UpdateNetworkInterface 更新网络接口配置
func (s *SystemServiceServer) UpdateNetworkInterface(ctx context.Context, req *v1.UpdateNetworkInterfaceRequest) (*v1.UpdateNetworkInterfaceResponse, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 验证请求
	if req.Interface == nil {
		return nil, fmt.Errorf("invalid request: interface is nil")
	}

	// 确保名称匹配
	if req.Interface.Name != req.Name {
		return nil, fmt.Errorf("name in path (%s) does not match name in body (%s)", req.Name, req.Interface.Name)
	}

	// 获取现有配置
	resources, err := s.controller.ListResourceConfigs(ctx, "NetworkConfiguration")
	if err != nil {
		return nil, err
	}

	// 查找包含该接口的配置
	var targetConfig *config.ResourceConfig

	for _, res := range resources {
		var networkSpec struct {
			Interfaces []struct {
				NodeSelector map[string]string `json:"nodeSelector"`
				Name         string            `json:"name"`
				IPAddress    string            `json:"ipAddress"`
				IPv6Address  string            `json:"ipv6Address"`
				Gateway      string            `json:"gateway"`
				IPv6Gateway  string            `json:"ipv6Gateway"`
				MTU          int               `json:"mtu"`
				MACAddress   string            `json:"macAddress"`
				Nameservers  []string          `json:"nameservers"`
			} `json:"interfaces"`
		}

		if err := json.Unmarshal(res.Config.Spec, &networkSpec); err != nil {
			continue
		}

		for i, iface := range networkSpec.Interfaces {
			if iface.Name == req.Name {
				targetConfig = res.Config

				// 更新接口配置
				networkSpec.Interfaces[i] = updateInterfaceFromAPI(networkSpec.Interfaces[i], req.Interface)

				// 将更新后的spec转换为JSON
				specBytes, err := json.Marshal(networkSpec)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal network spec: %v", err)
				}

				targetConfig.Spec = specBytes
				break
			}
		}

		if targetConfig != nil {
			break
		}
	}

	// 如果没有找到现有配置，创建新配置
	if targetConfig == nil {
		targetConfig = convertFromNetworkInterface(req.Interface)
	}

	// 更新资源配置
	updatedRes, err := s.controller.UpdateResourceConfig(ctx, targetConfig)
	if err != nil {
		return &v1.UpdateNetworkInterfaceResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// 转换为API响应格式
	networkInterfaces, err := convertToNetworkInterfaces(updatedRes)
	if err != nil {
		return &v1.UpdateNetworkInterfaceResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to convert response: %v", err),
		}, nil
	}

	// 查找更新后的接口
	var updatedInterface *v1.NetworkInterface
	for _, iface := range networkInterfaces {
		if iface.Name == req.Name {
			updatedInterface = iface
			break
		}
	}

	if updatedInterface == nil {
		return &v1.UpdateNetworkInterfaceResponse{
			Success:      false,
			ErrorMessage: "interface not found in updated configuration",
		}, nil
	}

	return &v1.UpdateNetworkInterfaceResponse{
		Interface: updatedInterface,
		Success:   true,
	}, nil
}

// convertToNetworkInterfaces 将内部资源配置转换为API的NetworkInterface列表
func convertToNetworkInterfaces(res *controller.ResourceWithStatus) ([]*v1.NetworkInterface, error) {
	if res == nil || res.Config == nil {
		return nil, fmt.Errorf("resource or config is nil")
	}

	// 检查资源类型
	if res.Config.Kind != "NetworkConfiguration" {
		return nil, fmt.Errorf("resource %s is not a network configuration", res.Config.Metadata.Name)
	}

	// 解析spec为NetworkSpec
	var networkSpec struct {
		Interfaces []struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Name         string            `json:"name"`
			IPAddress    string            `json:"ipAddress"`
			IPv6Address  string            `json:"ipv6Address"`
			Gateway      string            `json:"gateway"`
			IPv6Gateway  string            `json:"ipv6Gateway"`
			MTU          int               `json:"mtu"`
			MACAddress   string            `json:"macAddress"`
			Nameservers  []string          `json:"nameservers"`
		} `json:"interfaces"`
	}

	if err := json.Unmarshal(res.Config.Spec, &networkSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal network spec: %v", err)
	}

	// 创建API的NetworkInterface列表
	networkInterfaces := make([]*v1.NetworkInterface, 0, len(networkSpec.Interfaces))

	for _, iface := range networkSpec.Interfaces {
		// 创建NetworkInterface
		networkInterface := &v1.NetworkInterface{
			Name:       iface.Name,
			MacAddress: iface.MACAddress,
			Mtu:        int32(iface.MTU),
			Status:     v1.InterfaceStatus_Unknown, // 默认状态
		}

		// 设置节点选择器
		if len(iface.NodeSelector) > 0 {
			networkInterface.NodeSelector = &v1.NodeSelector{
				Labels: iface.NodeSelector,
			}
		}

		// 设置IPv4配置
		if iface.IPAddress != "" || iface.Gateway != "" {
			networkInterface.Ipv4 = &v1.IPv4Config{
				Address:     iface.IPAddress,
				Gateway:     iface.Gateway,
				DhcpEnabled: false, // 默认不启用DHCP
			}
		}

		// 设置IPv6配置
		if iface.IPv6Address != "" || iface.IPv6Gateway != "" {
			networkInterface.Ipv6 = &v1.IPv6Config{
				Address:      iface.IPv6Address,
				Gateway:      iface.IPv6Gateway,
				SlaacEnabled: false, // 默认不启用SLAAC
			}
		}

		networkInterfaces = append(networkInterfaces, networkInterface)
	}

	return networkInterfaces, nil
}

// convertFromNetworkInterface 将API的NetworkInterface转换为内部资源配置
func convertFromNetworkInterface(networkInterface *v1.NetworkInterface) *config.ResourceConfig {
	if networkInterface == nil {
		return nil
	}

	// 创建内部配置
	cfg := &config.ResourceConfig{
		APIVersion: "sysconfig.operator/v1",
		Kind:       "NetworkConfiguration",
		Metadata: config.Metadata{
			Name: networkInterface.Name,
		},
	}

	// 创建spec
	networkSpec := struct {
		Interfaces []struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Name         string            `json:"name"`
			IPAddress    string            `json:"ipAddress"`
			IPv6Address  string            `json:"ipv6Address"`
			Gateway      string            `json:"gateway"`
			IPv6Gateway  string            `json:"ipv6Gateway"`
			MTU          int               `json:"mtu"`
			MACAddress   string            `json:"macAddress"`
			Nameservers  []string          `json:"nameservers"`
		} `json:"interfaces"`
	}{
		Interfaces: []struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Name         string            `json:"name"`
			IPAddress    string            `json:"ipAddress"`
			IPv6Address  string            `json:"ipv6Address"`
			Gateway      string            `json:"gateway"`
			IPv6Gateway  string            `json:"ipv6Gateway"`
			MTU          int               `json:"mtu"`
			MACAddress   string            `json:"macAddress"`
			Nameservers  []string          `json:"nameservers"`
		}{
			{
				NodeSelector: make(map[string]string),
				Name:         networkInterface.Name,
				MACAddress:   networkInterface.MacAddress,
				MTU:          int(networkInterface.Mtu),
				Nameservers:  []string{},
			},
		},
	}

	// 设置节点选择器
	if networkInterface.NodeSelector != nil {
		networkSpec.Interfaces[0].NodeSelector = networkInterface.NodeSelector.Labels
	}

	// 设置IPv4配置
	if networkInterface.Ipv4 != nil {
		networkSpec.Interfaces[0].IPAddress = networkInterface.Ipv4.Address
		networkSpec.Interfaces[0].Gateway = networkInterface.Ipv4.Gateway
	}

	// 设置IPv6配置
	if networkInterface.Ipv6 != nil {
		networkSpec.Interfaces[0].IPv6Address = networkInterface.Ipv6.Address
		networkSpec.Interfaces[0].IPv6Gateway = networkInterface.Ipv6.Gateway
	}

	// 将spec转换为JSON
	specBytes, err := json.Marshal(networkSpec)
	if err != nil {
		return nil
	}

	cfg.Spec = specBytes
	return cfg
}

// updateInterfaceFromAPI 使用API的NetworkInterface更新内部接口配置
func updateInterfaceFromAPI(internalIface struct {
	NodeSelector map[string]string `json:"nodeSelector"`
	Name         string            `json:"name"`
	IPAddress    string            `json:"ipAddress"`
	IPv6Address  string            `json:"ipv6Address"`
	Gateway      string            `json:"gateway"`
	IPv6Gateway  string            `json:"ipv6Gateway"`
	MTU          int               `json:"mtu"`
	MACAddress   string            `json:"macAddress"`
	Nameservers  []string          `json:"nameservers"`
}, apiIface *v1.NetworkInterface) struct {
	NodeSelector map[string]string `json:"nodeSelector"`
	Name         string            `json:"name"`
	IPAddress    string            `json:"ipAddress"`
	IPv6Address  string            `json:"ipv6Address"`
	Gateway      string            `json:"gateway"`
	IPv6Gateway  string            `json:"ipv6Gateway"`
	MTU          int               `json:"mtu"`
	MACAddress   string            `json:"macAddress"`
	Nameservers  []string          `json:"nameservers"`
} {
	// 更新MAC地址和MTU
	if apiIface.MacAddress != "" {
		internalIface.MACAddress = apiIface.MacAddress
	}
	if apiIface.Mtu > 0 {
		internalIface.MTU = int(apiIface.Mtu)
	}

	// 更新节点选择器
	if apiIface.NodeSelector != nil && len(apiIface.NodeSelector.Labels) > 0 {
		internalIface.NodeSelector = apiIface.NodeSelector.Labels
	}

	// 更新IPv4配置
	if apiIface.Ipv4 != nil {
		if apiIface.Ipv4.Address != "" {
			internalIface.IPAddress = apiIface.Ipv4.Address
		}
		if apiIface.Ipv4.Gateway != "" {
			internalIface.Gateway = apiIface.Ipv4.Gateway
		}
	}

	// 更新IPv6配置
	if apiIface.Ipv6 != nil {
		if apiIface.Ipv6.Address != "" {
			internalIface.IPv6Address = apiIface.Ipv6.Address
		}
		if apiIface.Ipv6.Gateway != "" {
			internalIface.IPv6Gateway = apiIface.Ipv6.Gateway
		}
	}

	return internalIface
}
