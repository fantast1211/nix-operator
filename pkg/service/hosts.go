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

// SystemServiceServer 实现 SystemService 接口
type SystemServiceServer struct {
	v1.UnimplementedSystemServiceServer
	controller *controller.Controller
}

// NewSystemServiceServer 创建一个新的SystemServiceServer实例
func NewSystemServiceServer(controller *controller.Controller) *SystemServiceServer {
	return &SystemServiceServer{
		controller: controller,
	}
}

// ListHostsConfigurations 获取所有hosts配置
func (s *SystemServiceServer) ListHostsConfigurations(ctx context.Context, req *v1.ListHostsConfigurationsRequest) (*v1.ListHostsConfigurationsResponse, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 从控制器获取资源配置
	resources, err := s.controller.ListResourceConfigs(ctx, "HostsConfiguration")
	if err != nil {
		return nil, err
	}

	// 转换为API响应格式
	response := &v1.ListHostsConfigurationsResponse{
		Configurations: make([]*v1.HostsConfiguration, 0, len(resources)),
	}

	for _, res := range resources {
		hostsConfig, err := convertToHostsConfiguration(res)
		if err != nil {
			continue
		}
		response.Configurations = append(response.Configurations, hostsConfig)
	}

	return response, nil
}

// GetHostsConfiguration 获取单个hosts配置
func (s *SystemServiceServer) GetHostsConfiguration(ctx context.Context, req *v1.GetHostsConfigurationRequest) (*v1.GetHostsConfigurationResponse, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 从控制器获取资源配置
	res, err := s.controller.GetResourceConfig(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	if res == nil {
		return nil, fmt.Errorf("hosts configuration not found: %s", req.Name)
	}

	// 检查资源类型
	if res.Config.Kind != "HostsConfiguration" {
		return nil, fmt.Errorf("resource %s is not a hosts configuration", req.Name)
	}

	// 转换为API响应格式
	hostsConfig, err := convertToHostsConfiguration(res)
	if err != nil {
		return nil, err
	}

	return &v1.GetHostsConfigurationResponse{
		Configuration: hostsConfig,
	}, nil
}

// UpdateHostsConfiguration 更新hosts配置
func (s *SystemServiceServer) UpdateHostsConfiguration(ctx context.Context, req *v1.UpdateHostsConfigurationRequest) (*v1.UpdateHostsConfigurationResponse, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 验证请求
	if req.Configuration == nil {
		return nil, fmt.Errorf("invalid request: configuration is nil")
	}

	// 确保名称匹配
	if req.Configuration.Name != req.Name {
		return nil, fmt.Errorf("name in path (%s) does not match name in body (%s)", req.Name, req.Configuration.Name)
	}

	// 转换为内部配置格式
	cfg, err := convertFromHostsConfiguration(req.Configuration)
	if err != nil {
		return nil, err
	}

	// 更新资源配置
	updatedRes, err := s.controller.UpdateResourceConfig(ctx, cfg)
	if err != nil {
		return &v1.UpdateHostsConfigurationResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// 转换为API响应格式
	hostsConfig, err := convertToHostsConfiguration(updatedRes)
	if err != nil {
		return &v1.UpdateHostsConfigurationResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to convert response: %v", err),
		}, nil
	}

	return &v1.UpdateHostsConfigurationResponse{
		Configuration: hostsConfig,
		Success:       true,
	}, nil
}

// convertToHostsConfiguration 将内部资源配置转换为API的HostsConfiguration
func convertToHostsConfiguration(res *controller.ResourceWithStatus) (*v1.HostsConfiguration, error) {
	if res == nil || res.Config == nil {
		return nil, fmt.Errorf("resource or config is nil")
	}

	// 解析spec为HostsSpec
	var hostsSpec struct {
		Interfaces []struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Hostname     string            `json:"hostname"`
			Hosts        []struct {
				IP        string   `json:"ip"`
				Hostnames []string `json:"hostnames"`
			} `json:"hosts"`
		} `json:"interfaces"`
	}

	if err := json.Unmarshal(res.Config.Spec, &hostsSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hosts spec: %v", err)
	}

	// 创建API的HostsConfiguration
	hostsConfig := &v1.HostsConfiguration{
		Name: res.Config.Metadata.Name,
	}

	// 如果有接口配置，使用第一个接口的配置
	if len(hostsSpec.Interfaces) > 0 {
		iface := hostsSpec.Interfaces[0]

		// 设置节点选择器
		hostsConfig.NodeSelector = &v1.NodeSelector{
			Labels: iface.NodeSelector,
		}

		// 设置主机名
		hostsConfig.Hostname = iface.Hostname

		// 设置hosts条目
		hostsConfig.Hosts = make([]*v1.HostEntry, 0, len(iface.Hosts))
		for _, host := range iface.Hosts {
			hostsConfig.Hosts = append(hostsConfig.Hosts, &v1.HostEntry{
				Ip:        host.IP,
				Hostnames: host.Hostnames,
			})
		}
	}

	return hostsConfig, nil
}

// convertFromHostsConfiguration 将API的HostsConfiguration转换为内部资源配置
func convertFromHostsConfiguration(hostsConfig *v1.HostsConfiguration) (*config.ResourceConfig, error) {
	if hostsConfig == nil {
		return nil, fmt.Errorf("hosts configuration is nil")
	}

	// 创建内部配置
	cfg := &config.ResourceConfig{
		APIVersion: "sysconfig.operator/v1",
		Kind:       "HostsConfiguration",
		Metadata: config.Metadata{
			Name: hostsConfig.Name,
		},
	}

	// 创建spec
	hostsSpec := struct {
		Interfaces []struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Hostname     string            `json:"hostname"`
			Hosts        []struct {
				IP        string   `json:"ip"`
				Hostnames []string `json:"hostnames"`
			} `json:"hosts"`
		} `json:"interfaces"`
	}{
		Interfaces: []struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Hostname     string            `json:"hostname"`
			Hosts        []struct {
				IP        string   `json:"ip"`
				Hostnames []string `json:"hostnames"`
			} `json:"hosts"`
		}{
			{
				NodeSelector: make(map[string]string),
				Hostname:     hostsConfig.Hostname,
				Hosts: make([]struct {
					IP        string   `json:"ip"`
					Hostnames []string `json:"hostnames"`
				}, 0, len(hostsConfig.Hosts)),
			},
		},
	}

	// 设置节点选择器
	if hostsConfig.NodeSelector != nil {
		hostsSpec.Interfaces[0].NodeSelector = hostsConfig.NodeSelector.Labels
	}

	// 设置hosts条目
	for _, host := range hostsConfig.Hosts {
		hostsSpec.Interfaces[0].Hosts = append(hostsSpec.Interfaces[0].Hosts, struct {
			IP        string   `json:"ip"`
			Hostnames []string `json:"hostnames"`
		}{
			IP:        host.Ip,
			Hostnames: host.Hostnames,
		})
	}

	// 将spec转换为JSON
	specBytes, err := json.Marshal(hostsSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hosts spec: %v", err)
	}

	cfg.Spec = specBytes
	return cfg, nil
}
