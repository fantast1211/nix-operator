package status

import (
	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
)

// ReconcileResult 表示调谐操作的结果
type ReconcileResult struct {
	// Config 被处理的配置
	Config *systemv1.ResourceConfig
	// Status 调谐后的状态
	Status *systemv1.ResourceStatus
	// Error 调谐过程中的错误
	Error error
}

// 资源状态常量
const (
	PhaseReady   = "Ready"
	PhaseError   = "Error"
	PhaseUnknown = "Unknown"
	PhasePending = "Pending"

	ReasonNoHandler             = "NoHandler"
	ReasonReconcileError        = "ReconcileError"
	ReasonAppliedSuccessfully   = "AppliedSuccessfully"   // 原ReasonNoChange，表示配置已成功应用
	ReasonConfigurationUpdated  = "ConfigurationUpdated"  // 配置有变化并成功应用
	ReasonInitial               = "Initial"
	ReasonSpecError             = "SpecError"
	ReasonNodeSelectorError     = "NodeSelectorError"
	ReasonMatched               = "Matched"
	ReasonNotMatched            = "NotMatched"
	ReasonFallback              = "Fallback"

	// 向后兼容的别名
	ReasonNoChange = ReasonAppliedSuccessfully
)

// ReconcileError 返回调谐失败
func ReconcileError(cfg *systemv1.ResourceConfig, reason string, err error) (*ReconcileResult, error) {
	return &ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseError,
			Reason:  reason,
			Message: err.Error(),
		},
	}, nil
}

// ReconcileReady 返回调谐成功
func ReconcileReady(cfg *systemv1.ResourceConfig, reason, message string) (*ReconcileResult, error) {
	return &ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseReady,
			Reason:  reason,
			Message: message,
		},
	}, nil
}

// ReconcileNoChange 返回配置无变化（向后兼容）
func ReconcileNoChange(cfg *systemv1.ResourceConfig, message string) (*ReconcileResult, error) {
	return &ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseReady,
			Reason:  ReasonAppliedSuccessfully,
			Message: message,
		},
	}, nil
}

// ReconcileAppliedSuccessfully 返回配置已成功应用（推荐使用）
func ReconcileAppliedSuccessfully(cfg *systemv1.ResourceConfig, message string) (*ReconcileResult, error) {
	return &ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseReady,
			Reason:  ReasonAppliedSuccessfully,
			Message: message,
		},
	}, nil
}

// ReconcileConfigurationUpdated 返回配置有变化并成功应用
func ReconcileConfigurationUpdated(cfg *systemv1.ResourceConfig, message string) (*ReconcileResult, error) {
	return &ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseReady,
			Reason:  ReasonConfigurationUpdated,
			Message: message,
		},
	}, nil
}

// ReconcilePending 返回配置处于待处理状态
func ReconcilePending(cfg *systemv1.ResourceConfig, reason, message string) (*ReconcileResult, error) {
	return &ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhasePending,
			Reason:  reason,
			Message: message,
		},
	}, nil
}

// ReconcileUnknown 用于调谐状态不明确的情况（初始化/错误恢复）
func ReconcileUnknown(cfg *systemv1.ResourceConfig, reason, message string) (*ReconcileResult, error) {
	return &ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   PhaseUnknown,
			Reason:  reason,
			Message: message,
		},
	}, nil
}
