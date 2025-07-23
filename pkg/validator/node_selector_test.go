package validator

import (
	"testing"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// TestProtoValidator_ValidateNodeSelector 测试NodeSelector验证功能
func TestProtoValidator_ValidateNodeSelector(t *testing.T) {
	tests := []struct {
		name        string
		spec        interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid hosts config with nodeSelector",
			spec: &systemv1.HostsConfigurationSpec{
				NodeSelector: &systemv1.NodeSelector{
					MachineId: "test-machine-123",
				},
				Hostname: "test-host",
				Hosts: []*systemv1.HostEntry{
					{
						Ip:        "192.168.1.10",
						Hostnames: []string{"server1"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "hosts config without nodeSelector",
			spec: &systemv1.HostsConfigurationSpec{
				Hostname: "test-host",
				Hosts: []*systemv1.HostEntry{
					{
						Ip:        "192.168.1.10",
						Hostnames: []string{"server1"},
					},
				},
			},
			expectError: true,
			errorMsg:    "required field validation failed: nodeSelector is required but not provided",
		},
		{
			name: "hosts config with empty machineId",
			spec: &systemv1.HostsConfigurationSpec{
				NodeSelector: &systemv1.NodeSelector{
					MachineId: "",
				},
				Hostname: "test-host",
			},
			expectError: true,
			errorMsg:    "required field validation failed: nodeSelector.machineId is required but empty",
		},
		{
			name: "valid network config with nodeSelector",
			spec: &systemv1.NetworkConfigurationSpec{
				NodeSelector: &systemv1.NodeSelector{
					MachineId: "test-machine-456",
				},
				Interfaces: []*systemv1.NetworkInterface{
					{
						Name:        "eth0",
						Ipv4Address: "192.168.1.100/24",
					},
				},
			},
			expectError: false,
		},
		{
			name: "network config without nodeSelector",
			spec: &systemv1.NetworkConfigurationSpec{
				Interfaces: []*systemv1.NetworkInterface{
					{
						Name:        "eth0",
						Ipv4Address: "192.168.1.100/24",
					},
				},
			},
			expectError: true,
			errorMsg:    "required field validation failed: nodeSelector is required but not provided",
		},

	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建对应的validator
			var validator SpecValidator
			switch spec := tt.spec.(type) {
			case *systemv1.HostsConfigurationSpec:
				validator = NewProtoValidator("HostsConfiguration", &systemv1.HostsConfigurationSpec{})
			case *systemv1.NetworkConfigurationSpec:
				validator = NewProtoValidator("NetworkConfiguration", &systemv1.NetworkConfigurationSpec{})
			case *systemv1.TimeConfigurationSpec:
				validator = NewProtoValidator("TimeConfiguration", &systemv1.TimeConfigurationSpec{})
			default:
				t.Fatalf("Unsupported spec type: %T", spec)
			}

			// 转换为anypb.Any
			anySpec, err := anypb.New(tt.spec.(proto.Message))
			if err != nil {
				t.Fatalf("Failed to create anypb.Any: %v", err)
			}

			// 执行验证
			err = validator.Validate(anySpec)

			// 检查结果
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Expected error message '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestAutoValidatorRegistry_NodeSelectorValidation 测试自动注册的validator是否包含NodeSelector验证
func TestAutoValidatorRegistry_NodeSelectorValidation(t *testing.T) {
	registry := NewAutoValidatorRegistry()

	// 执行自动扫描
	err := registry.AutoScan()
	if err != nil {
		t.Fatalf("AutoScan failed: %v", err)
	}

	// 测试HostsConfiguration validator
	validator, exists := registry.GetValidator("HostsConfiguration")
	if !exists {
		t.Fatal("HostsConfiguration validator not found")
	}

	// 创建一个没有NodeSelector的配置
	hostsSpec := &systemv1.HostsConfigurationSpec{
		Hostname: "test-host",
		Hosts: []*systemv1.HostEntry{
			{
				Ip:        "192.168.1.10",
				Hostnames: []string{"server1"},
			},
		},
	}

	// 转换为anypb.Any
	anySpec, err := anypb.New(hostsSpec)
	if err != nil {
		t.Fatalf("Failed to create anypb.Any: %v", err)
	}

	// 验证应该失败
	err = validator.Validate(anySpec)
	if err == nil {
		t.Error("Expected validation to fail for missing nodeSelector")
	} else if err.Error() != "required field validation failed: nodeSelector is required but not provided" {
		t.Errorf("Unexpected error message: %s", err.Error())
	}
}