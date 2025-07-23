package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

// NodeInfo 节点信息结构
type NodeInfo struct {
	MachineID string `json:"machineId"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
}

// NodeConfigFile 节点配置文件结构
type NodeConfigFile struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Metadata   NodeConfigMetadata     `json:"metadata"`
	Spec       *anypb.Any             `json:"spec"`
	Status     interface{}            `json:"status"`
}

// NodeConfigMetadata 节点配置元数据
type NodeConfigMetadata struct {
	Name            string            `json:"name"`
	ResourceVersion string            `json:"resourceVersion"`
	Generation      int64             `json:"generation"`
	CreationTime    string            `json:"creationTime"`
	DeletionTime    string            `json:"deletionTime"`
	Labels          map[string]string `json:"labels"`
	Annotations     map[string]string `json:"annotations"`
}

// NodeConfigSpec 节点配置规范
type NodeConfigSpec struct {
	NodeSelector *systemv1.NodeSelector `json:"nodeSelector"`
	NodeInfo     *NodeInfo              `json:"nodeInfo"`
}

// GetCurrentNodeInfo 获取当前节点信息
func GetCurrentNodeInfo() (*NodeInfo, error) {
	// 获取机器ID
	machineID, err := ReadMachineID()
	if err != nil {
		return nil, fmt.Errorf("failed to get machine ID: %v", err)
	}

	// 获取主要IP地址
	ip, err := GetPrimaryIP()
	if err != nil {
		return nil, fmt.Errorf("failed to get primary IP: %v", err)
	}

	// 获取主机名
	hostname, err := GetHostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %v", err)
	}

	return &NodeInfo{
		MachineID: machineID,
		IP:        ip,
		Hostname:  hostname,
	}, nil
}

// GenerateNodeConfigFile 生成节点配置文件
func GenerateNodeConfigFile(nodeDir string) error {
	// 确保节点目录存在
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return fmt.Errorf("failed to create node directory: %v", err)
	}

	// 获取当前节点信息
	nodeInfo, err := GetCurrentNodeInfo()
	if err != nil {
		return fmt.Errorf("failed to get current node info: %v", err)
	}

	// 将规范转换为anypb.Any
	specAny, err := anypb.New(&systemv1.NodeSelector{
		MachineId: nodeInfo.MachineID,
	})
	if err != nil {
		return fmt.Errorf("failed to create spec any: %v", err)
	}

	// 创建配置文件结构
	configFile := &NodeConfigFile{
		APIVersion: "system.xbrother.com/v1",
		Kind:       "NodeConfiguration",
		Metadata: NodeConfigMetadata{
			Name:            fmt.Sprintf("node-%s", nodeInfo.MachineID),
			ResourceVersion: fmt.Sprintf("%d", time.Now().Unix()),
			Generation:      1,
			CreationTime:    time.Now().Format(time.RFC3339),
			DeletionTime:    "",
			Labels:          make(map[string]string),
			Annotations: map[string]string{
				"auto-generated": "true",
				"machine-id":     nodeInfo.MachineID,
				"ip":             nodeInfo.IP,
				"hostname":       nodeInfo.Hostname,
			},
		},
		Spec:   specAny,
		Status: nil,
	}

	// 生成配置文件路径
	configFilePath := filepath.Join(nodeDir, fmt.Sprintf("node-%s.json", nodeInfo.MachineID))

	// 检查文件是否已存在，如果存在则检查是否需要更新
	if needsUpdate, err := checkNodeConfigNeedsUpdate(configFilePath, nodeInfo); err != nil {
		return fmt.Errorf("failed to check if node config needs update: %v", err)
	} else if !needsUpdate {
		// 文件已存在且不需要更新
		return nil
	}

	// 将配置文件写入磁盘
	configData, err := json.MarshalIndent(configFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config file: %v", err)
	}

	if err := os.WriteFile(configFilePath, configData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	Infof("node-config", "Generated node configuration file: %s", configFilePath)
	return nil
}

// checkNodeConfigNeedsUpdate 检查节点配置文件是否需要更新
func checkNodeConfigNeedsUpdate(configFilePath string, currentNodeInfo *NodeInfo) (bool, error) {
	// 如果文件不存在，需要创建
	if _, err := os.Stat(configFilePath); os.IsNotExist(err) {
		return true, nil
	}

	// 读取现有配置文件
	existingData, err := os.ReadFile(configFilePath)
	if err != nil {
		return true, nil // 读取失败，重新生成
	}

	var existingConfig NodeConfigFile
	if err := json.Unmarshal(existingData, &existingConfig); err != nil {
		return true, nil // 解析失败，重新生成
	}

	// 检查注解中的信息是否有变化
	if existingConfig.Metadata.Annotations != nil {
		existingIP := existingConfig.Metadata.Annotations["ip"]
		existingHostname := existingConfig.Metadata.Annotations["hostname"]
		existingMachineID := existingConfig.Metadata.Annotations["machine-id"]

		// 如果关键信息有变化，需要更新
		if existingIP != currentNodeInfo.IP ||
			existingHostname != currentNodeInfo.Hostname ||
			existingMachineID != currentNodeInfo.MachineID {
			return true, nil
		}
	}

	// 不需要更新
	return false, nil
}

// CheckAndUpdateNodeConfig 检查并更新节点配置文件（在调谐完成后调用）
func CheckAndUpdateNodeConfig(nodeDir string) error {
	// 确保目录存在
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return fmt.Errorf("failed to create node directory: %w", err)
	}

	// 调用生成节点配置文件的函数
	return GenerateNodeConfigFile(nodeDir)
}