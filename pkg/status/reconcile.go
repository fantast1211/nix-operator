package status

import (
	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/domain"
)

// 资源状态常量
const (
	PhaseReady   = "Ready"
	PhaseError   = "Error"
	PhaseUnknown = "Unknown"
	PhaseSkipped = "Skipped"

	ReasonNoHandler          = "NoHandler"
	ReasonReconcileError     = "ReconcileError"
	ReasonNoChange           = "NoChange"
	ReasonSkippedByConfig    = "SkippedByConfig"
	ReasonInitial            = "Initial"
	ReasonSpecError          = "SpecError"
	ReasonNodeSelectorError  = "NodeSelectorError"
	ReasonMatched            = "Matched"
	ReasonNotMatched         = "NotMatched"
	ReasonFallback           = "Fallback"
)

// ReconcileError 返回调谐失败
func ReconcileError(cfg *systemv1.ResourceConfig, reason string, err error) (*domain.ReconcileResult, error) {
	return &domain.ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseError,
			Reason:  reason,
			Message: err.Error(),
		},
		Effective: cfg,
	}, nil
}

// ReconcileReady 返回调谐成功
func ReconcileReady(cfg *systemv1.ResourceConfig, reason, message string) (*domain.ReconcileResult, error) {
	return &domain.ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseReady,
			Reason:  reason,
			Message: message,
		},
		Effective: cfg,
	}, nil
}

// ReconcileNoChange 返回配置无变化
func ReconcileNoChange(cfg *systemv1.ResourceConfig, message string) (*domain.ReconcileResult, error) {
	return &domain.ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseReady,
			Reason:  ReasonNoChange,
			Message: message,
		},
		Effective: cfg,
	}, nil
}

// ReconcileSkipped 返回调谐被跳过（如条件不满足）
func ReconcileSkipped(cfg *systemv1.ResourceConfig, reason, message string) (*domain.ReconcileResult, error) {
	return &domain.ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseSkipped,
			Reason:  reason,
			Message: message,
		},
		Effective: cfg,
	}, nil
}

// ReconcileUnknown 用于调谐状态不明确的情况（初始化/错误恢复）
func ReconcileUnknown(cfg *systemv1.ResourceConfig, reason, message string) (*domain.ReconcileResult, error) {
	return &domain.ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseUnknown,
			Reason:  reason,
			Message: message,
		},
		Effective: cfg,
	}, nil
}
