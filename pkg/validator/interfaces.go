package validator

import (
	"google.golang.org/protobuf/types/known/anypb"
)

// SpecValidator Spec 校验器接口（插件机制）
type SpecValidator interface {
	// Validate 校验 spec 内容
	Validate(spec *anypb.Any) error
	// GetKind 获取支持的 kind
	GetKind() string
}