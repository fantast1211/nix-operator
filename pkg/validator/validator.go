package validator

import (
	"fmt"

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
	// 这里可以添加必填字段的校验逻辑
	// 例如检查字符串字段是否为空等
	return nil
}
