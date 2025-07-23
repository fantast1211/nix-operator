package status

import (
	"context"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/repository"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// Manager 状态管理器接口
type Manager interface {
	// UpdateStatus 更新资源状态
	UpdateStatus(ctx context.Context, config *systemv1.ResourceConfig, phase, reason, message string) error
	// UpdateStatusWithGeneration 更新资源状态并设置ObservedGeneration
	UpdateStatusWithGeneration(ctx context.Context, config *systemv1.ResourceConfig, phase, reason, message string) error
	// SetPendingStatus 设置资源为Pending状态
	SetPendingStatus(ctx context.Context, config *systemv1.ResourceConfig) error
}

// manager 状态管理器实现
type manager struct {
	statusRepo repository.StatusRepository
	logger     *utils.Logger
}

// NewManager 创建状态管理器实例
func NewManager(statusRepo repository.StatusRepository, logger *utils.Logger) Manager {
	return &manager{
		statusRepo: statusRepo,
		logger:     logger,
	}
}

// UpdateStatus 更新资源状态
func (m *manager) UpdateStatus(ctx context.Context, config *systemv1.ResourceConfig, phase, reason, message string) error {
	status := &systemv1.ResourceStatus{
		Phase:   phase,
		Reason:  reason,
		Message: message,
	}

	err := m.statusRepo.SetStatusWithKind(ctx, config.Metadata.Name, config.Kind, status)
	if err != nil {
		m.logger.Errorf("status_manager", "Failed to update status for %s/%s: %v", config.Kind, config.Metadata.Name, err)
		return err
	}

	m.logger.Debugf("status_manager", "Updated status for %s/%s: %s (%s)", config.Kind, config.Metadata.Name, phase, reason)
	return nil
}

// UpdateStatusWithGeneration 更新资源状态并设置ObservedGeneration
func (m *manager) UpdateStatusWithGeneration(ctx context.Context, config *systemv1.ResourceConfig, phase, reason, message string) error {
	status := &systemv1.ResourceStatus{
		Phase:              phase,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: config.Metadata.Generation,
	}

	err := m.statusRepo.SetStatusWithKind(ctx, config.Metadata.Name, config.Kind, status)
	if err != nil {
		m.logger.Errorf("status_manager", "Failed to update status for %s/%s: %v", config.Kind, config.Metadata.Name, err)
		return err
	}

	m.logger.Debugf("status_manager", "Updated status for %s/%s: %s (%s) with generation %d", 
		config.Kind, config.Metadata.Name, phase, reason, config.Metadata.Generation)
	return nil
}

// SetPendingStatus 设置资源为Pending状态
func (m *manager) SetPendingStatus(ctx context.Context, config *systemv1.ResourceConfig) error {
	return m.UpdateStatus(ctx, config, "Pending", "ConfigurationCreated", "Configuration created, waiting for reconciliation")
}