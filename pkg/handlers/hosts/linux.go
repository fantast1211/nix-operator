package hosts

import (
	context "context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"syscall"
	"text/template"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/status"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed hosts.tpl
var hostsTemplate string

//go:embed hostname.tpl
var hostnameTemplate string

func init() {
	controller.RegisterHandler("HostsConfiguration", &LinuxHostsHandler{})
}

type LinuxHostsHandler struct {
	// 移除statusManager依赖，Handler不再负责状态管理
}

func (h *LinuxHostsHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

func (h *LinuxHostsHandler) Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*status.ReconcileResult, error) {
	// Controller 层已经进行了节点选择器筛选，这里直接处理单个配置
	utils.Infof("hosts", "Starting hosts configuration reconciliation for config: %s", config.Metadata.Name)

	// 解析配置规格
	hostsSpec, err := utils.UnmarshalSpec[*systemv1.HostsConfigurationSpec](config.Spec)
	if err != nil {
		utils.Errorf("hosts", "Failed to unmarshal spec for config %s: %v", config.Metadata.Name, err)
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonSpecError,
				Message: fmt.Sprintf("failed to unmarshal spec: %v", err),
			},
			Error: err,
		}, nil
	}

	// 应用配置
	utils.Debugf("hosts", "Starting to apply hosts configuration for %s", config.Metadata.Name)
	err = h.applyConfiguration(ctx, hostsSpec)
	if err != nil {
		utils.Errorf("hosts", "Failed to apply configuration %s: %v", config.Metadata.Name, err)
		return &status.ReconcileResult{
			Config: config,
			Status: &systemv1.ResourceStatus{
				Phase:   status.PhaseError,
				Reason:  status.ReasonReconcileError,
				Message: fmt.Sprintf("failed to apply configuration: %v", err),
			},
			Error: err,
		}, nil
	}

	// 应用成功
	utils.Infof("hosts", "Successfully applied configuration: %s", config.Metadata.Name)
	utils.Infof("hosts", "Hosts configuration reconciliation completed")
	return &status.ReconcileResult{
		Config: config,
		Status: &systemv1.ResourceStatus{
			Phase:   status.PhaseReady,
			Reason:  status.ReasonAppliedSuccessfully,
			Message: "Hosts configuration applied successfully",
		},
		Error: nil,
	}, nil
}

func (h *LinuxHostsHandler) configureHostname(ctx context.Context, hostname string) error {
	utils.Infof("hosts", "Configuring hostname: %s", hostname)
	// 获取hostname模板
	templateContent, err := utils.GetTemplateContent("hostname.tpl", hostnameTemplate)
	if err != nil {
		utils.Errorf("hosts", "Failed to get hostname template: %v", err)
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
		utils.Debugf("hosts", "Hostname configuration is already up to date: %s", hostname)
		return nil // 配置相同，无需更新
	}
	utils.Debugf("hosts", "Hostname needs to be updated from current state")

	// 使用工具函数原子性写入文件
	utils.Debugf("hosts", "Writing hostname to /etc/hostname")
	if err := utils.AtomicWriteFile([]byte(desiredContent), "/etc/hostname", 0644); err != nil {
		utils.Errorf("hosts", "Failed to write hostname file: %v", err)
		return fmt.Errorf("failed to write hostname file: %v", err)
	}

	// 使用系统调用设置主机名
	utils.Debugf("hosts", "Setting system hostname via syscall")
	return h.setHostname(ctx, hostname)
}

func (h *LinuxHostsHandler) setHostname(ctx context.Context, hostname string) error {
	// 使用系统调用设置主机名
	utils.Debugf("hosts", "Executing sethostname syscall for: %s", hostname)
	err := syscall.Sethostname([]byte(hostname))
	if err != nil {
		utils.Errorf("hosts", "Failed to set hostname via syscall: %v", err)
		return err
	}
	utils.Infof("hosts", "Successfully set hostname to: %s", hostname)
	return nil
}

func (h *LinuxHostsHandler) configureHosts(ctx context.Context, hosts []*systemv1.HostEntry) error {
	utils.Infof("hosts", "Configuring hosts file with %d entries", len(hosts))
	// 获取hosts模板
	templateContent, err := utils.GetTemplateContent("hosts.tpl", hostsTemplate)
	if err != nil {
		utils.Errorf("hosts", "Failed to get hosts template: %v", err)
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
		utils.Debugf("hosts", "Hosts file configuration is already up to date")
		return nil // 配置相同，无需更新
	}
	utils.Debugf("hosts", "Hosts file needs to be updated")

	// 使用工具函数原子性写入文件
	utils.Debugf("hosts", "Writing hosts configuration to /etc/hosts")
	err = utils.AtomicWriteFile([]byte(desiredContent), "/etc/hosts", 0644)
	if err != nil {
		utils.Errorf("hosts", "Failed to write hosts file: %v", err)
		return err
	}
	utils.Infof("hosts", "Successfully updated hosts file with %d entries", len(hosts))
	return nil
}

// hasValidNodeSelector 方法已移除，节点选择器匹配逻辑已迁移到Controller层

// applyConfiguration 应用配置项
func (h *LinuxHostsHandler) applyConfiguration(ctx context.Context, hostsSpec *systemv1.HostsConfigurationSpec) error {
	utils.Debugf("hosts", "Applying hosts configuration - hostname: %s, hosts entries: %d", hostsSpec.Hostname, len(hostsSpec.Hosts))

	// 处理hostname配置
	if hostsSpec.Hostname != "" {
		utils.Debugf("hosts", "Processing hostname configuration")
		if err := h.configureHostname(ctx, hostsSpec.Hostname); err != nil {
			utils.Errorf("hosts", "Failed to configure hostname: %v", err)
			return fmt.Errorf("failed to configure hostname: %v", err)
		}
		utils.Infof("hosts", "Hostname configuration completed successfully")
	} else {
		utils.Debugf("hosts", "No hostname configuration specified")
	}

	// 处理hosts配置
	if len(hostsSpec.Hosts) > 0 {
		utils.Debugf("hosts", "Processing hosts entries configuration")
		if err := h.configureHosts(ctx, hostsSpec.Hosts); err != nil {
			utils.Errorf("hosts", "Failed to configure hosts: %v", err)
			return fmt.Errorf("failed to configure hosts: %v", err)
		}
		utils.Infof("hosts", "Hosts entries configuration completed successfully")
	} else {
		utils.Debugf("hosts", "No hosts entries specified")
	}

	utils.Infof("hosts", "All hosts configuration applied successfully")
	return nil
}
