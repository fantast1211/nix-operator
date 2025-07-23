package main

import (
	"fmt"
	"log"
	"time"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/validator"
	"google.golang.org/protobuf/types/known/anypb"
)

// 演示自动校验器注册系统的使用
func main() {
	fmt.Println("=== 自动校验器注册系统演示 ===")

	// 1. 创建自动校验器注册表
	registry := validator.NewAutoValidatorRegistry()

	// 2. 执行自动扫描
	fmt.Println("\n正在执行自动扫描...")
	err := registry.AutoScan()
	if err != nil {
		log.Fatalf("自动扫描失败: %v", err)
	}

	// 3. 显示已注册的校验器类型
	fmt.Println("\n已注册的校验器类型:")
	kinds := registry.ListRegisteredKinds()
	for i, kind := range kinds {
		fmt.Printf("  %d. %s\n", i+1, kind)
	}

	// 4. 演示配置校验
	fmt.Println("\n=== 配置校验演示 ===")

	// 创建一个测试配置
	hostsSpec := &systemv1.HostsConfigurationSpec{
		NodeSelector: &systemv1.NodeSelector{
			MachineId: "demo-machine",
		},
		Hosts: []*systemv1.HostEntry{
			{
				Hostnames: []string{"demo-host", "demo.local"},
			},
			{
				Hostnames: []string{"api-server"},
			},
		},
	}

	// 转换为 anypb.Any
	anySpec, err := anypb.New(hostsSpec)
	if err != nil {
		log.Fatalf("创建 anypb.Any 失败: %v", err)
	}

	// 执行校验
	fmt.Println("正在校验 HostsConfiguration...")
	err = registry.ValidateConfig("HostsConfiguration", anySpec)
	if err != nil {
		fmt.Printf("校验失败: %v\n", err)
	} else {
		fmt.Println("✓ 校验成功!")
	}

	// 5. 演示自定义校验器覆盖
	fmt.Println("\n=== 自定义校验器覆盖演示 ===")

	// 创建一个新的注册表
	newRegistry := validator.NewAutoValidatorRegistry()

	// 注册自定义校验器
	customValidator := &CustomHostsValidator{}
	newRegistry.RegisterOverride("HostsConfiguration", customValidator)

	// 执行自动扫描
	err = newRegistry.AutoScan()
	if err != nil {
		log.Fatalf("自动扫描失败: %v", err)
	}

	// 验证使用的是自定义校验器
	validatorInstance, exists := newRegistry.GetValidator("HostsConfiguration")
	if !exists {
		fmt.Println("未找到 HostsConfiguration 校验器")
	} else if validatorInstance == customValidator {
		fmt.Println("✓ 成功使用自定义校验器")
	} else {
		fmt.Println("✗ 未使用自定义校验器")
	}

	// 6. 性能对比演示
	fmt.Println("\n=== 性能对比演示 ===")
	demonstratePerfomance()

	fmt.Println("\n=== 演示完成 ===")
}

// CustomHostsValidator 自定义 Hosts 校验器示例
type CustomHostsValidator struct{}

func (v *CustomHostsValidator) GetKind() string {
	return "HostsConfiguration"
}

func (v *CustomHostsValidator) Validate(spec *anypb.Any) error {
	var hostsSpec systemv1.HostsConfigurationSpec
	if err := spec.UnmarshalTo(&hostsSpec); err != nil {
		return fmt.Errorf("无法解析 Hosts 配置: %w", err)
	}

	// 自定义校验逻辑：检查主机数量限制
	if len(hostsSpec.Hosts) > 10 {
		return fmt.Errorf("主机条目数量不能超过 10 个，当前: %d", len(hostsSpec.Hosts))
	}

	// 检查 IP 地址不能重复
	ipSet := make(map[string]bool)
	for _, host := range hostsSpec.Hosts {
		if ipSet[host.Ip] {
			return fmt.Errorf("发现重复的 IP 地址: %s", host.Ip)
		}
		ipSet[host.Ip] = true
	}

	fmt.Println("  使用自定义校验逻辑进行校验")
	return nil
}

// demonstratePerfomance 演示不同扫描方法的性能
func demonstratePerfomance() {

	// 测试预定义扫描性能
	start := time.Now()
	for i := 0; i < 100; i++ {
		registry := validator.NewAutoValidatorRegistry()
		_ = registry.AutoScan()
	}
	predifinedDuration := time.Since(start)

	// 测试反射扫描性能
	start = time.Now()
	for i := 0; i < 100; i++ {
		registry := validator.NewAutoValidatorRegistry()
		_ = registry.AutoScanWithReflection()
	}
	reflectionDuration := time.Since(start)

	fmt.Printf("预定义扫描 (100次): %v\n", predifinedDuration)
	fmt.Printf("反射扫描 (100次): %v\n", reflectionDuration)
	fmt.Printf("性能比率: %.2fx\n", float64(reflectionDuration)/float64(predifinedDuration))
}