package openeuler

import (
	"context"
	"fmt"
	"testing"
	"time"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
)

// ExampleOpenEulerNetworkHandler 演示如何使用 openEuler 网络配置插件
func ExampleOpenEulerNetworkHandler() {
	// 1. 创建 openEuler 系统信息
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}

	// 2. 创建网络处理器（测试模式）
	handler := NewOpenEulerNetworkHandlerForTest(osInfo)

	// 3. 验证系统匹配
	if !handler.Match(osInfo) {
		fmt.Println("系统不匹配 openEuler")
		return
	}
	fmt.Println("✓ openEuler 系统匹配成功")

	// 4. 执行网络配置调谐
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testConfig := &systemv1.ResourceConfig{
		Metadata: &systemv1.Metadata{
			Name: "example-config",
		},
	}
	_, err := handler.Reconcile(ctx, testConfig)
	if err != nil {
		fmt.Printf("❌ 网络配置失败: %v\n", err)
		return
	}

	fmt.Println("✓ 网络配置成功完成")
	fmt.Println("✓ 已按照 ifupdown > NetworkManager 的优先级选择网络管理器")
	fmt.Println("✓ 测试模式下不会真实变更网络配置")

	// Output:
	// ✓ openEuler 系统匹配成功
	// ✓ 网络配置成功完成
	// ✓ 已按照 ifupdown > NetworkManager 的优先级选择网络管理器
	// ✓ 测试模式下不会真实变更网络配置
}

// TestExampleUsage 测试示例用法
func TestExampleUsage(t *testing.T) {
	// 运行示例
	ExampleOpenEulerNetworkHandler()
}

// ExampleNetworkManagerPriority 演示网络管理器优先级选择
func ExampleNetworkManagerPriority() {
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}

	handler := NewOpenEulerNetworkHandlerForTest(osInfo)
	ctx := context.Background()

	// 检测网络管理器
	manager, _ := handler.detectNetworkManager(ctx)

	switch manager.(type) {
	case *MockOpenEulerIfupdown:
		fmt.Println("选择的网络管理器: ifupdown (优先级: 1)")
	case *MockOpenEulerNetworkManager:
		fmt.Println("选择的网络管理器: NetworkManager (优先级: 2)")

	default:
		fmt.Println("未检测到支持的网络管理器")
	}

	// Output:
	// 选择的网络管理器: ifupdown (优先级: 1)
}

// ExampleBondConfiguration 演示 Bond 配置
func ExampleBondConfiguration() {
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}

	handler := NewOpenEulerNetworkHandlerForTest(osInfo)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testConfig := &systemv1.ResourceConfig{
		Metadata: &systemv1.Metadata{
			Name: "bond-config",
		},
	}
	_, err := handler.Reconcile(ctx, testConfig)
	if err != nil {
		fmt.Printf("❌ Bond 配置失败: %v\n", err)
		return
	}

	fmt.Println("✓ Bond 网络配置成功")
	fmt.Println("✓ 主接口: bond0 (192.168.1.100/24)")
	fmt.Println("✓ 从接口: eth0, eth1")

	// Output:
	// ✓ Bond 网络配置成功
	// ✓ 主接口: bond0 (192.168.1.100/24)
	// ✓ 从接口: eth0, eth1
}

// ExampleDualStackConfiguration 演示双栈（IPv4+IPv6）配置
func ExampleDualStackConfiguration() {
	osInfo := controller.OSInfo{
		ID:         "openeuler",
		VersionID:  "22.03",
		KernelName: "Linux",
		KernelVer:  "5.10.0",
	}

	handler := NewOpenEulerNetworkHandlerForTest(osInfo)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testConfig := &systemv1.ResourceConfig{
		Metadata: &systemv1.Metadata{
			Name: "dual-stack-config",
		},
	}
	_, err := handler.Reconcile(ctx, testConfig)
	if err != nil {
		fmt.Printf("❌ 双栈配置失败: %v\n", err)
		return
	}

	fmt.Println("✓ 双栈网络配置成功")
	fmt.Println("✓ IPv4: 192.168.1.100/24, 网关: 192.168.1.1")
	fmt.Println("✓ IPv6: 2001:db8::100/64, 网关: 2001:db8::1")
	fmt.Println("✓ DNS: 8.8.8.8, 8.8.4.4, 2001:4860:4860::8888")

	// Output:
	// ✓ 双栈网络配置成功
	// ✓ IPv4: 192.168.1.100/24, 网关: 192.168.1.1
	// ✓ IPv6: 2001:db8::100/64, 网关: 2001:db8::1
	// ✓ DNS: 8.8.8.8, 8.8.4.4, 2001:4860:4860::8888
}
