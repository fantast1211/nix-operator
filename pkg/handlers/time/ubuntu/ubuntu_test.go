package ubuntu

import (
	"context"
	"testing"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"google.golang.org/protobuf/types/known/anypb"
)

// TestUbuntuTimeHandler_Match 测试Ubuntu系统匹配逻辑
func TestUbuntuTimeHandler_Match(t *testing.T) {
	tests := []struct {
		name     string
		osInfo   controller.OSInfo
		expected bool
	}{
		{
			name: "Ubuntu 20.04 should match",
			osInfo: controller.OSInfo{
				ID:         "ubuntu",
				VersionID:  "20.04",
				KernelName: "Linux",
			},
			expected: true,
		},
		{
			name: "Ubuntu 22.04 should match",
			osInfo: controller.OSInfo{
				ID:         "ubuntu",
				VersionID:  "22.04",
				KernelName: "Linux",
			},
			expected: true,
		},
		{
			name: "Ubuntu 24.04 should match",
			osInfo: controller.OSInfo{
				ID:         "ubuntu",
				VersionID:  "24.04",
				KernelName: "Linux",
			},
			expected: true,
		},
		{
			name: "CentOS should not match",
			osInfo: controller.OSInfo{
				ID:        "centos",
				VersionID: "7.9",
			},
			expected: false,
		},
		{
			name: "OpenEuler should not match",
			osInfo: controller.OSInfo{
				ID:        "openeuler",
				VersionID: "22.03",
			},
			expected: false,
		},
		{
			name: "Ubuntu without Linux kernel should not match",
			osInfo: controller.OSInfo{
				ID:         "ubuntu",
				VersionID:  "20.04",
				KernelName: "Darwin",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewUbuntuTimeHandler()
			result := handler.Match(tt.osInfo)
			if result != tt.expected {
				t.Errorf("Match() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestUbuntuTimeHandler_ReconcileWithNilNtp 测试Ubuntu处理器处理nil NTP配置
func TestUbuntuTimeHandler_ReconcileWithNilNtp(t *testing.T) {
	// 创建一个 TimeConfigurationSpec，其中 ntp 为 nil
	timeSpec := &systemv1.TimeConfigurationSpec{
		Timezone: "Asia/Shanghai",
		Ntp:      nil,
	}

	// 将 timeSpec 转换为 Any
	anySpec, err := anypb.New(timeSpec)
	if err != nil {
		t.Fatalf("Failed to create Any from TimeConfigurationSpec: %v", err)
	}

	// 创建 ResourceConfig
	cfg := &systemv1.ResourceConfig{
		Metadata: &systemv1.Metadata{
			Name: "test-ubuntu-time-config",
		},
		Spec: anySpec,
	}

	// 创建 UbuntuTimeHandler
	handler := NewUbuntuTimeHandler()

	// 调用 Reconcile 方法
	_, err = handler.Reconcile(context.Background(), cfg)
	// 我们只关心是否有空指针引用，不关心其他错误
	if err != nil {
		// 检查错误是否是空指针引用
		if err.Error() == "runtime error: invalid memory address or nil pointer dereference" {
			t.Fatalf("Nil pointer dereference occurred: %v", err)
		}
		// 其他错误可能是正常的，比如找不到 chronyd 命令等
		t.Logf("Expected error occurred: %v", err)
	}
}

// TestUbuntuTimeHandler_ReconcileWithEmptyNtp 测试Ubuntu处理器处理空NTP配置
func TestUbuntuTimeHandler_ReconcileWithEmptyNtp(t *testing.T) {
	// 创建一个 TimeConfigurationSpec，其中 ntp 不为 nil，但 Enable 为 false
	timeSpec := &systemv1.TimeConfigurationSpec{
		Timezone: "Asia/Shanghai",
		Ntp:      &systemv1.NtpConfig{Enable: false},
	}

	// 将 timeSpec 转换为 Any
	anySpec, err := anypb.New(timeSpec)
	if err != nil {
		t.Fatalf("Failed to create Any from TimeConfigurationSpec: %v", err)
	}

	// 创建 ResourceConfig
	cfg := &systemv1.ResourceConfig{
		Metadata: &systemv1.Metadata{
			Name: "test-ubuntu-time-config-empty-ntp",
		},
		Spec: anySpec,
	}

	// 创建 UbuntuTimeHandler
	handler := NewUbuntuTimeHandler()

	// 调用 Reconcile 方法
	_, err = handler.Reconcile(context.Background(), cfg)
	// 我们只关心是否有空指针引用，不关心其他错误
	if err != nil {
		// 检查错误是否是空指针引用
		if err.Error() == "runtime error: invalid memory address or nil pointer dereference" {
			t.Fatalf("Nil pointer dereference occurred: %v", err)
		}
		// 其他错误可能是正常的，比如找不到 chronyd 命令等
		t.Logf("Expected error occurred: %v", err)
	}
}