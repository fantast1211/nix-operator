package bond

import (
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/centos"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/kylinos"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/openeuler"
)

// init 注册Bond处理器
func init() {
	// 1. CentOS 7-7.9 专用Bond处理器（最高优先级）
	controller.RegisterHandler("BondConfiguration", centos.NewCentOSBondHandler())

	// 2. KylinOS V10+ 专用Bond处理器
	controller.RegisterHandler("BondConfiguration", kylinos.NewKylinOSBondHandler())

	// 3. OpenEuler 20.x-24.x 专用Bond处理器
	controller.RegisterHandler("BondConfiguration", openeuler.NewOpenEulerBondHandler())

	// 注意：这里可以添加其他操作系统的Bond处理器
	// 4. 通用 Linux Bond处理器（兜底处理器）
	// controller.RegisterHandler("BondConfiguration", &linux.LinuxBondHandler{})
}
