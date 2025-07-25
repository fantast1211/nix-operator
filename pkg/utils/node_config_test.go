package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGenerateNodeConfigFile 测试节点配置文件生成功能
func TestGenerateNodeConfigFile(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "node-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 生成节点配置文件
	err = GenerateNodeConfigFile(tempDir)
	if err != nil {
		t.Fatalf("Failed to generate node config file: %v", err)
	}

	// 获取当前节点信息用于验证
	nodeInfo, err := GetCurrentNodeInfo()
	if err != nil {
		t.Fatalf("Failed to get current node info: %v", err)
	}

	// 检查文件是否存在
	filePath := filepath.Join(tempDir, "node-"+nodeInfo.MachineID+".json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("Node config file was not created: %s", filePath)
	}

	// 读取并验证文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read node config file: %v", err)
	}

	var config NodeConfigFile
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("Failed to parse node config file: %v", err)
	}

	// 验证基本字段
	if config.Name != "node-"+nodeInfo.MachineID {
		t.Errorf("Expected name 'node-%s', got '%s'", nodeInfo.MachineID, config.Name)
	}
	if config.Hostname != nodeInfo.Hostname {
		t.Errorf("Expected hostname '%s', got '%s'", nodeInfo.Hostname, config.Hostname)
	}
	if config.IP != nodeInfo.IP {
		t.Errorf("Expected IP '%s', got '%s'", nodeInfo.IP, config.IP)
	}
	if config.MachineID != nodeInfo.MachineID {
		t.Errorf("Expected MachineID '%s', got '%s'", nodeInfo.MachineID, config.MachineID)
	}
	if config.LastModified == "" {
		t.Error("LastModified should not be empty")
	}
}

// TestCheckAndUpdateNodeConfig 测试节点配置文件检查更新功能
func TestCheckAndUpdateNodeConfig(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "node-config-update-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 第一次调用，应该创建文件
	err = CheckAndUpdateNodeConfig(tempDir)
	if err != nil {
		t.Fatalf("Failed to check and update node config: %v", err)
	}

	// 获取当前节点信息
	nodeInfo, err := GetCurrentNodeInfo()
	if err != nil {
		t.Fatalf("Failed to get current node info: %v", err)
	}

	// 检查文件是否存在
	filePath := filepath.Join(tempDir, "node-"+nodeInfo.MachineID+".json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("Node config file was not created: %s", filePath)
	}

	// 获取文件的修改时间
	fileInfo1, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to get file info: %v", err)
	}

	// 等待一秒确保时间戳不同
	time.Sleep(1 * time.Second)

	// 第二次调用，如果没有变化应该不更新文件
	err = CheckAndUpdateNodeConfig(tempDir)
	if err != nil {
		t.Fatalf("Failed to check and update node config (second call): %v", err)
	}

	// 检查文件修改时间是否相同（表示没有更新）
	fileInfo2, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to get file info (second check): %v", err)
	}

	if !fileInfo1.ModTime().Equal(fileInfo2.ModTime()) {
		t.Error("File should not have been updated when no changes occurred")
	}
}

// TestGetCurrentNodeInfo 测试获取当前节点信息功能
func TestGetCurrentNodeInfo(t *testing.T) {
	nodeInfo, err := GetCurrentNodeInfo()
	if err != nil {
		t.Fatalf("Failed to get current node info: %v", err)
	}

	// 验证基本字段不为空
	if nodeInfo.MachineID == "" {
		t.Error("MachineID should not be empty")
	}
	if nodeInfo.IP == "" {
		t.Error("IP should not be empty")
	}
	if nodeInfo.Hostname == "" {
		t.Error("Hostname should not be empty")
	}

	// 验证字段格式
	if len(nodeInfo.MachineID) < 10 {
		t.Errorf("MachineID seems too short: %s", nodeInfo.MachineID)
	}

	t.Logf("Node info: MachineID=%s, IP=%s, Hostname=%s", 
		nodeInfo.MachineID, nodeInfo.IP, nodeInfo.Hostname)
}