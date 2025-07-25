package time

import (
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/time/ubuntu"
)

// init 注册时间处理器
func init() {
	// 1. Ubuntu 专用时间处理器（高优先级）
	controller.RegisterHandler("TimeConfiguration", ubuntu.NewUbuntuTimeHandler())

	// 2. 通用 Linux 时间处理器（兜底处理器）
	controller.RegisterHandler("TimeConfiguration", &LinuxTimeHandler{})

	// 注意：这里可以添加其他操作系统的时间处理器
	// 例如：CentOS、OpenEuler等特定处理器
}