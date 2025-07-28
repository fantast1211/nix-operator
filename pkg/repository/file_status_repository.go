package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/utils"
	"google.golang.org/protobuf/encoding/protojson"
)

// fileStatusRepository 基于文件的状态仓库实现
type fileStatusRepository struct {
	statusDir string
	mu        sync.RWMutex
	logger    *utils.Logger
}

// NewFileStatusRepository 创建基于文件的状态仓库实例
func NewFileStatusRepository(statusDir string, logger *utils.Logger) StatusRepository {
	return &fileStatusRepository{
		statusDir: statusDir,
		logger:    logger,
	}
}

// GetStatus 获取资源状态
func (r *fileStatusRepository) GetStatus(ctx context.Context, name string) (*systemv1.ResourceStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 扫描所有kind子目录，查找对应的状态文件
	var foundStatus *systemv1.ResourceStatus
	err := filepath.Walk(r.statusDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 检查是否是目标状态文件
		if !info.IsDir() && info.Name() == name+".json" {
			data, err := os.ReadFile(path)
			if err != nil {
				r.logger.Warnf("status_repo", "Error reading status file %s: %v", path, err)
				return nil // 继续查找其他文件
			}

			status := &systemv1.ResourceStatus{}
			if err := protojson.Unmarshal(data, status); err != nil {
				r.logger.Warnf("status_repo", "Error unmarshaling status file %s: %v", path, err)
				return nil // 继续查找其他文件
			}

			foundStatus = status
			r.logger.Debugf("status_repo", "Retrieved status for resource %s: %s (%s)", name, status.Phase, status.Reason)
			return filepath.SkipDir // 找到后停止搜索
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk status directory: %w", err)
	}

	if foundStatus == nil {
		r.logger.Debugf("status_repo", "Status not found for resource: %s", name)
	}

	return foundStatus, nil // 如果没找到，返回nil
}

// SetStatus 设置资源状态
func (r *fileStatusRepository) SetStatus(ctx context.Context, name string, status *systemv1.ResourceStatus) error {
	if status == nil {
		return fmt.Errorf("status cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 序列化状态
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		Indent:          "  ",
	}
	data, err := marshaler.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	// 确保状态目录存在
	if err := os.MkdirAll(r.statusDir, 0755); err != nil {
		return fmt.Errorf("failed to create status directory: %w", err)
	}

	// 构建状态文件路径
	statusPath := filepath.Join(r.statusDir, name+".json")

	// 原子性写入文件
	if err := utils.AtomicWriteFile(data, statusPath, 0644); err != nil {
		return fmt.Errorf("failed to write status file: %w", err)
	}

	r.logger.Debugf("status_repo", "Set status for resource %s: %s (%s)", name, status.Phase, status.Reason)
	return nil
}

// SetStatusWithKind 设置资源状态并记录kind信息
func (r *fileStatusRepository) SetStatusWithKind(ctx context.Context, name, kind string, status *systemv1.ResourceStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if status == nil {
		return fmt.Errorf("status cannot be nil")
	}

	// 序列化状态
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,
		Indent:          "  ",
	}
	data, err := marshaler.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}
	// 创建kind子目录
	kindDir := filepath.Join(r.statusDir, kind)
	if err := os.MkdirAll(kindDir, 0755); err != nil {
		return fmt.Errorf("failed to create kind directory: %w", err)
	}

	// 构建状态文件路径（在kind子目录中）
	statusPath := filepath.Join(kindDir, name+".json")
	// 原子性写入文件
	if err := utils.AtomicWriteFile(data, statusPath, 0644); err != nil {
		return fmt.Errorf("failed to write status file: %w", err)
	}

	r.logger.Debugf("status_repo", "Set status for resource %s/%s: %s (%s) ,ObservedGeneration: %v", kind, name, status.Phase, status.Reason, status.ObservedGeneration)
	return nil
}

// ListStatuses 列出指定类型的所有状态
func (r *fileStatusRepository) ListStatuses(ctx context.Context, kind string) (map[string]*systemv1.ResourceStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*systemv1.ResourceStatus)

	// 如果指定了kind，只扫描对应的子目录
	scanDir := r.statusDir
	if kind != "" {
		scanDir = filepath.Join(r.statusDir, kind)
		// 检查kind目录是否存在
		if _, err := os.Stat(scanDir); os.IsNotExist(err) {
			r.logger.Debugf("status_repo", "Kind directory %s does not exist, returning empty result", scanDir)
			return result, nil
		}
	}

	// 扫描状态目录
	err := filepath.Walk(scanDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过非JSON文件
		if !utils.IsJSONFile(info) {
			return nil
		}

		// 读取状态文件
		data, err := os.ReadFile(path)
		if err != nil {
			r.logger.Warnf("status_repo", "Error reading status file %s: %v", path, err)
			return nil // 继续处理其他文件
		}

		status := &systemv1.ResourceStatus{}
		if err := protojson.Unmarshal(data, status); err != nil {
			r.logger.Warnf("status_repo", "Error unmarshaling status file %s: %v", path, err)
			return nil // 继续处理其他文件
		}

		// 从文件名提取资源名称
		resourceName := strings.TrimSuffix(info.Name(), ".json")
		result[resourceName] = status

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk status directory: %w", err)
	}

	r.logger.Debugf("status_repo", "Listed %d statuses for kind: %s", len(result), kind)
	return result, nil
}

// DeleteStatus 删除资源状态
func (r *fileStatusRepository) DeleteStatus(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 扫描所有kind子目录，查找并删除对应的状态文件
	var deleted bool
	err := filepath.Walk(r.statusDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 检查是否是目标状态文件
		if !info.IsDir() && info.Name() == name+".json" {
			if err := os.Remove(path); err != nil {
				r.logger.Warnf("status_repo", "Failed to delete status file %s: %v", path, err)
			} else {
				r.logger.Debugf("status_repo", "Deleted status file: %s", path)
				deleted = true
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk status directory: %w", err)
	}

	if !deleted {
		r.logger.Debugf("status_repo", "Status file for resource %s does not exist", name)
	}

	return nil
}

// Clear 清空所有状态缓存
func (r *fileStatusRepository) Clear(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 删除状态目录中的所有JSON文件
	err := filepath.Walk(r.statusDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录和非JSON文件
		if info.IsDir() || !utils.IsJSONFile(info) {
			return nil
		}

		if err := os.Remove(path); err != nil {
			r.logger.Warnf("status_repo", "Failed to remove status file %s: %v", path, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to clear status directory: %w", err)
	}

	r.logger.Debug("status_repo", "Cleared all status files")
	return nil
}
