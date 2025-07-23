package validator

import (
	"fmt"
	"reflect"
	"strings"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// AutoValidatorRegistry 自动校验器注册表
type AutoValidatorRegistry struct {
	validators map[string]SpecValidator
	overrides  map[string]SpecValidator // 用于存储特殊校验器覆盖
}

// NewAutoValidatorRegistry 创建自动校验器注册表
func NewAutoValidatorRegistry() *AutoValidatorRegistry {
	return &AutoValidatorRegistry{
		validators: make(map[string]SpecValidator),
		overrides:  make(map[string]SpecValidator),
	}
}

// RegisterOverride 注册特殊校验器覆盖
// 用于某些配置类型需要特殊校验逻辑的情况
func (r *AutoValidatorRegistry) RegisterOverride(kind string, validator SpecValidator) {
	r.overrides[kind] = validator
}

// AutoScan 自动扫描并注册所有符合条件的校验器
func (r *AutoValidatorRegistry) AutoScan() error {
	// 定义所有需要扫描的配置类型
	// 这些是从 proto 生成的类型，以 ConfigurationSpec 结尾
	configTypes := []struct {
		name     string
		instance proto.Message
	}{
		{"HostsConfiguration", &systemv1.HostsConfigurationSpec{}},
		{"TimeConfiguration", &systemv1.TimeConfigurationSpec{}},
		{"NetworkConfiguration", &systemv1.NetworkConfigurationSpec{}},
		{"BondConfiguration", &systemv1.BondConfigurationSpec{}},
	}

	// 自动注册每个配置类型的校验器
	for _, configType := range configTypes {
		kind := configType.name

		// 检查是否有覆盖的特殊校验器
		if override, exists := r.overrides[kind]; exists {
			r.validators[kind] = override
			continue
		}

		// 创建默认的 ProtoValidator
		validator := NewProtoValidator(kind, configType.instance)
		r.validators[kind] = validator
	}

	return nil
}

// AutoScanWithReflection 使用反射自动扫描 systemv1 包中的所有 ConfigurationSpec 类型
// 这是一个更高级的版本，可以自动发现新增的类型
func (r *AutoValidatorRegistry) AutoScanWithReflection() error {
	// 通过反射扫描 systemv1 包中的所有类型
	configTypes := r.scanConfigurationSpecs()

	// 自动注册每个配置类型的校验器
	for kind, protoType := range configTypes {
		// 检查是否有覆盖的特殊校验器
		if override, exists := r.overrides[kind]; exists {
			r.validators[kind] = override
			continue
		}

		// 创建 proto 消息实例
		instance := reflect.New(protoType.Elem()).Interface().(proto.Message)

		// 创建默认的 ProtoValidator
		validator := NewProtoValidator(kind, instance)
		r.validators[kind] = validator
	}

	return nil
}

// scanConfigurationSpecs 扫描所有以 ConfigurationSpec 结尾的类型
func (r *AutoValidatorRegistry) scanConfigurationSpecs() map[string]reflect.Type {
	configTypes := make(map[string]reflect.Type)

	// 定义已知的配置类型映射
	// 这里我们使用静态映射，因为 Go 的反射无法直接扫描包中的所有类型
	typeMap := map[string]reflect.Type{
		"HostsConfiguration":   reflect.TypeOf((*systemv1.HostsConfigurationSpec)(nil)),
		"TimeConfiguration":    reflect.TypeOf((*systemv1.TimeConfigurationSpec)(nil)),
		"NetworkConfiguration": reflect.TypeOf((*systemv1.NetworkConfigurationSpec)(nil)),
		"BondConfiguration":    reflect.TypeOf((*systemv1.BondConfigurationSpec)(nil)),
	}

	// 验证类型名称是否符合 ConfigurationSpec 命名规则
	for kind, protoType := range typeMap {
		typeName := protoType.Elem().Name()
		if strings.HasSuffix(typeName, "ConfigurationSpec") {
			configTypes[kind] = protoType
		}
	}

	return configTypes
}

// GetValidators 获取所有注册的校验器
func (r *AutoValidatorRegistry) GetValidators() map[string]SpecValidator {
	return r.validators
}

// GetValidator 根据 kind 获取特定的校验器
func (r *AutoValidatorRegistry) GetValidator(kind string) (SpecValidator, bool) {
	validator, exists := r.validators[kind]
	return validator, exists
}

// ListRegisteredKinds 列出所有已注册的配置类型
func (r *AutoValidatorRegistry) ListRegisteredKinds() []string {
	kinds := make([]string, 0, len(r.validators))
	for kind := range r.validators {
		kinds = append(kinds, kind)
	}
	return kinds
}

// ValidateConfig 校验指定类型的配置
func (r *AutoValidatorRegistry) ValidateConfig(kind string, spec interface{}) error {
	validator, exists := r.GetValidator(kind)
	if !exists {
		return fmt.Errorf("no validator found for kind: %s", kind)
	}

	// 这里需要根据实际的校验接口进行调用
	// 假设 spec 是 *anypb.Any 类型
	if anySpec, ok := spec.(*anypb.Any); ok {
		return validator.Validate(anySpec)
	}

	return fmt.Errorf("unsupported spec type for validation")
}
