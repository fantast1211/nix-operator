package ubuntu

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

//go:embed ubuntu_chrony.conf.tpl
var ubuntuChronyConfigTemplate string

// UbuntuTimeHandler Ubuntu专用时间处理器
type UbuntuTimeHandler struct {
	osInfo controller.OSInfo
}

// NewUbuntuTimeHandler 创建Ubuntu时间处理器实例
func NewUbuntuTimeHandler() *UbuntuTimeHandler {
	return &UbuntuTimeHandler{}
}

// Match 检查是否匹配Ubuntu系统
func (h *UbuntuTimeHandler) Match(osInfo controller.OSInfo) bool {
	if osInfo.ID == "ubuntu" && osInfo.KernelName == "Linux" {
		h.osInfo = osInfo
		utils.Infof("time", "Ubuntu %s time handler matched", osInfo.VersionID)
		return true
	}
	return false
}

// Reconcile 处理Ubuntu系统的时间配置
func (h *UbuntuTimeHandler) Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*status.ReconcileResult, error) {
	utils.Infof("time", "Starting Ubuntu time configuration reconciliation for config: %s", config.Metadata.Name)

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
	utils.Debugf("time", "Starting to apply Ubuntu time configuration for %s", config.Metadata.Name)
	err = h.applyUbuntuTimeConfiguration(ctx, timeSpec)
	if err != nil {
		utils.Errorf("time", "Failed to apply Ubuntu configuration %s: %v", config.Metadata.Name, err)
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
	utils.Infof("time", "Successfully applied Ubuntu configuration: %s", config.Metadata.Name)
	return &status.ReconcileResult{
		Config: config,
		Status: &systemv1.ResourceStatus{
			Phase:   status.PhaseReady,
			Reason:  status.ReasonAppliedSuccessfully,
			Message: "Ubuntu time configuration applied successfully",
		},
		Error: nil,
	}, nil
}

// applyUbuntuTimeConfiguration 应用Ubuntu时间配置项
func (h *UbuntuTimeHandler) applyUbuntuTimeConfiguration(ctx context.Context, timeSpec *systemv1.TimeConfigurationSpec) error {
	utils.Debugf("time", "Applying Ubuntu time configuration - timezone: %s, ntp enabled: %v", timeSpec.Timezone, timeSpec.Ntp != nil && timeSpec.Ntp.Enable)

	// 安全地打印 NTP 配置
	ntpEnable := false
	var ntpServers []string
	if timeSpec.Ntp != nil {
		ntpEnable = timeSpec.Ntp.Enable
		ntpServers = timeSpec.Ntp.Servers
	}
	utils.Debugf("time", "Parsed Ubuntu timeSpec: timezone=%s, ntp.enable=%v, ntp.servers=%v",
		timeSpec.Timezone, ntpEnable, ntpServers)

	// 设置时区
	if timeSpec.Timezone != "" {
		utils.Infof("time", "Setting Ubuntu timezone to: %s", timeSpec.Timezone)
		if err := h.setTimezone(ctx, timeSpec.Timezone); err != nil {
			return fmt.Errorf("failed to set timezone: %v", err)
		}
		utils.Infof("time", "Successfully set Ubuntu timezone to: %s", timeSpec.Timezone)
	}

	// 配置NTP
	if timeSpec.Ntp == nil || !timeSpec.Ntp.Enable {
		utils.Infof("time", "NTP is disabled, only timezone was configured")
		return nil
	}

	utils.Infof("time", "Configuring Ubuntu NTP with %d servers: %v", len(ntpServers), ntpServers)

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

	// 获取 Ubuntu chrony 模板内容
	templateContent, err := utils.GetTemplateContent("ubuntu_chrony.conf.tpl", ubuntuChronyConfigTemplate)
	if err != nil {
		return fmt.Errorf("failed to get Ubuntu template content: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("ubuntu_chrony").Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse Ubuntu template: %v", err)
	}

	// 渲染配置
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return fmt.Errorf("failed to execute Ubuntu template: %v", err)
	}
	desiredContent := content.String()

	// Ubuntu系统使用 /etc/chrony/chrony.conf 路径
	chronyConfigPath := "/etc/chrony/chrony.conf"

	// 检查现有配置是否一致
	currentContent, err := os.ReadFile(chronyConfigPath)
	if err == nil && string(currentContent) == desiredContent {
		utils.Infof("time", "Ubuntu chrony configuration is already up to date")
		return nil // 配置已是最新的
	}

	utils.Infof("time", "Updating Ubuntu chrony configuration file: %s", chronyConfigPath)

	// 写入新配置
	if err := os.MkdirAll("/etc/chrony", 0755); err != nil {
		return fmt.Errorf("failed to create Ubuntu config directory: %v", err)
	}

	if err := utils.AtomicWriteFile([]byte(desiredContent), chronyConfigPath, 0644); err != nil {
		return fmt.Errorf("failed to write Ubuntu chrony.conf: %v", err)
	}
	utils.Infof("time", "Successfully wrote Ubuntu chrony configuration to %s", chronyConfigPath)

	// 重新加载 chrony 配置
	if _, err := exec.LookPath("systemctl"); err == nil {
		utils.Infof("time", "Restarting Ubuntu chronyd service using systemctl")
		restartCmd := exec.CommandContext(ctx, "systemctl", "restart", "chronyd")
		if output, err := restartCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to restart chronyd on Ubuntu: %v, output: %s", err, output)
		}
		utils.Infof("time", "Successfully restarted Ubuntu chronyd service")
	} else {
		// 如果 systemctl 不可用，尝试使用 chronyc fallback
		utils.Infof("time", "Reloading Ubuntu chronyd configuration using chronyc")
		reloadCmd := exec.CommandContext(ctx, "chronyc", "reload", "sources")
		if output, err := reloadCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to reload chronyd config via chronyc on Ubuntu: %v, output: %s", err, output)
		}
		utils.Infof("time", "Successfully reloaded Ubuntu chronyd configuration")
	}

	return nil
}

// setTimezone 设置Ubuntu系统时区
func (h *UbuntuTimeHandler) setTimezone(ctx context.Context, timezone string) error {
	// 检查 timedatectl 命令是否存在
	_, err := exec.LookPath("timedatectl")
	if err != nil {
		// timedatectl 命令不存在，尝试使用符号链接方式设置时区
		utils.Warnf("time", "timedatectl command not found on Ubuntu, trying alternative method: %v", err)

		// 检查时区文件是否存在
		zoneInfoPath := fmt.Sprintf("/usr/share/zoneinfo/%s", timezone)
		if _, err := os.Stat(zoneInfoPath); os.IsNotExist(err) {
			return fmt.Errorf("timezone file not found: %s", zoneInfoPath)
		}

		// 删除现有的符号链接
		os.Remove("/etc/localtime")

		// 创建新的符号链接
		if err := os.Symlink(zoneInfoPath, "/etc/localtime"); err != nil {
			return fmt.Errorf("failed to create symlink for timezone on Ubuntu: %v", err)
		}

		// 写入时区信息到 /etc/timezone 文件
		if err := utils.AtomicWriteFile([]byte(timezone+"\n"), "/etc/timezone", 0644); err != nil {
			return fmt.Errorf("failed to write timezone file on Ubuntu: %v", err)
		}

		return nil
	}

	// 使用 timedatectl 设置时区
	cmd := exec.CommandContext(ctx, "timedatectl", "set-timezone", timezone)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set timezone on Ubuntu: %v, output: %s", err, output)
	}
	return nil
}
