package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/network/centos"
	"go.xbrother.com/nix-operator/pkg/handlers/network/kylinos"
	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
)

// 简单的非侵入性测试，用于验证网络配置调谐功能
func main() {
	fmt.Println("=== 网络配置调谐测试 ===")

	// 测试接口配置
	testInterface := &types.Interface{
		Name:         "eth0",
		IPv4Address:  "192.168.1.100/24",
		IPv4Gateway:  "192.168.1.1",
		Nameservers:  []string{"8.8.8.8", "8.8.4.4"},
		MTU:          1500,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 测试 CentOS NetworkManager
	fmt.Println("\n--- 测试 CentOS NetworkManager ---")
	testCentOSNetworkManager(ctx, testInterface)

	// 测试 KylinOS NetworkManager
	fmt.Println("\n--- 测试 KylinOS NetworkManager ---")
	testKylinOSNetworkManager(ctx, testInterface)

	fmt.Println("\n=== 测试完成 ===")
}

func testCentOSNetworkManager(ctx context.Context, iface *types.Interface) {
	// 创建模拟的 OSInfo
	osInfo := &controller.OSInfo{
		ID:        "centos",
		VersionID: "7.9",
	}

	// 创建 CentOS NetworkManager 实例
	manager := centos.NewCentOSNetworkManager(osInfo)

	// 检查是否安装
	if !manager.IsInstall(ctx) {
		fmt.Println("CentOS NetworkManager 未安装，跳过测试")
		return
	}

	fmt.Printf("测试接口: %s\n", iface.Name)
	fmt.Printf("期望配置: IPv4=%s, Gateway=%s, MTU=%d\n", iface.IPv4Address, iface.IPv4Gateway, iface.MTU)

	// 调用 ConfigureWithCheck 方法
	changed, err := manager.ConfigureWithCheck(ctx, *iface)
	if err != nil {
		log.Printf("CentOS NetworkManager 配置检查失败: %v", err)
		return
	}

	fmt.Printf("配置检查结果: 需要变更=%t\n", changed)

	if changed {
		fmt.Println("检测到配置变更，建议执行网络重载")
	} else {
		fmt.Println("配置无变更，无需重载网络")
	}
}

func testKylinOSNetworkManager(ctx context.Context, iface *types.Interface) {
	// 创建模拟的 OSInfo
	osInfo := &controller.OSInfo{
		ID:        "kylin",
		VersionID: "V10",
	}

	// 创建 KylinOS NetworkManager 实例
	manager := kylinos.NewKylinOSNetworkManager(osInfo)

	// 检查是否安装
	if !manager.IsInstall(ctx) {
		fmt.Println("KylinOS NetworkManager 未安装，跳过测试")
		return
	}

	fmt.Printf("测试接口: %s\n", iface.Name)
	fmt.Printf("期望配置: IPv4=%s, Gateway=%s, MTU=%d\n", iface.IPv4Address, iface.IPv4Gateway, iface.MTU)

	// 调用 ConfigureWithCheck 方法
	changed, err := manager.ConfigureWithCheck(ctx, *iface)
	if err != nil {
		log.Printf("KylinOS NetworkManager 配置检查失败: %v", err)
		return
	}

	fmt.Printf("配置检查结果: 需要变更=%t\n", changed)

	if changed {
		fmt.Println("检测到配置变更，建议执行网络重载")
	} else {
		fmt.Println("配置无变更，无需重载网络")
	}
}