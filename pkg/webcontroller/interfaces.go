package webcontroller

import "go.xbrother.com/nix-operator/pkg/interfaces"

// Controller Web控制器接口聚合
type Controller interface {
	interfaces.ResourceController
}