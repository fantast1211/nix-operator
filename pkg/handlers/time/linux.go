package time

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/utils"
)

type TimeSpec struct {
	Timezone string    `json:"timezone" yaml:"timezone"`
	NTP      NTPConfig `json:"ntp" yaml:"ntp"`
}

type NTPConfig struct {
	Enable  bool     `json:"enable" yaml:"enable"`
	Servers []string `json:"servers" yaml:"servers"`
}

//go:embed chrony.conf.tpl
var chronyConfigTemplate string

func init() {
	controller.RegisterHandler("TimeConfiguration", &LinuxTimeHandler{})
}

type LinuxTimeHandler struct{}

func (h *LinuxTimeHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

type chronyConfig struct {
	Servers []string
}

func (h *LinuxTimeHandler) Reconcile(ctx context.Context, cfg *config.ResourceConfig) (*controller.ReconcileResult, error) {
	// 解析时间配置
	var timeSpec TimeSpec

	// 将Spec转换为时间配置
	specBytes, err := json.Marshal(cfg.Spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal spec: %v", err)
	}

	if err := json.Unmarshal(specBytes, &timeSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal time spec: %v", err)
	}

	// 设置时区
	if timeSpec.Timezone != "" {
		if err := h.setTimezone(ctx, timeSpec.Timezone); err != nil {
			return nil, fmt.Errorf("failed to set timezone: %v", err)
		}
	}

	// 配置NTP
	if !timeSpec.NTP.Enable {
		return &controller.ReconcileResult{
			Status: &config.ResourceStatus{
				Phase:   "Ready",
				Reason:  "NTPDisabled",
				Message: "NTP is disabled, only timezone was configured",
			},
		}, nil
	}

	// 准备模板数据
	templateData := chronyConfig{
		Servers: timeSpec.NTP.Servers,
	}

	// 解析模板
	tmpl, err := template.New("chrony").Parse(chronyConfigTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %v", err)
	}

	// 渲染配置
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return nil, fmt.Errorf("failed to execute template: %v", err)
	}

	desiredContent := content.String()

	// 读取现有配置
	currentContent, err := os.ReadFile("/etc/chrony/chrony.conf")
	if err == nil {
		// 配置文件存在，比较内容
		if string(currentContent) == desiredContent {
			return &controller.ReconcileResult{
				Status: &config.ResourceStatus{
					Phase:   "Ready",
					Reason:  "NoChange",
					Message: "Configuration is up to date",
				},
			}, nil // 配置相同，无需更新
		}
	}
	// 如果文件不存在或读取失败，继续写入新配置

	// 使用工具函数原子性写入文件
	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/chrony/chrony.conf", 0644); err != nil {
		return nil, fmt.Errorf("failed to write chrony.conf: %v", err)
	}

	// 重新加载配置（不需要 root 权限）
	cmd := exec.CommandContext(ctx, "chronyc", "reload", "sources")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to reload chronyd config: %v, output: %s", err, output)
	}

	return &controller.ReconcileResult{
		Status: &config.ResourceStatus{
			Phase:   "Ready",
			Reason:  "Configured",
			Message: "Time configuration applied successfully",
		},
	}, nil
}

func (h *LinuxTimeHandler) setTimezone(ctx context.Context, timezone string) error {
	// 使用timedatectl设置时区
	cmd := exec.CommandContext(ctx, "timedatectl", "set-timezone", timezone)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set timezone: %v, output: %s", err, output)
	}
	return nil
}
