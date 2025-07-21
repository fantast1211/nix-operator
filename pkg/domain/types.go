package domain

import (
	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
)

// ReconcileResult 调谐结果
type ReconcileResult struct {
	Effective *systemv1.ResourceConfig
	Status    *systemv1.ResourceStatus
}
