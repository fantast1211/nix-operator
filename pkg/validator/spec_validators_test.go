package validator

import (
	"testing"
)

// TestHostsConfigurationValidator 测试hosts配置校验器
func TestHostsConfigurationValidator(t *testing.T) {
	validator := &HostsConfigurationValidator{}

	// 测试有效配置
	validSpec := `{
		"hosts": [
			{
				"ip": "127.0.0.1",
				"hostnames": ["localhost", "local"]
			},
			{
				"ip": "192.168.1.100",
				"hostnames": ["server1"]
			}
		]
	}`

	if err := validator.Validate([]byte(validSpec)); err != nil {
		t.Errorf("Expected valid spec to pass validation, got error: %v", err)
	}

	// 测试无效配置 - 缺少IP
	invalidSpec1 := `{
		"hosts": [
			{
				"ip": "",
				"hostnames": ["localhost"]
			}
		]
	}`

	if err := validator.Validate([]byte(invalidSpec1)); err == nil {
		t.Error("Expected validation to fail for empty IP, but it passed")
	}

	// 测试无效配置 - 缺少hostnames
	invalidSpec2 := `{
		"hosts": [
			{
				"ip": "127.0.0.1",
				"hostnames": []
			}
		]
	}`

	if err := validator.Validate([]byte(invalidSpec2)); err == nil {
		t.Error("Expected validation to fail for empty hostnames, but it passed")
	}

	// 测试无效配置 - 空hostname
	invalidSpec3 := `{
		"hosts": [
			{
				"ip": "127.0.0.1",
				"hostnames": ["localhost", ""]
			}
		]
	}`

	if err := validator.Validate([]byte(invalidSpec3)); err == nil {
		t.Error("Expected validation to fail for empty hostname, but it passed")
	}

	// 测试GetKind方法
	if kind := validator.GetKind(); kind != "HostsConfiguration" {
		t.Errorf("Expected kind 'HostsConfiguration', got '%s'", kind)
	}
}

// TestTimeConfigurationValidator 测试时间配置校验器
func TestTimeConfigurationValidator(t *testing.T) {
	validator := &TimeConfigurationValidator{}

	// 测试有效配置
	validSpec := `{
		"timezone": "Asia/Shanghai",
		"servers": ["ntp1.aliyun.com", "ntp2.aliyun.com"]
	}`

	if err := validator.Validate([]byte(validSpec)); err != nil {
		t.Errorf("Expected valid spec to pass validation, got error: %v", err)
	}

	// 测试有效配置 - 无NTP服务器
	validSpec2 := `{
		"timezone": "UTC"
	}`

	if err := validator.Validate([]byte(validSpec2)); err != nil {
		t.Errorf("Expected valid spec without servers to pass validation, got error: %v", err)
	}

	// 测试无效配置 - 缺少timezone
	invalidSpec1 := `{
		"timezone": "",
		"servers": ["ntp1.aliyun.com"]
	}`

	if err := validator.Validate([]byte(invalidSpec1)); err == nil {
		t.Error("Expected validation to fail for empty timezone, but it passed")
	}

	// 测试无效配置 - 空服务器地址
	invalidSpec2 := `{
		"timezone": "UTC",
		"servers": ["ntp1.aliyun.com", ""]
	}`

	if err := validator.Validate([]byte(invalidSpec2)); err == nil {
		t.Error("Expected validation to fail for empty server address, but it passed")
	}

	// 测试GetKind方法
	if kind := validator.GetKind(); kind != "TimeConfiguration" {
		t.Errorf("Expected kind 'TimeConfiguration', got '%s'", kind)
	}
}

// TestValidatorRegistry 测试校验器注册表
func TestValidatorRegistry(t *testing.T) {
	registry := NewValidatorRegistry()

	// 注册校验器
	hostsValidator := &HostsConfigurationValidator{}
	timeValidator := &TimeConfigurationValidator{}

	registry.RegisterValidator(hostsValidator)
	registry.RegisterValidator(timeValidator)

	// 测试获取校验器
	if validator := registry.GetValidator("HostsConfiguration"); validator == nil {
		t.Error("Expected to find HostsConfiguration validator")
	} else if validator.GetKind() != "HostsConfiguration" {
		t.Errorf("Expected HostsConfiguration validator, got %s", validator.GetKind())
	}

	if validator := registry.GetValidator("TimeConfiguration"); validator == nil {
		t.Error("Expected to find TimeConfiguration validator")
	} else if validator.GetKind() != "TimeConfiguration" {
		t.Errorf("Expected TimeConfiguration validator, got %s", validator.GetKind())
	}

	// 测试不存在的校验器
	if validator := registry.GetValidator("NonExistentConfiguration"); validator != nil {
		t.Error("Expected nil for non-existent validator")
	}

	// 测试列出所有校验器
	validators := registry.ListValidators()
	if len(validators) != 2 {
		t.Errorf("Expected 2 validators, got %d", len(validators))
	}

	// 验证校验器类型
	kinds := make(map[string]bool)
	for _, v := range validators {
		kinds[v.GetKind()] = true
	}

	if !kinds["HostsConfiguration"] {
		t.Error("Expected HostsConfiguration validator in list")
	}

	if !kinds["TimeConfiguration"] {
		t.Error("Expected TimeConfiguration validator in list")
	}
}

// TestRegisterDefaultValidators 测试默认校验器注册
func TestRegisterDefaultValidators(t *testing.T) {
	registry := NewValidatorRegistry()

	// 注册默认校验器
	RegisterDefaultValidators(registry)

	// 验证所有默认校验器都已注册
	expectedKinds := []string{"HostsConfiguration", "TimeConfiguration"}

	for _, kind := range expectedKinds {
		if validator := registry.GetValidator(kind); validator == nil {
			t.Errorf("Expected default validator for kind %s to be registered", kind)
		}
	}

	// 验证校验器数量
	validators := registry.ListValidators()
	if len(validators) != len(expectedKinds) {
		t.Errorf("Expected %d default validators, got %d", len(expectedKinds), len(validators))
	}
}