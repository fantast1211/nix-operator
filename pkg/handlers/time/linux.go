package time

import (
	context "context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"path/filepath"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed chrony.conf.tpl
var chronyConfigTemplate string

// 外部模板目录，用于高优先级覆盖
const externalTemplateDir = "/etc/nix-operator/templates"

func init() {
	controller.RegisterHandler("TimeConfiguration", &LinuxTimeHandler{})
}

type LinuxTimeHandler struct{}

func (h *LinuxTimeHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

// 获取模板内容，优先使用外部模板
func getTemplateContent(templateName, defaultContent string) (string, error) {
	externalPath := filepath.Join(externalTemplateDir, templateName)
	if _, err := os.Stat(externalPath); err == nil {
		content, err := os.ReadFile(externalPath)
		if err != nil {
			return "", fmt.Errorf("failed to read external template %s: %v", externalPath, err)
		}
		return string(content), nil
	}
	return defaultContent, nil
}

func (h *LinuxTimeHandler) Reconcile(ctx context.Context, configs []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error) {
	var results []*domain.ReconcileResult

	// time类型配置只会存在一个，因为这个配置可以各节点一致
	if len(configs) == 0 {
		result, _ := status.ReconcileError(nil, "NoTimeConfig", fmt.Errorf("no time configuration found"))
		return []*domain.ReconcileResult{result}, nil
	}

	// 使用第一个配置，如果有多个则记录警告并为其他配置设置跳过状态
	cfg := configs[0]
	if len(configs) > 1 {
		utils.Warnf("time", "Multiple time configurations found (%d), using the first one: %s", len(configs), cfg.Metadata.Name)
		// 为其他配置设置跳过状态
		for i := 1; i < len(configs); i++ {
			result, _ := status.ReconcileSkipped(configs[i], status.ReasonSkippedByConfig, "Multiple time configurations found, only the first one is applied")
			results = append(results, result)
		}
	}

	// 解析时间配置
	var timeSpec *systemv1.TimeConfigurationSpec

	// 从 anypb.Any 中解析 TimeConfigurationSpec
	if cfg.Spec != nil {
		var err error
		timeSpec, err = utils.UnmarshalSpec[*systemv1.TimeConfigurationSpec](cfg.Spec)
		if err != nil {
			result, _ := status.ReconcileError(cfg, "DecodeSpecFailed", fmt.Errorf("decode spec failed: %w", err))
			results = append(results, result)
			return results, nil
		}
	} else {
		result, _ := status.ReconcileError(cfg, "SpecIsNil", fmt.Errorf("spec is nil"))
		results = append(results, result)
		return results, nil
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
			result, _ := status.ReconcileError(cfg, "SetTimezoneFailed", fmt.Errorf("failed to set timezone: %v", err))
			results = append(results, result)
			return results, nil
		}
	}

	// 配置NTP
	if timeSpec.Ntp == nil || !timeSpec.Ntp.Enable {
		result, _ := status.ReconcileReady(cfg, "NTPDisabled", "NTP is disabled, only timezone was configured")
		results = append(results, result)
		return results, nil
	}

	// 检查 chronyd 是否存在
	if _, err := exec.LookPath("chronyd"); err != nil {
		result, _ := status.ReconcileError(cfg, "ChronydNotInstalled", fmt.Errorf("chronyd not found in system, please install chrony before applying NTP configuration"))
		results = append(results, result)
		return results, nil
	}

	// 检查 chronyc 是否存在
	if _, err := exec.LookPath("chronyc"); err != nil {
		result, _ := status.ReconcileError(cfg, "ChronycNotInstalled", fmt.Errorf("chronyc command not found, cannot reload chrony configuration"))
		results = append(results, result)
		return results, nil
	}

	// 准备模板数据
	// 重用前面已经声明的 ntpServers 变量
	templateData := struct {
		Servers []string
	}{
		Servers: ntpServers,
	}

	// 获取 chrony 模板内容
	templateContent, err := getTemplateContent("chrony.conf.tpl", chronyConfigTemplate)
	if err != nil {
		result, _ := status.ReconcileError(cfg, "GetTemplateContentFailed", err)
		results = append(results, result)
		return results, nil
	}

	// 解析模板
	tmpl, err := template.New("chrony").Parse(templateContent)
	if err != nil {
		result, _ := status.ReconcileError(cfg, "ParseTemplateFailed", fmt.Errorf("failed to parse template: %v", err))
		results = append(results, result)
		return results, nil
	}

	// 渲染配置
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		result, _ := status.ReconcileError(cfg, "ExecuteTemplateFailed", fmt.Errorf("failed to execute template: %v", err))
		results = append(results, result)
		return results, nil
	}
	desiredContent := content.String()

	// 检查现有配置是否一致
	currentContent, err := os.ReadFile("/etc/chrony/chrony.conf")
	if err == nil && string(currentContent) == desiredContent {
		result, _ := status.ReconcileNoChange(cfg, "Configuration is up to date")
		results = append(results, result)
		return results, nil
	}

	// 写入新配置
	if err := os.MkdirAll("/etc/chrony", 0755); err != nil {
		result, _ := status.ReconcileError(cfg, "CreateConfigDirFailed", fmt.Errorf("failed to create config directory: %v", err))
		results = append(results, result)
		return results, nil
	}

	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/chrony/chrony.conf", 0644); err != nil {
		result, _ := status.ReconcileError(cfg, "WriteConfigFileFailed", fmt.Errorf("failed to write chrony.conf: %v", err))
		results = append(results, result)
		return results, nil
	}

	// 重新加载 chrony 配置
	// 优先尝试重启 chronyd 服务
	if _, err := exec.LookPath("systemctl"); err == nil {
		restartCmd := exec.CommandContext(ctx, "systemctl", "restart", "chronyd")
		if output, err := restartCmd.CombinedOutput(); err != nil {
			result, _ := status.ReconcileError(cfg, "RestartChronydFailed", fmt.Errorf("failed to restart chronyd: %v, output: %s", err, output))
			results = append(results, result)
			return results, nil
		}
	} else {
		// 如果 systemctl 不可用（如非 root 或 alpine 容器），尝试使用 chronyc fallback
		reloadCmd := exec.CommandContext(ctx, "chronyc", "reload", "sources")
		if output, err := reloadCmd.CombinedOutput(); err != nil {
			result, _ := status.ReconcileError(cfg, "ReloadChronydConfigFailed", fmt.Errorf("failed to reload chronyd config via chronyc: %v, output: %s", err, output))
			results = append(results, result)
			return results, nil
		}
	}

	result, _ := status.ReconcileReady(cfg, "Configured", "Time configuration applied successfully")
	results = append(results, result)
	return results, nil
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
