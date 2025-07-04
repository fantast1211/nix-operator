package validator

import (
	"encoding/json"
	"fmt"

	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/interfaces"
)

// validatorRegistry 校验器注册表实现
type validatorRegistry struct {
	validators map[string]interfaces.SpecValidator
}

// NewValidatorRegistry 创建新的校验器注册表
func NewValidatorRegistry() ValidatorRegistry {
	return &validatorRegistry{
		validators: make(map[string]interfaces.SpecValidator),
	}
}

func (r *validatorRegistry) RegisterValidator(validator interfaces.SpecValidator) {
	r.validators[validator.GetKind()] = validator
}

func (r *validatorRegistry) GetValidator(kind string) interfaces.SpecValidator {
	return r.validators[kind]
}

func (r *validatorRegistry) ListValidators() []interfaces.SpecValidator {
	validators := make([]interfaces.SpecValidator, 0, len(r.validators))
	for _, v := range r.validators {
		validators = append(validators, v)
	}
	return validators
}

// HostsConfigurationValidator hosts配置校验器
type HostsConfigurationValidator struct{}

func (v *HostsConfigurationValidator) Validate(spec []byte) error {
	var hostsSpec domain.HostsConfigurationSpec
	if err := json.Unmarshal(spec, &hostsSpec); err != nil {
		return fmt.Errorf("invalid hosts configuration spec: %v", err)
	}

	// 校验hosts配置
	for i, host := range hostsSpec.Hosts {
		if host.IP == "" {
			return fmt.Errorf("hosts[%d]: IP address is required", i)
		}
		if len(host.Hostnames) == 0 {
			return fmt.Errorf("hosts[%d]: at least one hostname is required", i)
		}
		for j, hostname := range host.Hostnames {
			if hostname == "" {
				return fmt.Errorf("hosts[%d].hostnames[%d]: hostname cannot be empty", i, j)
			}
		}
	}

	return nil
}

func (v *HostsConfigurationValidator) GetKind() string {
	return "HostsConfiguration"
}

// TimeConfigurationValidator 时间配置校验器
type TimeConfigurationValidator struct{}

func (v *TimeConfigurationValidator) Validate(spec []byte) error {
	var timeSpec domain.TimeConfigurationSpec
	if err := json.Unmarshal(spec, &timeSpec); err != nil {
		return fmt.Errorf("invalid time configuration spec: %v", err)
	}

	// 校验时间配置
	if timeSpec.Timezone == "" {
		return fmt.Errorf("timezone is required")
	}

	// 校验NTP服务器格式（简单校验）
	for i, server := range timeSpec.Servers {
		if server == "" {
			return fmt.Errorf("servers[%d]: server address cannot be empty", i)
		}
	}

	return nil
}

func (v *TimeConfigurationValidator) GetKind() string {
	return "TimeConfiguration"
}

// 默认校验器注册
func RegisterDefaultValidators(registry ValidatorRegistry) {
	registry.RegisterValidator(&HostsConfigurationValidator{})
	registry.RegisterValidator(&TimeConfigurationValidator{})
}