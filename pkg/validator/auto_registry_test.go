package validator

import (
	"testing"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

// TestAutoValidatorRegistry_AutoScan 测试自动扫描功能
func TestAutoValidatorRegistry_AutoScan(t *testing.T) {
	registry := NewAutoValidatorRegistry()

	// 执行自动扫描
	err := registry.AutoScan()
	if err != nil {
		t.Fatalf("AutoScan failed: %v", err)
	}

	// 验证所有预期的配置类型都已注册
	expectedKinds := []string{
		"HostsConfiguration",
		"TimeConfiguration",
		"NetworkConfiguration",
		"BondConfiguration",
	}

	for _, kind := range expectedKinds {
		validator, exists := registry.GetValidator(kind)
		if !exists {
			t.Errorf("Expected validator for kind %s not found", kind)
		}
		if validator == nil {
			t.Errorf("Validator for kind %s is nil", kind)
		}
		if validator.GetKind() != kind {
			t.Errorf("Validator kind mismatch: expected %s, got %s", kind, validator.GetKind())
		}
	}

	// 验证注册的校验器数量
	validators := registry.GetValidators()
	if len(validators) != len(expectedKinds) {
		t.Errorf("Expected %d validators, got %d", len(expectedKinds), len(validators))
	}
}

// TestAutoValidatorRegistry_RegisterOverride 测试覆盖功能
func TestAutoValidatorRegistry_RegisterOverride(t *testing.T) {
	registry := NewAutoValidatorRegistry()

	// 创建一个自定义校验器
	customValidator := &mockValidator{kind: "HostsConfiguration"}

	// 注册覆盖
	registry.RegisterOverride("HostsConfiguration", customValidator)

	// 执行自动扫描
	err := registry.AutoScan()
	if err != nil {
		t.Fatalf("AutoScan failed: %v", err)
	}

	// 验证覆盖的校验器被使用
	validator, exists := registry.GetValidator("HostsConfiguration")
	if !exists {
		t.Fatal("HostsConfiguration validator not found")
	}

	// 验证返回的是自定义校验器
	if validator != customValidator {
		t.Error("Expected custom validator, got default validator")
	}
}

// TestAutoValidatorRegistry_ListRegisteredKinds 测试列出已注册类型功能
func TestAutoValidatorRegistry_ListRegisteredKinds(t *testing.T) {
	registry := NewAutoValidatorRegistry()

	// 执行自动扫描
	err := registry.AutoScan()
	if err != nil {
		t.Fatalf("AutoScan failed: %v", err)
	}

	// 获取已注册的类型列表
	kinds := registry.ListRegisteredKinds()

	// 验证列表不为空
	if len(kinds) == 0 {
		t.Error("No registered kinds found")
	}

	// 验证包含预期的类型
	expectedKinds := map[string]bool{
		"HostsConfiguration":   false,
		"TimeConfiguration":    false,
		"NetworkConfiguration": false,
		"BondConfiguration":    false,
	}

	for _, kind := range kinds {
		if _, exists := expectedKinds[kind]; exists {
			expectedKinds[kind] = true
		}
	}

	// 验证所有预期类型都被找到
	for kind, found := range expectedKinds {
		if !found {
			t.Errorf("Expected kind %s not found in registered kinds", kind)
		}
	}
}

// TestAutoValidatorRegistry_ValidateConfig 测试配置校验功能
func TestAutoValidatorRegistry_ValidateConfig(t *testing.T) {
	registry := NewAutoValidatorRegistry()

	// 执行自动扫描
	err := registry.AutoScan()
	if err != nil {
		t.Fatalf("AutoScan failed: %v", err)
	}

	// 创建一个测试配置
	hostsSpec := &systemv1.HostsConfigurationSpec{
		NodeSelector: &systemv1.NodeSelector{
			MachineId: "test-machine",
		},
		Hosts: []*systemv1.HostEntry{
			{
				Ip:        "192.168.1.100",
				Hostnames: []string{"test-host"},
			},
		},
	}

	// 将配置转换为 anypb.Any
	anySpec, err := anypb.New(hostsSpec)
	if err != nil {
		t.Fatalf("Failed to create anypb.Any: %v", err)
	}

	// 测试校验
	err = registry.ValidateConfig("HostsConfiguration", anySpec)
	if err != nil {
		t.Errorf("Validation failed: %v", err)
	}

	// 测试不存在的配置类型
	err = registry.ValidateConfig("NonExistentConfiguration", anySpec)
	if err == nil {
		t.Error("Expected error for non-existent configuration type")
	}
}

// TestAutoValidatorRegistry_AutoScanWithReflection 测试反射扫描功能
func TestAutoValidatorRegistry_AutoScanWithReflection(t *testing.T) {
	registry := NewAutoValidatorRegistry()

	// 执行反射扫描
	err := registry.AutoScanWithReflection()
	if err != nil {
		t.Fatalf("AutoScanWithReflection failed: %v", err)
	}

	// 验证结果与普通扫描相同
	validators := registry.GetValidators()
	if len(validators) == 0 {
		t.Error("No validators registered after reflection scan")
	}

	// 验证所有预期的配置类型都已注册
	expectedKinds := []string{
		"HostsConfiguration",
		"TimeConfiguration",
		"NetworkConfiguration",
		"BondConfiguration",
	}

	for _, kind := range expectedKinds {
		_, exists := registry.GetValidator(kind)
		if !exists {
			t.Errorf("Expected validator for kind %s not found after reflection scan", kind)
		}
	}
}

// mockValidator 用于测试的模拟校验器
type mockValidator struct {
	kind string
}

func (m *mockValidator) Validate(spec *anypb.Any) error {
	return nil // 模拟校验总是成功
}

func (m *mockValidator) GetKind() string {
	return m.kind
}

// BenchmarkAutoValidatorRegistry_AutoScan 性能测试
func BenchmarkAutoValidatorRegistry_AutoScan(b *testing.B) {
	for i := 0; i < b.N; i++ {
		registry := NewAutoValidatorRegistry()
		err := registry.AutoScan()
		if err != nil {
			b.Fatalf("AutoScan failed: %v", err)
		}
	}
}

// BenchmarkAutoValidatorRegistry_AutoScanWithReflection 反射扫描性能测试
func BenchmarkAutoValidatorRegistry_AutoScanWithReflection(b *testing.B) {
	for i := 0; i < b.N; i++ {
		registry := NewAutoValidatorRegistry()
		err := registry.AutoScanWithReflection()
		if err != nil {
			b.Fatalf("AutoScanWithReflection failed: %v", err)
		}
	}
}
