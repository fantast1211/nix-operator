package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/utils"
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
		return nil, fmt.Errorf("failed to list hosts configurations: %v", err)
	}

	// 转换为API响应格式
	response := &v1.ListHostsConfigurationsResponse{
		Configurations: make([]*v1.HostsConfiguration, 0),
	}

	// 如果需要本地过滤，获取本机信息
	var localMachineID string
	var localIPs []string
	if req.LocalOnly {
		localMachineID, _ = utils.ReadMachineID()
		localIPs, _ = utils.GetLocalIPs()
	}

	for _, res := range resources {
		hostsConfigs, err := convertToHostsConfigurations(res)
		if err != nil {
			continue
		}

		// 如果启用了本地过滤，检查节点选择器
		if req.LocalOnly {
			filteredConfigs := make([]*v1.HostsConfiguration, 0)
			for _, config := range hostsConfigs {
				if config.NodeSelector != nil {
					// 检查machine ID匹配
					if config.NodeSelector.MachineId != "" && config.NodeSelector.MachineId == localMachineID {
						filteredConfigs = append(filteredConfigs, config)
						continue
					}
					// 检查IP匹配
					if config.NodeSelector.Ip != "" {
						for _, localIP := range localIPs {
							if config.NodeSelector.Ip == localIP {
								filteredConfigs = append(filteredConfigs, config)
								break
							}
						}
					}
				} else {
					// 没有节点选择器，认为是全局配置
					filteredConfigs = append(filteredConfigs, config)
				}
			}
			hostsConfigs = filteredConfigs
		}

		response.Configurations = append(response.Configurations, hostsConfigs...)

		// 设置配置名称（使用第一个资源的名称）
		if response.Name == "" && res.Config != nil {
			response.Name = res.Config.Metadata.Name
		}
	}

	return response, nil
}

// UpdateHostsConfiguration 更新hosts配置
func (s *SystemServiceServer) UpdateHostsConfiguration(ctx context.Context, req *v1.UpdateHostsConfigurationRequest) (*v1.UpdateHostsConfigurationResponse, error) {
	// 创建一个带超时的上下文
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 验证请求参数
	if req.Name == "" {
		return &v1.UpdateHostsConfigurationResponse{
			Success:      false,
			ErrorMessage: "configuration name is required",
		}, nil
	}

	if len(req.Configuration) == 0 {
		return &v1.UpdateHostsConfigurationResponse{
			Success:      false,
			ErrorMessage: "configuration is required",
		}, nil
	}

	// 获取现有配置
	existingRes, err := s.controller.GetResourceConfig(ctx, req.Name)
	var finalConfigurations []*v1.HostsConfiguration

	if err != nil || existingRes == nil {
		// 资源不存在，直接使用新配置创建
		finalConfigurations = req.Configuration
	} else {
		// 资源存在，需要进行配置合并
		existingConfigs, err := convertToHostsConfigurations(existingRes)
		if err != nil {
			return &v1.UpdateHostsConfigurationResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("failed to parse existing configuration: %v", err),
			}, nil
		}

		// 执行配置合并逻辑
		finalConfigurations = s.mergeHostsConfigurations(existingConfigs, req.Configuration)
	}

	// 转换为内部配置格式
	targetConfig, err := convertFromHostsConfigurations(req.Name, finalConfigurations)
	if err != nil {
		return &v1.UpdateHostsConfigurationResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to convert configuration: %v", err),
		}, nil
	}

	// 更新资源配置
	updatedRes, err := s.controller.UpdateResourceConfig(ctx, targetConfig)
	if err != nil {
		return &v1.UpdateHostsConfigurationResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to update configuration: %v", err),
		}, nil
	}

	// 转换为API响应格式
	updatedConfigs, err := convertToHostsConfigurations(updatedRes)
	if err != nil {
		return &v1.UpdateHostsConfigurationResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to convert updated configuration: %v", err),
		}, nil
	}

	return &v1.UpdateHostsConfigurationResponse{
		Configuration: updatedConfigs,
		Success:       true,
		Name:          req.Name,
	}, nil
}

// convertToHostsConfigurations 将内部资源配置转换为API的HostsConfiguration列表
func convertToHostsConfigurations(res *controller.ResourceWithStatus) ([]*v1.HostsConfiguration, error) {
	if res == nil || res.Config == nil {
		return nil, fmt.Errorf("resource or config is nil")
	}

	// 检查资源类型
	if res.Config.Kind != "HostsConfiguration" {
		return nil, fmt.Errorf("resource %s is not a hosts configuration", res.Config.Metadata.Name)
	}

	// 解析spec为HostsSpec
	var hostsSpec struct {
		Configurations []struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Hostname     string            `json:"hostname"`
			Hosts        []struct {
				IP        string   `json:"ip"`
				Hostnames []string `json:"hostnames"`
			} `json:"hosts"`
		} `json:"configurations"`
	}

	if err := json.Unmarshal(res.Config.Spec, &hostsSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hosts spec: %v", err)
	}

	// 转换为API格式
	var configurations []*v1.HostsConfiguration
	// 遍历所有配置项
	for _, configItem := range hostsSpec.Configurations {
		config := &v1.HostsConfiguration{
			Hostname: configItem.Hostname,
		}

		// 转换节点选择器
		if len(configItem.NodeSelector) > 0 {
			config.NodeSelector = &v1.NodeSelector{
				MachineId: configItem.NodeSelector["machineId"],
				Ip:        configItem.NodeSelector["ip"],
			}
		}

		// 转换hosts条目
		for _, host := range configItem.Hosts {
			hostEntry := &v1.HostEntry{
				Ip:        host.IP,
				Hostnames: host.Hostnames,
			}
			config.Hosts = append(config.Hosts, hostEntry)
		}

		configurations = append(configurations, config)
	}

	return configurations, nil
}

