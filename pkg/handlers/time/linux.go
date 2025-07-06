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
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/utils"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"reflect"
	"encoding/json"
	"google.golang.org/protobuf/encoding/protojson"
	"path/filepath"
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

func UnmarshalSpec[T proto.Message](anySpec *anypb.Any) (T, error) {
	var zero T
	if anySpec == nil {
		return zero, fmt.Errorf("spec is nil")
	}

	// 创建目标类型的实例
	msg := reflect.New(reflect.TypeOf(zero).Elem()).Interface().(proto.Message)

	// 直接尝试解包为目标类型
	if err := anySpec.UnmarshalTo(msg); err != nil {
		// 如果直接解包失败，尝试通过Struct中转
		var structSpec structpb.Struct
		if err2 := anySpec.UnmarshalTo(&structSpec); err2 != nil {
			return zero, fmt.Errorf("failed to unmarshal Any to target type: %w", err)
		}

		// 将 structpb.Struct 转换为 JSON，并过滤掉 @type 字段
		structMap := structSpec.AsMap()
		// 移除 @type 字段，因为它不属于目标 proto 消息的字段
		delete(structMap, "@type")

		jsonBytes, err := json.Marshal(structMap)
		if err != nil {
			return zero, fmt.Errorf("failed to marshal struct to JSON: %w", err)
		}

		// 使用 protojson 将 JSON 解析为 proto 消息
		if err := protojson.Unmarshal(jsonBytes, msg); err != nil {
			return zero, fmt.Errorf("failed to unmarshal JSON to proto: %w", err)
		}
	}

	return msg.(T), nil
}

func (h *LinuxTimeHandler) Reconcile(ctx context.Context, cfg *systemv1.ResourceConfig) (*domain.ReconcileResult, error) {
	// 解析时间配置
	var timeSpec *systemv1.TimeConfigurationSpec

	// 从 anypb.Any 中解析 TimeConfigurationSpec
	if cfg.Spec != nil {
		var err error
		timeSpec, err = UnmarshalSpec[*systemv1.TimeConfigurationSpec](cfg.Spec)
		if err != nil {
			return nil, fmt.Errorf("decode spec failed: %w", err)
		}
	} else {
		return nil, fmt.Errorf("spec is nil")
	}

	utils.Debugf("time", "Parsed timeSpec: timezone=%s, ntp.enable=%v, ntp.servers=%v",
		timeSpec.Timezone, timeSpec.Ntp.Enable, timeSpec.Ntp.Servers)

	// 设置时区
	if timeSpec.Timezone != "" {
		if err := h.setTimezone(ctx, timeSpec.Timezone); err != nil {
			return nil, fmt.Errorf("failed to set timezone: %v", err)
		}
	}

	// 配置NTP
	if !timeSpec.Ntp.Enable {
		return &domain.ReconcileResult{
			Status: &systemv1.ResourceStatus{
				Phase:   "Ready",
				Reason:  "NTPDisabled",
				Message: "NTP is disabled, only timezone was configured",
			},
			Effective: cfg,
		}, nil
	}

	// 准备模板数据
	templateData := struct {
		Servers []string
	}{
		Servers: timeSpec.Ntp.Servers,
	}

	// 获取chrony模板
	templateContent, err := getTemplateContent("chrony.conf.tpl", chronyConfigTemplate)
	if err != nil {
		return nil, err
	}

	// 解析模板
	tmpl, err := template.New("chrony").Parse(templateContent)
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
			return &domain.ReconcileResult{
				Status: &systemv1.ResourceStatus{
					Phase:   "Ready",
					Reason:  "NoChange",
					Message: "Configuration is up to date",
				},
				Effective: cfg,
			}, nil // 配置相同，无需更新
		}
	}
	// 如果文件不存在或读取失败，继续写入新配置

	// 确保目录存在
	os.MkdirAll("/etc/chrony", 0755)

	// 使用工具函数原子性写入文件
	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/chrony/chrony.conf", 0644); err != nil {
		return nil, fmt.Errorf("failed to write chrony.conf: %v", err)
	}

	// 重新加载配置
	// 检查 chronyc 命令是否存在
	_, err = exec.LookPath("chronyc")
	if err != nil {
		// chronyc 命令不存在，记录警告但不返回错误
		utils.Warnf("time", "chronyc command not found, skipping reload: %v", err)
	} else {
		// 执行 chronyc reload sources 命令
		cmd := exec.CommandContext(ctx, "chronyc", "reload", "sources")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to reload chronyd config: %v, output: %s", err, output)
		}
	}

	return &domain.ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   "Ready",
			Reason:  "Configured",
			Message: "Time configuration applied successfully",
		},
		Effective: cfg,
	}, nil
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
		if err := os.WriteFile("/etc/timezone", []byte(timezone + "\n"), 0644); err != nil {
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