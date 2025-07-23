package kylinos

import (
	"context"
	"fmt"
	"testing"
	"time"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// ExampleKylinOSNetworkHandler_basicUsage 展示基本的 KylinOS 网络配置用法
func ExampleKylinOSNetworkHandler_basicUsage(t *testing.T) {
	// 创建测试用的 KylinOS 网络处理器
	osInfo := controller.OSInfo{
		ID:         "kylin",
		VersionID:  "V10",
		KernelName: "Linux",
		KernelVer:  "4.19.0",
	}

	handler := NewKylinOSNetworkHandlerForTest(osInfo)

	// 创建网络配置规格
	networkSpec := &systemv1.NetworkConfigurationSpec{
		Interfaces: []*systemv1.NetworkInterface{
			{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100/24",
				Ipv4Gateway: "192.168.1.1",
				Mtu:         1500,
				Nameservers: []string{"8.8.8.8", "8.8.4.4"},
			},
		},
	}

	// 序列化配置规格
	specData, err := marshalSpec(networkSpec)
	if err != nil {
		t.Fatalf("Failed to marshal spec: %v", err)
	}

	// 创建网络配置
	testConfigs := []*systemv1.ResourceConfig{
		{
			Metadata: &systemv1.Metadata{
				Name: "kylin-network-config",
			},
			Spec: specData,
		},
	}

	// 应用网络配置
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = handler.Reconcile(ctx, testConfigs)
	if err != nil {
		t.Fatalf("Failed to reconcile network config: %v", err)
	}

	t.Logf("KylinOS network configuration applied successfully")
}

// ExampleKylinOSNetworkHandler_bondingConfiguration 展示绑定网络配置
func ExampleKylinOSNetworkHandler_bondingConfiguration(t *testing.T) {
	// 创建测试用的 KylinOS 网络处理器
	osInfo := controller.OSInfo{
		ID:         "kylin",
		VersionID:  "V10",
		KernelName: "Linux",
		KernelVer:  "4.19.0",
	}

	handler := NewKylinOSNetworkHandlerForTest(osInfo)

	// 创建绑定网络配置规格
	networkSpec := &systemv1.NetworkConfigurationSpec{
		Interfaces: []*systemv1.NetworkInterface{
			{
				Name: "eth0",
				BondingSlave: &systemv1.BondingSlaveConfig{
					Enabled: true,
					Master:  "bond0",
				},
			},
			{
				Name: "eth1",
				BondingSlave: &systemv1.BondingSlaveConfig{
					Enabled: true,
					Master:  "bond0",
				},
			},
		},
	}

	// 序列化配置规格
	specData, err := marshalSpec(networkSpec)
	if err != nil {
		t.Fatalf("Failed to marshal spec: %v", err)
	}

	// 创建绑定网络配置
	testConfigs := []*systemv1.ResourceConfig{
		{
			Metadata: &systemv1.Metadata{
				Name: "kylin-bonding-config",
			},
			Spec: specData,
		},
	}

	// 应用绑定网络配置
	ctx := context.Background()
	_, err = handler.Reconcile(ctx, testConfigs)
	if err != nil {
		t.Fatalf("Failed to reconcile bonding config: %v", err)
	}

	t.Logf("KylinOS bonding configuration applied successfully")
}

// ExampleKylinOSNetworkHandler_ipv6Configuration 展示 IPv6 网络配置
func ExampleKylinOSNetworkHandler_ipv6Configuration(t *testing.T) {
	// 创建测试用的 KylinOS 网络处理器
	osInfo := controller.OSInfo{
		ID:         "kylin",
		VersionID:  "V10",
		KernelName: "Linux",
		KernelVer:  "4.19.0",
	}

	handler := NewKylinOSNetworkHandlerForTest(osInfo)

	// 创建 IPv6 网络配置规格
	networkSpec := &systemv1.NetworkConfigurationSpec{
		Interfaces: []*systemv1.NetworkInterface{
			{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100/24",
				Ipv6Address: "2001:db8::100/64",
				Ipv4Gateway: "192.168.1.1",
				Ipv6Gateway: "2001:db8::1",
				Mtu:         1500,
				Nameservers: []string{"8.8.8.8", "2001:4860:4860::8888"},
			},
		},
	}

	// 序列化配置规格
	specData, err := marshalSpec(networkSpec)
	if err != nil {
		t.Fatalf("Failed to marshal spec: %v", err)
	}

	// 创建 IPv6 网络配置
	testConfigs := []*systemv1.ResourceConfig{
		{
			Metadata: &systemv1.Metadata{
				Name: "kylin-ipv6-config",
			},
			Spec: specData,
		},
	}

	// 应用 IPv6 网络配置
	ctx := context.Background()
	_, err = handler.Reconcile(ctx, testConfigs)
	if err != nil {
		t.Fatalf("Failed to reconcile IPv6 config: %v", err)
	}

	t.Logf("KylinOS IPv6 configuration applied successfully")
}

// ExampleKylinOSNetworkHandler_multipleInterfaces 展示多接口网络配置
func ExampleKylinOSNetworkHandler_multipleInterfaces(t *testing.T) {
	// 创建测试用的 KylinOS 网络处理器
	osInfo := controller.OSInfo{
		ID:         "kylin",
		VersionID:  "V10",
		KernelName: "Linux",
		KernelVer:  "4.19.0",
	}

	handler := NewKylinOSNetworkHandlerForTest(osInfo)

	// 创建多接口网络配置规格
	networkSpec := &systemv1.NetworkConfigurationSpec{
		Interfaces: []*systemv1.NetworkInterface{
			{
				Name:        "eth0",
				Ipv4Address: "192.168.1.100/24",
				Ipv4Gateway: "192.168.1.1",
				Mtu:         1500,
				Nameservers: []string{"8.8.8.8"},
			},
			{
				Name:        "eth1",
				Ipv4Address: "10.0.0.100/8",
				Ipv4Gateway: "10.0.0.1",
				Mtu:         9000,
				Nameservers: []string{"8.8.4.4"},
			},
		},
	}

	// 序列化配置规格
	specData, err := marshalSpec(networkSpec)
	if err != nil {
		t.Fatalf("Failed to marshal spec: %v", err)
	}

	// 创建多接口网络配置
	testConfigs := []*systemv1.ResourceConfig{
		{
			Metadata: &systemv1.Metadata{
				Name: "kylin-multi-interface-config",
			},
			Spec: specData,
		},
	}

	// 应用多接口网络配置
	ctx := context.Background()
	_, err = handler.Reconcile(ctx, testConfigs)
	if err != nil {
		t.Fatalf("Failed to reconcile multi-interface config: %v", err)
	}

	t.Logf("KylinOS multi-interface configuration applied successfully")
}

// MarshalSpec 将 proto 消息序列化为 anypb.Any
func marshalSpec(msg proto.Message) (*anypb.Any, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is nil")
	}

	return anypb.New(msg)
}
