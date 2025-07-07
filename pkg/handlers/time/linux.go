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

func (h *LinuxTimeHandler) Reconcile(ctx context.Context, cfg *systemv1.ResourceConfig) (*domain.ReconcileResult, error) {
	// 解析时间配置
	var timeSpec *systemv1.TimeConfigurationSpec

	// 从 anypb.Any 中解析 TimeConfigurationSpec
	if cfg.Spec != nil {
		var err error
		timeSpec, err = utils.UnmarshalSpec[*systemv1.TimeConfigurationSpec](cfg.Spec)
		if err != nil {
			return status.ReconcileError(cfg, "DecodeSpecFailed", fmt.Errorf("decode spec failed: %w", err))
		}
	} else {
		return status.ReconcileError(cfg, "SpecIsNil", fmt.Errorf("spec is nil"))
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
			return status.ReconcileError(cfg, "SetTimezoneFailed", fmt.Errorf("failed to set timezone: %v", err))
		}
	}

	// 配置NTP
	if timeSpec.Ntp == nil || !timeSpec.Ntp.Enable {
		return status.ReconcileReady(cfg, "NTPDisabled", "NTP is disabled, only timezone was configured")
	}

	// 检查 chronyd 是否存在
	if _, err := exec.LookPath("chronyd"); err != nil {
		return status.ReconcileError(cfg, "ChronydNotInstalled", fmt.Errorf("chronyd not found in system, please install chrony before applying NTP configuration"))
	}

	// 检查 chronyc 是否存在
	if _, err := exec.LookPath("chronyc"); err != nil {
		return status.ReconcileError(cfg, "ChronycNotInstalled", fmt.Errorf("chronyc command not found, cannot reload chrony configuration"))
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
		return status.ReconcileError(cfg, "GetTemplateContentFailed", err)
	}

	// 解析模板
	tmpl, err := template.New("chrony").Parse(templateContent)
	if err != nil {
		return status.ReconcileError(cfg, "ParseTemplateFailed", fmt.Errorf("failed to parse template: %v", err))
	}

	// 渲染配置
	var content strings.Builder
	if err := tmpl.Execute(&content, templateData); err != nil {
		return status.ReconcileError(cfg, "ExecuteTemplateFailed", fmt.Errorf("failed to execute template: %v", err))
	}
	desiredContent := content.String()

	// 检查现有配置是否一致
	currentContent, err := os.ReadFile("/etc/chrony/chrony.conf")
	if err == nil && string(currentContent) == desiredContent {
		return status.ReconcileNoChange(cfg, "Configuration is up to date")
	}

	// 写入新配置
	if err := os.MkdirAll("/etc/chrony", 0755); err != nil {
		return status.ReconcileError(cfg, "CreateConfigDirFailed", fmt.Errorf("failed to create config directory: %v", err))
	}

	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/chrony/chrony.conf", 0644); err != nil {
		return status.ReconcileError(cfg, "WriteConfigFileFailed", fmt.Errorf("failed to write chrony.conf: %v", err))
	}

	// 重新加载 chrony 配置
	// 优先尝试重启 chronyd 服务
	if _, err := exec.LookPath("systemctl"); err == nil {
		restartCmd := exec.CommandContext(ctx, "systemctl", "restart", "chronyd")
		if output, err := restartCmd.CombinedOutput(); err != nil {
			return status.ReconcileError(cfg, "RestartChronydFailed", fmt.Errorf("failed to restart chronyd: %v, output: %s", err, output))
		}
	} else {
		// 如果 systemctl 不可用（如非 root 或 alpine 容器），尝试使用 chronyc fallback
		reloadCmd := exec.CommandContext(ctx, "chronyc", "reload", "sources")
		if output, err := reloadCmd.CombinedOutput(); err != nil {
			return status.ReconcileError(cfg, "ReloadChronydConfigFailed", fmt.Errorf("failed to reload chronyd config via chronyc: %v, output: %s", err, output))
		}
	}

	return status.ReconcileReady(cfg, "Configured", "Time configuration applied successfully")
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
