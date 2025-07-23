package validator

import (
	"fmt"
	"reflect"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// ProtoValidator 基于 proto 定义的通用校验器
type ProtoValidator struct {
	kind         string
	protoMessage proto.Message
}

// NewProtoValidator 创建 proto 校验器
func NewProtoValidator(kind string, protoMessage proto.Message) SpecValidator {
	return &ProtoValidator{
		kind:         kind,
		protoMessage: protoMessage,
	}
}

// GetKind 获取支持的 kind
func (v *ProtoValidator) GetKind() string {
	return v.kind
}

// Validate 校验 spec 内容是否符合 proto 定义
func (v *ProtoValidator) Validate(spec *anypb.Any) error {
	if spec == nil {
		return fmt.Errorf("spec cannot be nil")
	}

	// 创建目标 proto 消息的新实例
	targetMsg := proto.Clone(v.protoMessage)
	proto.Reset(targetMsg)

	// 直接将 anypb.Any 解包为目标类型
	if err := spec.UnmarshalTo(targetMsg); err != nil {
		return fmt.Errorf("failed to unmarshal Any to %s: %w", v.kind, err)
	}

	// 验证必填字段（可选）
	if err := v.validateRequiredFields(targetMsg); err != nil {
		return fmt.Errorf("required field validation failed: %w", err)
	}

	return nil
}

// validateRequiredFields 验证必填字段
func (v *ProtoValidator) validateRequiredFields(msg proto.Message) error {
	// 验证NodeSelector必填
	if err := v.validateNodeSelector(msg); err != nil {
		return err
	}
	return nil
}

// validateNodeSelector 验证NodeSelector字段必填
func (v *ProtoValidator) validateNodeSelector(msg proto.Message) error {
	// 使用反射检查是否有NodeSelector字段
	reflectValue := reflect.ValueOf(msg)
	if reflectValue.Kind() == reflect.Ptr {
		reflectValue = reflectValue.Elem()
	}
	
	if reflectValue.Kind() != reflect.Struct {
		return nil
	}
	
	// 查找NodeSelector字段
	nodeSelectorField := reflectValue.FieldByName("NodeSelector")
	if !nodeSelectorField.IsValid() {
		return nil // 如果没有NodeSelector字段，跳过验证
	}
	
	// 检查NodeSelector是否为nil
	if nodeSelectorField.IsNil() {
		return fmt.Errorf("nodeSelector is required but not provided")
	}
	
	// 检查NodeSelector的MachineId字段是否为空
	nodeSelector := nodeSelectorField.Elem()
	machineIdField := nodeSelector.FieldByName("MachineId")
	if machineIdField.IsValid() && machineIdField.String() == "" {
		return fmt.Errorf("nodeSelector.machineId is required but empty")
	}
	
	return nil
}
