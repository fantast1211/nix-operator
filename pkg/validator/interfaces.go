package validator

import "go.xbrother.com/nix-operator/pkg/interfaces"

// ValidatorRegistry 校验器注册表
type ValidatorRegistry interface {
	// RegisterValidator 注册校验器
	RegisterValidator(validator interfaces.SpecValidator)
	// GetValidator 获取校验器
	GetValidator(kind string) interfaces.SpecValidator
	// ListValidators 列出所有校验器
	ListValidators() []interfaces.SpecValidator
}