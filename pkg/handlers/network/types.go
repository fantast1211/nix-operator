package network

import (
	"context"
	"os/exec"
)

type INetworkManager interface {
	IsInstall(ctx context.Context) bool
	Configure(ctx context.Context, iface Interface) error
	ReloadIfy(ctx context.Context) error
}

// 检查服务是否启动
func isServiceActive(ctx context.Context, service string) bool {
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", service)
	return cmd.Run() == nil
}
