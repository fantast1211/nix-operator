package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// NodeInfo 节点信息结构
type NodeInfo struct {
	MachineID string `json:"machineId"`
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
}

// NodeConfigFile 简化的节点配置文件结构
type NodeConfigFile struct {
	Name         string `json:"name"`
	Hostname     string `json:"hostname"`
	IP           string `json:"ip"`
	MachineID    string `json:"machine-id"`
	LastModified string `json:"last-modified"`
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

	// 创建简化的配置文件结构
	configFile := &NodeConfigFile{
		Name:         fmt.Sprintf("node-%s", nodeInfo.MachineID),
		Hostname:     nodeInfo.Hostname,
		IP:           nodeInfo.IP,
		MachineID:    nodeInfo.MachineID,
		LastModified: fmt.Sprintf("%d", time.Now().Unix()),
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

	// 检查节点信息是否有变化
	if existingConfig.IP != currentNodeInfo.IP ||
		existingConfig.Hostname != currentNodeInfo.Hostname ||
		existingConfig.MachineID != currentNodeInfo.MachineID {
		return true, nil
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