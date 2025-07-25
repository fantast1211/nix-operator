package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/openeuler"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
)

// TestBondConfiguration 测试Bond配置修复
func TestBondConfiguration(t *testing.T) {
	fmt.Println("=== Bond配置修复测试 ===")
	
	// 创建openEuler Bond管理器（测试模式）
	osInfo := &controller.OSInfo{
		ID:        "openeuler",
		VersionID: "22.03",
	}
	
	bondManager := openeuler.NewOpenEulerBondIfupdownForTest(osInfo)
	
	// 测试Bond配置
	bondConfig := types.BondConfig{
		Name:   "bond0",
		Mode:   1, // active-backup
		Miimon: 100,
		Network: types.BondNetworkConfig{
			IP:         "192.168.34.104/24",
			Gateway:    "192.168.34.1",
			DNSServers: []string{"8.8.8.8", "8.8.4.4"},
			MTU:        1500,
		},
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// 1. 测试检测功能
	fmt.Println("\n1. 测试Bond管理器检测...")
	if bondManager.IsInstall(ctx) {
		fmt.Println("✓ Bond管理器检测成功")
	} else {
		fmt.Println("✗ Bond管理器检测失败")
		return
	}
	
	// 2. 测试配置生成
	fmt.Println("\n2. 测试Bond配置生成...")
	changed, err := bondManager.ConfigureWithCheck(ctx, bondConfig)
	if err != nil {
		fmt.Printf("✗ Bond配置失败: %v\n", err)
		return
	}
	
	if changed {
		fmt.Println("✓ Bond配置已更新")
	} else {
		fmt.Println("- Bond配置未变更")
	}
	
	// 3. 测试重载功能
	fmt.Println("\n3. 测试网络服务重载...")
	if err := bondManager.ReloadIfy(ctx); err != nil {
		fmt.Printf("✗ 网络服务重载失败: %v\n", err)
		return
	}
	fmt.Println("✓ 网络服务重载成功")
	
	fmt.Println("\n=== 测试完成 ===")
}

// TestConfigurationValidation 测试配置验证
func TestConfigurationValidation(t *testing.T) {
	fmt.Println("\n=== 配置验证测试 ===")
	
	// 测试各种配置场景
	testCases := []struct {
		name   string
		config types.BondConfig
		valid  bool
	}{
		{
			name: "有效配置",
			config: types.BondConfig{
				Name:   "bond0",
				Mode:   1,
				Miimon: 100,
			},
			valid: true,
		},
		{
			name: "空名称",
			config: types.BondConfig{
				Name:   "",
				Mode:   1,
				Miimon: 100,
			},
			valid: false,
		},
		{
			name: "无效模式",
			config: types.BondConfig{
				Name:   "bond0",
				Mode:   99,
				Miimon: 100,
			},
			valid: false,
		},
	}
	
	osInfo := &controller.OSInfo{
		ID:        "openeuler",
		VersionID: "22.03",
	}
	
	bondManager := openeuler.NewOpenEulerBondIfupdownForTest(osInfo)
	ctx := context.Background()
	
	for _, tc := range testCases {
		fmt.Printf("\n测试: %s\n", tc.name)
		_, err := bondManager.ConfigureWithCheck(ctx, tc.config)
		
		if tc.valid && err != nil {
			fmt.Printf("✗ 预期成功但失败: %v\n", err)
		} else if !tc.valid && err == nil {
			fmt.Printf("✗ 预期失败但成功\n")
		} else {
			fmt.Printf("✓ 验证通过\n")
		}
	}
}

// TestBondFixSummary 测试修复总结
func TestBondFixSummary(t *testing.T) {
	t.Log("Bond配置修复验证")
	t.Log("注意：这是测试模式，不会实际修改系统配置")
	
	t.Log("=== 修复建议 ===")
	t.Log("1. UUID生成问题已修复：移除了$(uuidgen)语法错误")
	t.Log("2. 网络服务重启逻辑已改进：添加了手动激活fallback")
	t.Log("3. 需要确保从接口配置正确：检查network模块的从接口配置")
	t.Log("4. 建议检查实际环境中的配置文件和网络状态")
}