package registry

import (
	"fmt"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/interfaces"
	"go.xbrother.com/nix-operator/pkg/utils"
	"go.xbrother.com/nix-operator/pkg/validator"
)

// ComponentRegistry 组件注册表
type ComponentRegistry struct {
	handlers           map[string]controller.Handler
	validatorRegistry  validator.ValidatorRegistry
	osInfo             domain.OSInfo
	logger             *utils.Logger
}

// NewComponentRegistry 创建组件注册表
func NewComponentRegistry(osInfo domain.OSInfo, logger *utils.Logger) *ComponentRegistry {
	return &ComponentRegistry{
		handlers:          make(map[string]controller.Handler),
		validatorRegistry: validator.NewValidatorRegistry(),
		osInfo:            osInfo,
		logger:            logger,
	}
}

// RegisterHandler 注册配置处理器
func (r *ComponentRegistry) RegisterHandler(kind string, handler controller.Handler) error {
	if kind == "" {
		return fmt.Errorf("handler kind cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	// 检查处理器是否支持当前操作系统
	if !handler.Match(controller.OSInfo{
		ID:         r.osInfo.ID,
		VersionID:  r.osInfo.VersionID,
		KernelName: r.osInfo.KernelName,
		KernelVer:  r.osInfo.KernelVer,
	}) {
		return fmt.Errorf("handler for kind %s does not support current OS: %s %s", 
			kind, r.osInfo.ID, r.osInfo.VersionID)
	}

	r.handlers[kind] = handler
	r.logger.Infof("registry", "Registered handler for kind: %s", kind)
	return nil
}

// RegisterValidator 注册类型校验器
func (r *ComponentRegistry) RegisterValidator(validator interfaces.SpecValidator) error {
	if validator == nil {
		return fmt.Errorf("validator cannot be nil")
	}

	r.validatorRegistry.RegisterValidator(validator)
	r.logger.Infof("registry", "Registered validator for kind: %s", validator.GetKind())
	return nil
}

// GetHandlers 获取所有处理器
func (r *ComponentRegistry) GetHandlers() map[string]controller.Handler {
	return r.handlers
}

// GetValidatorRegistry 获取校验器注册表
func (r *ComponentRegistry) GetValidatorRegistry() validator.ValidatorRegistry {
	return r.validatorRegistry
}

// GetHandler 获取指定类型的处理器
func (r *ComponentRegistry) GetHandler(kind string) (controller.Handler, bool) {
	handler, exists := r.handlers[kind]
	return handler, exists
}

// GetValidator 获取指定类型的校验器
func (r *ComponentRegistry) GetValidator(kind string) interfaces.SpecValidator {
	return r.validatorRegistry.GetValidator(kind)
}

// ListRegisteredKinds 列出所有已注册的类型
func (r *ComponentRegistry) ListRegisteredKinds() []string {
	kinds := make([]string, 0, len(r.handlers))
	for kind := range r.handlers {
		kinds = append(kinds, kind)
	}
	return kinds
}

// ValidateRegistration 验证注册的完整性
func (r *ComponentRegistry) ValidateRegistration() error {
	// 检查每个处理器是否都有对应的校验器
	for kind := range r.handlers {
		if r.validatorRegistry.GetValidator(kind) == nil {
			return fmt.Errorf("no validator registered for handler kind: %s", kind)
		}
	}

	// 检查每个校验器是否都有对应的处理器
	for _, validator := range r.validatorRegistry.ListValidators() {
		if _, exists := r.handlers[validator.GetKind()]; !exists {
			return fmt.Errorf("no handler registered for validator kind: %s", validator.GetKind())
		}
	}

	r.logger.Info("registry", "Component registration validation passed")
	return nil
}

// GetOSInfo 获取操作系统信息
func (r *ComponentRegistry) GetOSInfo() domain.OSInfo {
	return r.osInfo
}