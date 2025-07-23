package time

import (
	context "context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed chrony.conf.tpl
var chronyConfigTemplate string

func init() {
	controller.RegisterHandler("TimeConfiguration", &LinuxTimeHandler{})
}

type LinuxTimeHandler struct{
}

func (h *LinuxTimeHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

func (h *LinuxTimeHandler) Reconcile(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*status.ReconcileResult, error) {
	// time类型配置只会存在一个，因为这个配置可以各节点一致
	if len(configs) == 0 {
		return nil, fmt.Errorf("no time configuration found")
	}

	// 使用第一个配置，如果有多个则记录警告
	cfg := configs[0]
	if len(configs) > 1 {
		utils.Warnf("time", "Multiple time configurations found (%d), using the first one: %s", len(configs), cfg.Metadata.Name)
	}

	// 解析时间配置
	var timeSpec *systemv1.TimeConfigurationSpec
	utils.Debugf("time", "Parsed configs: configs=%s, ", configs)

	// 从 anypb.Any 中解析 TimeConfigurationSpec
	if cfg.Spec != nil {
		var err error
		timeSpec, err = utils.UnmarshalSpec[*systemv1.TimeConfigurationSpec](cfg.Spec)
		if err != nil {
			return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "DecodeSpecFailed",
				Message: fmt.Sprintf("decode spec failed: %v", err),
			},
			Error: err,
		}}, nil
		}
	} else {
		err := fmt.Errorf("spec is nil")
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "SpecIsNil",
				Message: "spec is nil",
			},
			Error: err,
		}}, nil
	}

	// 安全地打印 NTP 配置
	ntpEnable := false
	var ntpServers []string
	if timeSpec.Ntp != nil {
		ntpEnable = timeSpec.Ntp.Enable
		ntpServers = timeSpec.Ntp.Servers
	}
	utils.Debugf("time", "Parsed timeSpec: timezone=%s, ntp.enable=%v, ntp.servers=%v",
		timeSpec.Timezone, ntpEnable, ntpServers)

	// 设置时区
	if timeSpec.Timezone != "" {
		if err := h.setTimezone(ctx, timeSpec.Timezone); err != nil {
			return []*status.ReconcileResult{{
				Config: cfg,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  "SetTimezoneFailed",
					Message: fmt.Sprintf("failed to set timezone: %v", err),
				},
				Error: err,
			}}, nil
		}
	}

	// 配置NTP
	if timeSpec.Ntp == nil || !timeSpec.Ntp.Enable {
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  "NTPDisabled",
				Message: "NTP is disabled, only timezone was configured",
			},
		}}, nil
	}

	// 检查 chronyd 是否存在
	if _, err := exec.LookPath("chronyd"); err != nil {
		err := fmt.Errorf("chronyd not found in system, please install chrony before applying NTP configuration")
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "ChronydNotInstalled",
				Message: "chronyd not found in system, please install chrony before applying NTP configuration",
			},
			Error: err,
		}}, nil
	}

	// 检查 chronyc 是否存在
	if _, err := exec.LookPath("chronyc"); err != nil {
		err := fmt.Errorf("chronyc command not found, cannot reload chrony configuration")
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "ChronycNotInstalled",
				Message: "chronyc command not found, cannot reload chrony configuration",
			},
			Error: err,
		}}, nil
	}

	// 准备模板数据
	templateData := struct {
		Servers []string
	}{
		Servers: ntpServers,
	}

	// 获取 chrony 模板内容
	templateContent, err := utils.GetTemplateContent("chrony.conf.tpl", chronyConfigTemplate)
	if err != nil {
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "GetTemplateContentFailed",
				Message: fmt.Sprintf("failed to get template content: %v", err),
			},
			Error: err,
		}}, nil
	}

	// 解析模板
	tmpl, err := template.New("chrony").Parse(templateContent)
	if err != nil {
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "ParseTemplateFailed",
				Message: fmt.Sprintf("failed to parse template: %v", err),
			},
			Error: err,
		}}, nil
	}

	// 渲染配置
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "ExecuteTemplateFailed",
				Message: fmt.Sprintf("failed to execute template: %v", err),
			},
			Error: err,
		}}, nil
	}
	desiredContent := content.String()

	// 检查现有配置是否一致
	currentContent, err := os.ReadFile("/etc/chrony/chrony.conf")
	if err == nil && string(currentContent) == desiredContent {
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseReady,
				Reason:  "NoChange",
				Message: "Configuration is up to date",
			},
		}}, nil
	}

	// 写入新配置
	if err := os.MkdirAll("/etc/chrony", 0755); err != nil {
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "CreateConfigDirFailed",
				Message: fmt.Sprintf("failed to create config directory: %v", err),
			},
			Error: err,
		}}, nil
	}

	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/chrony/chrony.conf", 0644); err != nil {
		return []*status.ReconcileResult{{
			Config: cfg,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  "WriteConfigFileFailed",
				Message: fmt.Sprintf("failed to write chrony.conf: %v", err),
			},
			Error: err,
		}}, nil
	}

	// 重新加载 chrony 配置
	if _, err := exec.LookPath("systemctl"); err == nil {
		restartCmd := exec.CommandContext(ctx, "systemctl", "restart", "chronyd")
		if output, err := restartCmd.CombinedOutput(); err != nil {
			return []*status.ReconcileResult{{
				Config: cfg,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  "RestartChronydFailed",
					Message: fmt.Sprintf("failed to restart chronyd: %v, output: %s", err, output),
				},
				Error: err,
			}}, nil
		}
	} else {
		// 如果 systemctl 不可用，尝试使用 chronyc fallback
		reloadCmd := exec.CommandContext(ctx, "chronyc", "reload", "sources")
		if output, err := reloadCmd.CombinedOutput(); err != nil {
			return []*status.ReconcileResult{{
				Config: cfg,
				Status: &systemv1.ResourceStatus{
					Phase:   status.PhaseError,
					Reason:  "ReloadChronydConfigFailed",
					Message: fmt.Sprintf("failed to reload chronyd config via chronyc: %v, output: %s", err, output),
				},
				Error: err,
			}}, nil
		}
	}

	return []*status.ReconcileResult{{
		Config: cfg,
		Status: &systemv1.ResourceStatus{
			Phase:   status.PhaseReady,
			Reason:  "Configured",
			Message: "Time configuration applied successfully",
		},
	}}, nil
}

