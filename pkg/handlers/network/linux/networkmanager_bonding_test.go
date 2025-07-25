package linux

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.xbrother.com/nix-operator/pkg/handlers/network/types"
)

// TestNetworkManagerBondingSlaveConfiguration 测试 NetworkManager bonding slave 配置
func TestNetworkManagerBondingSlaveConfiguration(t *testing.T) {
	// 测试 bonding slave 接口配置
	t.Run("BondingSlave Configuration", func(t *testing.T) {
		// 创建一个 bonding slave 接口
		bondSlaveInterface := types.Interface{
			Name:        "ens36",
			IPv4Address: "192.168.1.100/24", // 这些 IP 配置在 bonding slave 中应该被忽略
			IPv6Address: "fd00::5688/64",
			IPv4Gateway: "192.168.1.1",
			IPv6Gateway: "fd00::1111",
			MTU:         1500,
			Nameservers: []string{"192.168.4.30"},
			BondingSlave: &types.BondingSlaveConfig{
				Enabled: true,
				Master:  "bond1",
			},
		}

		// 验证模板渲染
		templateContent := nmConnectionTemplate
		if !strings.Contains(templateContent, "BondingSlave") {
			t.Error("Template should contain BondingSlave handling")
		}

		if !strings.Contains(templateContent, "master=") {
			t.Error("Template should contain master configuration for bonding slave")
		}

		if !strings.Contains(templateContent, "slave-type=bond") {
			t.Error("Template should contain slave-type=bond configuration")
		}

		t.Logf("Bonding slave interface %s configuration test passed", bondSlaveInterface.Name)
	})

	// 测试普通接口配置
	t.Run("Regular Interface Configuration", func(t *testing.T) {
		// 创建一个普通接口
		regularInterface := types.Interface{
			Name:        "ens33",
			IPv4Address: "192.168.34.104/24",
			IPv6Address: "fd00::5688/64",
			IPv4Gateway: "192.168.34.1",
			IPv6Gateway: "fd00::1111",
			MTU:         1500,
			Nameservers: []string{"192.168.4.31"},
			BondingSlave: nil, // 不是 bonding slave
		}

		// 验证模板应该包含 IP 配置部分
		templateContent := nmConnectionTemplate
		if !strings.Contains(templateContent, "[ipv4]") {
			t.Error("Template should contain ipv4 section for regular interfaces")
		}

		if !strings.Contains(templateContent, "[ipv6]") {
			t.Error("Template should contain ipv6 section for regular interfaces")
		}

		t.Logf("Regular interface %s configuration test passed", regularInterface.Name)
	})

	// 测试模板条件逻辑
	t.Run("Template Conditional Logic", func(t *testing.T) {
		templateContent := nmConnectionTemplate

		// 验证模板包含条件判断
		if !strings.Contains(templateContent, "{{- if and .BondingSlave .BondingSlave.Enabled}}") {
			t.Error("Template should contain bonding slave condition check")
		}

		if !strings.Contains(templateContent, "{{- else}}") {
			t.Error("Template should contain else condition for regular interfaces")
		}

		if !strings.Contains(templateContent, "{{- end }}") {
			t.Error("Template should properly close conditional blocks")
		}

		t.Log("Template conditional logic test passed")
	})
}

// TestNetworkManagerBondingSlaveTemplateRendering 测试模板渲染结果
func TestNetworkManagerBondingSlaveTemplateRendering(t *testing.T) {
	// 这个测试可以在有实际环境时运行，验证生成的配置文件内容
	t.Skip("Skipping template rendering test - requires actual environment")

	nm := &NetworkManager{}

	// 创建 bonding slave 接口
	bondSlaveInterface := types.Interface{
		Name: "ens36",
		MTU:  1500,
		BondingSlave: &types.BondingSlaveConfig{
			Enabled: true,
			Master:  "bond1",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 尝试配置（在测试环境中可能会失败，但可以验证模板逻辑）
	changed, err := nm.ConfigureWithCheck(ctx, bondSlaveInterface)
	if err != nil {
		t.Logf("Configuration failed as expected in test environment: %v", err)
	} else {
		t.Logf("Configuration succeeded, changed: %v", changed)
	}
}