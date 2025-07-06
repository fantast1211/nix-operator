package hosts

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"text/template"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/domain"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed hosts.tpl
var hostsTemplate string

//go:embed hostname.tpl
var hostnameTemplate string

// 外部模板目录，用于高优先级覆盖
const externalTemplateDir = "/etc/nix-operator/templates"

func init() {
	controller.RegisterHandler("HostsConfiguration", &LinuxHostsHandler{})
}

type LinuxHostsHandler struct{}

func (h *LinuxHostsHandler) Match(osInfo controller.OSInfo) bool {
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

func (h *LinuxHostsHandler) Reconcile(ctx context.Context, cfg *systemv1.ResourceConfig) (*domain.ReconcileResult, error) {
	// 解析hosts配置
	var hostsSpec *systemv1.HostsConfigurationSpec

	// 从 anypb.Any 中解析 HostsConfigurationSpec
	if cfg.Spec != nil {
		// 尝试将 cfg.Spec 解码为目标结构体
		// if err := cfg.Spec.UnmarshalTo(&hostsSpec); err != nil {
		// 	return nil, fmt.Errorf("failed to unmarshal spec to HostsConfigurationSpec: %w", err)
		// }
		var err error
		hostsSpec, err = UnmarshalSpec[*systemv1.HostsConfigurationSpec](cfg.Spec)
		if err != nil {
			return nil, fmt.Errorf("decode spec failed: %w", err)
		}
	} else {
		return nil, fmt.Errorf("spec is nil")
	}

	utils.Debugf("hosts", "Parsed hostsSpec: nodeSelector=%+v, hostname=%s, hosts=%+v",
		hostsSpec.NodeSelector, hostsSpec.Hostname, hostsSpec.Hosts)

	// 第一阶段：优先匹配有 nodeSelector 的配置项
	// 检查是否有有效的 nodeSelector
	if h.hasValidNodeSelector(hostsSpec.NodeSelector) {
		match, err := utils.MatchNodeSelector(hostsSpec.NodeSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to check node selector: %v", err)
		}
		if match {
			// 找到匹配的特定配置，应用并返回有效配置
			result, err := h.applyConfiguration(ctx, hostsSpec)
			if err != nil {
				return nil, err
			}
			// 设置有效配置，多机场景下Config = EffectiveConfig，在按节点拆分的场景下，
			// 由于每个配置文件只包含一个节点的配置， Config 和 EffectiveConfig 实际上是相同的
			result.Effective = cfg
			return result, nil
		}
	}

	// 第二阶段：如果没有匹配的特定配置，使用通用配置
	if !h.hasValidNodeSelector(hostsSpec.NodeSelector) {
		// 应用通用配置并返回有效配置
		result, err := h.applyConfiguration(ctx, hostsSpec)
		if err != nil {
			return nil, err
		}
		// 设置有效配置，多机场景下Config = EffectiveConfig，在按节点拆分的场景下，
		// 由于每个配置文件只包含一个节点的配置， Config 和 EffectiveConfig 实际上是相同的
		result.Effective = cfg
		return result, nil
	}

	// 没有找到任何匹配的配置
	return &domain.ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   "Skipped",
			Reason:  "NoMatchingConfiguration",
			Message: "No matching configuration found for this host",
		},
		// 即使跳过，也应该返回原始配置作为有效配置
		Effective: cfg,
	}, nil
}

func (h *LinuxHostsHandler) configureHostname(ctx context.Context, hostname string) error {
	// 获取hostname模板
	templateContent, err := getTemplateContent("hostname.tpl", hostnameTemplate)
	if err != nil {
		return err
	}

	// 解析模板
	tmpl, err := template.New("hostname").Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse hostname template: %v", err)
	}

	// 准备模板数据
	data := struct {
		Hostname string
	}{
		Hostname: hostname,
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, data); err != nil {
		return fmt.Errorf("failed to execute hostname template: %v", err)
	}

	desiredContent := content.String()

	// 读取现有配置
	currentContent, err := os.ReadFile("/etc/hostname")
	if err == nil && string(currentContent) == desiredContent {
		return nil // 配置相同，无需更新
	}

	// 使用工具函数原子性写入文件
	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/hostname", 0644); err != nil {
		return fmt.Errorf("failed to write hostname file: %v", err)
	}

	// 使用系统调用设置主机名
	return h.setHostname(ctx, hostname)
}

func (h *LinuxHostsHandler) setHostname(ctx context.Context, hostname string) error {
	// 使用系统调用设置主机名
	return syscall.Sethostname([]byte(hostname))
}

func (h *LinuxHostsHandler) configureHosts(ctx context.Context, hosts []*systemv1.HostEntry) error {
	// 获取hosts模板
	templateContent, err := getTemplateContent("hosts.tpl", hostsTemplate)
	if err != nil {
		return err
	}

	// 解析模板
	tmpl, err := template.New("hosts").Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse hosts template: %v", err)
	}

	// 准备模板数据
	data := struct {
		Hosts []*systemv1.HostEntry
	}{
		Hosts: hosts,
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, data); err != nil {
		return fmt.Errorf("failed to execute hosts template: %v", err)
	}

	desiredContent := content.String()

	// 读取现有配置
	currentContent, err := os.ReadFile("/etc/hosts")
	if err == nil && string(currentContent) == desiredContent {
		return nil // 配置相同，无需更新
	}

	// 使用工具函数原子性写入文件
	return utils.AtomicWriteFile([]byte(desiredContent), "/etc/hosts", 0644)
}

// hasValidNodeSelector 检查是否有有效的 nodeSelector
func (h *LinuxHostsHandler) hasValidNodeSelector(selector *systemv1.NodeSelector) bool {
	if selector == nil {
		return false
	}
	return selector.MachineId != "" || selector.Ip != ""
}

// applyConfiguration 应用配置项
func (h *LinuxHostsHandler) applyConfiguration(ctx context.Context, hostsSpec *systemv1.HostsConfigurationSpec) (*domain.ReconcileResult, error) {
	// 处理hostname配置
	if hostsSpec.Hostname != "" {
		if err := h.configureHostname(ctx, hostsSpec.Hostname); err != nil {
			return nil, fmt.Errorf("failed to configure hostname: %v", err)
		}
	}

	// 处理hosts配置
	if len(hostsSpec.Hosts) > 0 {
		if err := h.configureHosts(ctx, hostsSpec.Hosts); err != nil {
			return nil, fmt.Errorf("failed to configure hosts: %v", err)
		}
	}

	return &domain.ReconcileResult{
		Status: &systemv1.ResourceStatus{
			Phase:   "Ready",
			Reason:  "Configured",
			Message: "Hosts configuration applied successfully",
		},
	}, nil
}