func (h *LinuxTimeHandler) setTimezone(ctx context.Context, timezone string) error {
	// 检查 timedatectl 命令是否存在
	_, err := exec.LookPath("timedatectl")
	if err != nil {
		// timedatectl 命令不存在，尝试使用符号链接方式设置时区
		utils.Warnf("time", "timedatectl command not found, trying alternative method: %v", err)

		// 检查时区文件是否存在
		zoneInfoPath := fmt.Sprintf("/usr/share/zoneinfo/%s", timezone)
		if _, err := os.Stat(zoneInfoPath); os.IsNotExist(err) {
			return fmt.Errorf("timezone file not found: %s", zoneInfoPath)
		}

		// 删除现有的符号链接
		os.Remove("/etc/localtime")

		// 创建新的符号链接
		if err := os.Symlink(zoneInfoPath, "/etc/localtime"); err != nil {
			return fmt.Errorf("failed to create symlink for timezone: %v", err)
		}

		// 写入时区信息到 /etc/timezone 文件
		if err := utils.AtomicWriteFile([]byte(timezone+"\n"), "/etc/timezone", 0644); err != nil {
			return fmt.Errorf("failed to write timezone file: %v", err)
		}

		return nil
	}

	// 使用 timedatectl 设置时区
	cmd := exec.CommandContext(ctx, "timedatectl", "set-timezone", timezone)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set timezone: %v, output: %s", err, output)
	}
	return nil
}
