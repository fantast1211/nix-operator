package webrepository

import "go.xbrother.com/nix-operator/pkg/interfaces"

// Repository 仓储层接口聚合
type Repository interface {
	interfaces.ConfigRepository
	interfaces.ReconcileRepository
}