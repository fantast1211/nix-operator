package network

import (
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/centos"
	"go.xbrother.com/nix-operator/pkg/handlers/network/kylinos"
	"go.xbrother.com/nix-operator/pkg/handlers/network/linux"
	"go.xbrother.com/nix-operator/pkg/handlers/network/openeuler"
)

// init 函数用于注册所有网络处理器
// 注册顺序很重要：越具体的处理器优先级越高
// 一旦匹配到一个处理器就直接不会再匹配之后的了
func init() {
	// 1. CentOS 7-7.9 专用处理器（最高优先级）
	controller.RegisterHandler("NetworkConfiguration", centos.NewCentOSNetworkHandler())

	// 2. openEuler 专用处理器
	controller.RegisterHandler("NetworkConfiguration", openeuler.NewOpenEulerNetworkHandler())

	// 3. KylinOS 专用处理器
	controller.RegisterHandler("NetworkConfiguration", kylinos.NewKylinOSNetworkHandler())

	// 4. 通用 Linux 处理器（兜底处理器）
	controller.RegisterHandler("NetworkConfiguration", &linux.LinuxNetworkHandler{})

	// 未来可以添加其他操作系统的专用处理器：
	// controller.RegisterHandler("NetworkConfiguration", NewUbuntuNetworkHandler())
	// controller.RegisterHandler("NetworkConfiguration", NewDebianNetworkHandler())
	// controller.RegisterHandler("NetworkConfiguration", NewFedoraNetworkHandler())
	// controller.RegisterHandler("NetworkConfiguration", NewOpenSUSENetworkHandler())
	// controller.RegisterHandler("NetworkConfiguration", NewArchNetworkHandler())
}
