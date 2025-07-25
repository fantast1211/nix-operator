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

// init函数已移至init.go文件中统一管理

type LinuxTimeHandler struct {
}

func (h *LinuxTimeHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

func (h *LinuxTimeHandler) Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*status.ReconcileResult, error) {
	// Controller 层已经进行了节点选择器筛选，这里直接处理单个配置
	utils.Infof("time", "Starting time configuration reconciliation for config: %s", config.Metadata.Name)

	// 解析配置规格
	timeSpec, err := utils.UnmarshalSpec[*systemv1.TimeConfigurationSpec](config.Spec)
	if err != nil {
		utils.Errorf("time", "Failed to unmarshal spec for config %s: %v", config.Metadata.Name, err)
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonSpecError,
				Message: fmt.Sprintf("failed to unmarshal spec: %v", err),
			},
			Error: err,
		}, err
	}

	// 应用配置
	utils.Debugf("time", "Starting to apply time configuration for %s", config.Metadata.Name)
	err = h.applyTimeConfiguration(ctx, timeSpec)
	if err != nil {
		utils.Errorf("time", "Failed to apply configuration %s: %v", config.Metadata.Name, err)
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonReconcileError,
				Message: fmt.Sprintf("failed to apply configuration: %v", err),
			},
			Error: err,
		}, err
	}

	// 应用成功
	utils.Infof("time", "Successfully applied configuration: %s", config.Metadata.Name)
	return &status.ReconcileResult{
		Config: config,
		Status: &systemv1.ResourceStatus{
			Phase:   status.PhaseReady,
			Reason:  status.ReasonAppliedSuccessfully,
			Message: "Time configuration applied successfully",
		},
		Error: nil,
	}, nil
}

// applyTimeConfiguration 应用时间配置项
func (h *LinuxTimeHandler) applyTimeConfiguration(ctx context.Context, timeSpec *systemv1.TimeConfigurationSpec) error {
	utils.Debugf("time", "Applying time configuration - timezone: %s, ntp enabled: %v", timeSpec.Timezone, timeSpec.Ntp != nil && timeSpec.Ntp.Enable)

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
		utils.Infof("time", "Setting timezone to: %s", timeSpec.Timezone)
		if err := h.setTimezone(ctx, timeSpec.Timezone); err != nil {
			return fmt.Errorf("failed to set timezone: %v", err)
		}
		utils.Infof("time", "Successfully set timezone to: %s", timeSpec.Timezone)
	}

	// 配置NTP
	if timeSpec.Ntp == nil || !timeSpec.Ntp.Enable {
		utils.Infof("time", "NTP is disabled, only timezone was configured")
		return nil
	}

	utils.Infof("time", "Configuring NTP with %d servers: %v", len(ntpServers), ntpServers)

	// 检查 chronyd 是否存在
	if _, err := exec.LookPath("chronyd"); err != nil {
		return fmt.Errorf("chronyd not found in system, please install chrony before applying NTP configuration")
	}

	// 检查 chronyc 是否存在
	if _, err := exec.LookPath("chronyc"); err != nil {
		return fmt.Errorf("chronyc command not found, cannot reload chrony configuration")
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
		return fmt.Errorf("failed to get template content: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("chrony").Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}

	// 渲染配置
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}
	desiredContent := content.String()

	// 检查现有配置是否一致
	currentContent, err := os.ReadFile("/etc/chrony.conf")
	if err == nil && string(currentContent) == desiredContent {
		utils.Infof("time", "Chrony configuration is already up to date")
		return nil // 配置已是最新的
	}

	utils.Infof("time", "Updating chrony configuration file: /etc/chrony.conf")

	// 写入新配置 (Linux通用路径)

	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/chrony.conf", 0644); err != nil {
		return fmt.Errorf("failed to write chrony.conf: %v", err)
	}
	utils.Infof("time", "Successfully wrote chrony configuration to /etc/chrony.conf")

	// 重新加载 chrony 配置
	if _, err := exec.LookPath("systemctl"); err == nil {
		utils.Infof("time", "Restarting chronyd service using systemctl")
		restartCmd := exec.CommandContext(ctx, "systemctl", "restart", "chronyd")
		if output, err := restartCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to restart chronyd: %v, output: %s", err, output)
		}
		utils.Infof("time", "Successfully restarted chronyd service")
	} else {
		// 如果 systemctl 不可用，尝试使用 chronyc fallback
		utils.Infof("time", "Reloading chronyd configuration using chronyc")
		reloadCmd := exec.CommandContext(ctx, "chronyc", "reload", "sources")
		if output, err := reloadCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to reload chronyd config via chronyc: %v, output: %s", err, output)
		}
		utils.Infof("time", "Successfully reloaded chronyd configuration")
	}

	return nil
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
