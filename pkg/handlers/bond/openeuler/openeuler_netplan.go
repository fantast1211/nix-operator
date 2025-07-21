package openeuler

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// OpenEulerBondNetplan openEuler系统Bond Netplan管理器
type OpenEulerBondNetplan struct {
	osInfo *controller.OSInfo
}

//go:embed openeuler_bond_netplan.tpl
var openeulerBondNetplanTemplate string

// NewOpenEulerBondNetplan 创建openEuler Bond Netplan实例
func NewOpenEulerBondNetplan(osInfo *controller.OSInfo) *OpenEulerBondNetplan {
	return &OpenEulerBondNetplan{
		osInfo: osInfo,
	}
}

func (obn *OpenEulerBondNetplan) IsInstall(ctx context.Context) bool {
	// 检查netplan是否安装
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "which", "netplan")
	if err := cmd.Run(); err != nil {
		utils.Debug("bond", "netplan command not found")
		return false
	}

	// 检查netplan配置目录是否存在
	netplanDirs := []string{"/etc/netplan", "/run/netplan", "/lib/netplan"}
	for _, dir := range netplanDirs {
		if _, err := os.Stat(dir); err == nil {
			utils.Infof("bond", "OpenEuler Bond Netplan detected at %s", dir)
			return true
		}
	}

	// openEuler特定检查：验证netplan版本兼容性
	if obn.osInfo != nil {
		if obn.osInfo.ID == "openeuler" && obn.osInfo.VersionID != "" {
			// 检查netplan版本
			cmd = exec.CommandContext(ctx, "netplan", "--version")
			if output, err := cmd.Output(); err == nil {
				version := strings.TrimSpace(string(output))
				utils.Infof("bond", "OpenEuler Bond Netplan version: %s", version)
				return true
			}
		}
	}

	utils.Debug("bond", "netplan configuration directories not found")
	return false
}

func (obn *OpenEulerBondNetplan) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := obn.ConfigureWithCheck(ctx, bondConfig)
	return err
}

func (obn *OpenEulerBondNetplan) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	// 验证Bond名称不能为空
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 获取模板内容，优先使用外部模板
	templateContent, err := utils.GetTemplateContent("openeuler_bond_netplan.tpl", openeulerBondNetplanTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to get openEuler Bond Netplan template: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("openeuler_bond_netplan").Parse(templateContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse openEuler Bond Netplan template: %v", err)
	}

	// 渲染模板
	var content strings.Builder
	if err := tmpl.Execute(&content, bondConfig); err != nil {
		return false, fmt.Errorf("failed to execute openEuler Bond Netplan template: %v", err)
	}

	newConfigData := []byte(content.String())

	// 确保netplan配置目录存在
	netplanDir := "/etc/netplan"
	if err := os.MkdirAll(netplanDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create netplan directory: %v", err)
	}

	// 检查配置文件是否存在以及内容是否相同
	configPath := filepath.Join(netplanDir, fmt.Sprintf("99-nix-operator-bond-%s.yaml", bondConfig.Name))
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if bytes.Equal(existingData, newConfigData) {
			utils.Debugf("bond", "OpenEuler Bond Netplan config for bond %s unchanged, skipping write", bondConfig.Name)
			return false, nil // 配置未变更
		}
	}

	// 写入netplan配置文件
	if err := utils.AtomicWriteFile(newConfigData, configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write openEuler Bond Netplan config: %v", err)
	}

	utils.Infof("bond", "OpenEuler Bond Netplan configuration written for bond %s", bondConfig.Name)
	return true, nil // 配置已变更
}

func (obn *OpenEulerBondNetplan) ReloadIfy(ctx context.Context) error {
	// openEuler特定的Bond网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 1. 验证netplan配置语法
	cmd := exec.CommandContext(ctx, "netplan", "try", "--timeout=30")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to validate openEuler Bond Netplan config: %v, output: %s", err, string(output))
	}

	// 2. 应用netplan配置
	cmd = exec.CommandContext(ctx, "netplan", "apply")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply openEuler Bond Netplan config: %v, output: %s", err, string(output))
	}

	// 3. openEuler特定：等待网络稳定
	time.Sleep(3 * time.Second)

	// 4. 验证bond接口是否正确创建
	cmd = exec.CommandContext(ctx, "ip", "link", "show", "type", "bond")
	output, err = cmd.CombinedOutput()
	if err != nil {
		utils.Warnf("bond", "Failed to verify bond interfaces: %v", err)
	} else {
		utils.Infof("bond", "Bond interfaces status: %s", string(output))
	}

	utils.Infof("bond", "OpenEuler Bond Netplan configuration applied successfully")
	return nil
}