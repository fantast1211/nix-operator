package network

import (
	"go.xbrother.com/nix-operator/pkg/controller"
)

// init 函数用于注册所有网络处理器
// 注册顺序很重要：越具体的处理器优先级越高
func init() {
	// 1. CentOS 7.2-7.9 专用处理器（最高优先级）
	controller.RegisterHandler("NetworkConfiguration", NewCentOSNetworkHandler())

	// 2. 通用 Linux 处理器（兜底处理器）
	controller.RegisterHandler("NetworkConfiguration", &LinuxNetworkHandler{})

	// 未来可以添加其他操作系统的专用处理器：
	// controller.RegisterHandler("NetworkConfiguration", NewUbuntuNetworkHandler())
	// controller.RegisterHandler("NetworkConfiguration", NewDebianNetworkHandler())
	// controller.RegisterHandler("NetworkConfiguration", NewFedoraNetworkHandler())
	// controller.RegisterHandler("NetworkConfiguration", NewOpenSUSENetworkHandler())
	// controller.RegisterHandler("NetworkConfiguration", NewArchNetworkHandler())
}