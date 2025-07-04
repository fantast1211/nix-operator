package webservice

import "go.xbrother.com/nix-operator/pkg/interfaces"

// Service 业务服务层接口聚合
type Service interface {
	interfaces.ResourceService
}