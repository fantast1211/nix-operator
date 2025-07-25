package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// nodeRepository 节点配置操作实现
type nodeRepository struct {
	nodeDir string
	logger  *utils.Logger
}

// NewNodeRepository 创建节点仓库实例
func NewNodeRepository(nodeDir string, logger *utils.Logger) NodeRepository {
	return &nodeRepository{
		nodeDir: nodeDir,
		logger:  logger,
	}
}

// ListNodes 列出所有节点信息
func (r *nodeRepository) ListNodes(ctx context.Context) ([]*systemv1.NodeConfig, error) {
	r.logger.Debugf("repository", "Listing all nodes from %s", r.nodeDir)

	// 读取目录中的所有文件
	files, err := ioutil.ReadDir(r.nodeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read nodes directory: %w", err)
	}

	var nodes []*systemv1.NodeConfig

	// 遍历所有 JSON 文件
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(r.nodeDir, file.Name())
		r.logger.Debugf("repository", "Reading node file: %s", filePath)

		// 读取文件内容
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			r.logger.Warnf("repository", "Failed to read node file %s: %v", filePath, err)
			continue
		}

		// 解析节点配置文件
		var nodeConfigFile struct {
			Name         string `json:"name"`
			Hostname     string `json:"hostname"`
			IP           string `json:"ip"`
			MachineID    string `json:"machine-id"`
			LastModified string `json:"last-modified"`
		}
		if err := json.Unmarshal(data, &nodeConfigFile); err != nil {
			r.logger.Warnf("repository", "Failed to parse node config file %s: %v", filePath, err)
			continue
		}

		// 构建简化的NodeConfig
		nodeConfig := &systemv1.NodeConfig{
			Name:         nodeConfigFile.Name,
			Hostname:     nodeConfigFile.Hostname,
			Ip:           nodeConfigFile.IP,
			MachineId:    nodeConfigFile.MachineID,
			LastModified: nodeConfigFile.LastModified,
		}

		nodes = append(nodes, nodeConfig)
	}

	r.logger.Debugf("repository", "Found %d nodes", len(nodes))
	return nodes, nil
}