// mergeHostsConfigurations 合并现有配置和新配置
func (s *SystemServiceServer) mergeHostsConfigurations(existing []*v1.HostsConfiguration, new []*v1.HostsConfiguration) []*v1.HostsConfiguration {
	// 检查是否有任何新配置项没有 nodeSelector，如果有则进行全量替换
	for _, newConfig := range new {
		if newConfig.NodeSelector == nil || (newConfig.NodeSelector.MachineId == "" && newConfig.NodeSelector.Ip == "") {
			// 发现没有 nodeSelector 的配置项，进行全量替换
			return new
		}
	}

	// 所有新配置项都有 nodeSelector，进行选择性合并
	result := make([]*v1.HostsConfiguration, 0)

	// 首先复制所有现有配置项
	for _, existingConfig := range existing {
		matched := false
		// 检查是否有新配置项匹配当前现有配置项
		for _, newConfig := range new {
			if s.isNodeSelectorMatch(existingConfig.NodeSelector, newConfig.NodeSelector) {
				matched = true
				break
			}
		}
		// 如果没有匹配的新配置项，保留现有配置项
		if !matched {
			result = append(result, existingConfig)
		}
	}

	// 添加所有新配置项（替换匹配的，添加新的）
	for _, newConfig := range new {
		result = append(result, newConfig)
	}

	return result
}

// isNodeSelectorMatch 检查两个 nodeSelector 是否匹配
func (s *SystemServiceServer) isNodeSelectorMatch(existing, new *v1.NodeSelector) bool {
	// 如果其中一个为 nil，则不匹配
	if existing == nil || new == nil {
		return false
	}

	// 获取有效的字段值（非空字符串）
	existingMachineId := existing.MachineId
	existingIp := existing.Ip
	newMachineId := new.MachineId
	newIp := new.Ip

	// 如果两个 nodeSelector 都没有有效字段，则不匹配
	if (existingMachineId == "" && existingIp == "") || (newMachineId == "" && newIp == "") {
		return false
	}

	// 比较 machineId（如果两者都有值）
	if existingMachineId != "" && newMachineId != "" {
		if existingMachineId != newMachineId {
			return false
		}
	}

	// 比较 ip（如果两者都有值）
	if existingIp != "" && newIp != "" {
		if existingIp != newIp {
			return false
		}
	}

	// 至少有一个字段匹配，且没有冲突的字段
	return (existingMachineId == "" || newMachineId == "" || existingMachineId == newMachineId) &&
		(existingIp == "" || newIp == "" || existingIp == newIp) &&
		((existingMachineId != "" && newMachineId != "") || (existingIp != "" && newIp != ""))
}

// convertFromHostsConfigurations 将API的HostsConfiguration列表转换为内部资源配置
func convertFromHostsConfigurations(name string, configurations []*v1.HostsConfiguration) (*config.ResourceConfig, error) {
	if len(configurations) == 0 {
		return nil, fmt.Errorf("configurations list is empty")
	}

	// 构建内部spec格式
	hostsSpec := struct {
		Configurations []struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Hostname     string            `json:"hostname"`
			Hosts        []struct {
				IP        string   `json:"ip"`
				Hostnames []string `json:"hostnames"`
			} `json:"hosts"`
		} `json:"configurations"`
	}{
		Configurations: make([]struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Hostname     string            `json:"hostname"`
			Hosts        []struct {
				IP        string   `json:"ip"`
				Hostnames []string `json:"hostnames"`
			} `json:"hosts"`
		}, 0, len(configurations)),
	}

	// 转换每个配置项
	for _, config := range configurations {
		configItem := struct {
			NodeSelector map[string]string `json:"nodeSelector"`
			Hostname     string            `json:"hostname"`
			Hosts        []struct {
				IP        string   `json:"ip"`
				Hostnames []string `json:"hostnames"`
			} `json:"hosts"`
		}{
			Hostname:     config.Hostname,
			NodeSelector: make(map[string]string),
		}

		// 转换节点选择器
		if config.NodeSelector != nil {
			if config.NodeSelector.MachineId != "" {
				configItem.NodeSelector["machineId"] = config.NodeSelector.MachineId
			}
			if config.NodeSelector.Ip != "" {
				configItem.NodeSelector["ip"] = config.NodeSelector.Ip
			}
		}

		// 转换hosts条目
		for _, hostEntry := range config.Hosts {
			host := struct {
				IP        string   `json:"ip"`
				Hostnames []string `json:"hostnames"`
			}{
				IP:        hostEntry.Ip,
				Hostnames: hostEntry.Hostnames,
			}
			configItem.Hosts = append(configItem.Hosts, host)
		}

		hostsSpec.Configurations = append(hostsSpec.Configurations, configItem)
	}

	// 序列化spec
	specBytes, err := json.Marshal(hostsSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal hosts spec: %v", err)
	}

	// 创建资源配置
	cfg := &config.ResourceConfig{
		APIVersion: "sysconfig.operator/v1",
		Kind:       "HostsConfiguration",
		Metadata: config.Metadata{
			Name: name,
		},
		Spec: specBytes,
	}

	return cfg, nil
}
