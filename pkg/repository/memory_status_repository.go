package repository

import (
	"context"
	"fmt"
	"sync"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// memoryStatusRepository 内存状态仓库实现
type memoryStatusRepository struct {
	mu       sync.RWMutex
	statuses map[string]*systemv1.ResourceStatus // key: resource name
	kindMap  map[string]string                    // key: resource name, value: kind
	logger   *utils.Logger
}

// NewMemoryStatusRepository 创建内存状态仓库实例
func NewMemoryStatusRepository(logger *utils.Logger) StatusRepository {
	return &memoryStatusRepository{
		statuses: make(map[string]*systemv1.ResourceStatus),
		kindMap:  make(map[string]string),
		logger:   logger,
	}
}

// GetStatus 获取资源状态
func (r *memoryStatusRepository) GetStatus(ctx context.Context, name string) (*systemv1.ResourceStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status, exists := r.statuses[name]
	if !exists {
		return nil, fmt.Errorf("status not found for resource: %s", name)
	}

	// 返回状态的深拷贝以避免并发修改
	return &systemv1.ResourceStatus{
		Phase:   status.Phase,
		Reason:  status.Reason,
		Message: status.Message,
	}, nil
}

// SetStatus 设置资源状态
func (r *memoryStatusRepository) SetStatus(ctx context.Context, name string, status *systemv1.ResourceStatus) error {
	if status == nil {
		return fmt.Errorf("status cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 存储状态的深拷贝
	r.statuses[name] = &systemv1.ResourceStatus{
		Phase:   status.Phase,
		Reason:  status.Reason,
		Message: status.Message,
	}

	r.logger.Debugf("status_repo", "Set status for resource %s: %s (%s)", name, status.Phase, status.Reason)
	return nil
}

// SetStatusWithKind 设置资源状态并记录kind信息
func (r *memoryStatusRepository) SetStatusWithKind(ctx context.Context, name, kind string, status *systemv1.ResourceStatus) error {
	if err := r.SetStatus(ctx, name, status); err != nil {
		return err
	}

	r.mu.Lock()
	r.kindMap[name] = kind
	r.mu.Unlock()

	return nil
}

// ListStatuses 列出指定类型的所有状态
func (r *memoryStatusRepository) ListStatuses(ctx context.Context, kind string) (map[string]*systemv1.ResourceStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*systemv1.ResourceStatus)
	for name, resourceKind := range r.kindMap {
		if resourceKind == kind {
			if status, exists := r.statuses[name]; exists {
				// 返回状态的深拷贝
				result[name] = &systemv1.ResourceStatus{
					Phase:   status.Phase,
					Reason:  status.Reason,
					Message: status.Message,
				}
			}
		}
	}

	r.logger.Debugf("status_repo", "Listed %d statuses for kind: %s", len(result), kind)
	return result, nil
}

// DeleteStatus 删除资源状态
func (r *memoryStatusRepository) DeleteStatus(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.statuses, name)
	delete(r.kindMap, name)

	r.logger.Debugf("status_repo", "Deleted status for resource: %s", name)
	return nil
}

// Clear 清空所有状态缓存
func (r *memoryStatusRepository) Clear(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.statuses = make(map[string]*systemv1.ResourceStatus)
	r.kindMap = make(map[string]string)

	r.logger.Debug("status_repo", "Cleared all status cache")
	return nil
}

// GetAllStatuses 获取所有状态（用于调试）
func (r *memoryStatusRepository) GetAllStatuses(ctx context.Context) map[string]*systemv1.ResourceStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*systemv1.ResourceStatus)
	for name, status := range r.statuses {
		result[name] = &systemv1.ResourceStatus{
			Phase:   status.Phase,
			Reason:  status.Reason,
			Message: status.Message,
		}
	}
	return result
}