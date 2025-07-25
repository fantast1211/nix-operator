package time

import (
	"context"
	"testing"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestReconcileWithNilNtp(t *testing.T) {
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
			Name: "test-time-config",
		},
		Spec: anySpec,
	}

	// 创建 LinuxTimeHandler
	handler := &LinuxTimeHandler{}

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

func TestReconcileWithEmptyNtp(t *testing.T) {
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
			Name: "test-time-config-empty-ntp",
		},
		Spec: anySpec,
	}

	// 创建 LinuxTimeHandler
	handler := &LinuxTimeHandler{}

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
