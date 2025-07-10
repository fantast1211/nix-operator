package network

import (
	"context"
)

type INetworkManager interface {
	IsInstall(ctx context.Context) bool
	Configure(ctx context.Context, iface Interface) error
	ReloadIfy(ctx context.Context) error
}
