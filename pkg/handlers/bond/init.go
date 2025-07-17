package bond

import (
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/centos"
)

// init 注册Bond处理器
func init() {
	// 1. CentOS 7-7.9 专用Bond处理器（最高优先级）
	controller.RegisterHandler("BondConfiguration", centos.NewCentOSBondHandler())

	// 注意：这里可以添加其他操作系统的Bond处理器
	// 2. 通用 Linux Bond处理器（兜底处理器）
	// controller.RegisterHandler("BondConfiguration", &linux.LinuxBondHandler{})
}
