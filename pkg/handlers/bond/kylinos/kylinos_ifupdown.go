package kylinos

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

//go:embed kylinos_ifupdown.tpl
var kylinOSBondIfupdownTemplate string

// KylinOSBondIfupdown KylinOS ifupdown Bond管理器
type KylinOSBondIfupdown struct {
	osInfo *controller.OSInfo
}

// NewKylinOSBondIfupdown 创建KylinOS ifupdown Bond实例
func NewKylinOSBondIfupdown(osInfo *controller.OSInfo) *KylinOSBondIfupdown {
	return &KylinOSBondIfupdown{
		osInfo: osInfo,
	}
}

// IsInstall 检查ifupdown是否已安装
func (bi *KylinOSBondIfupdown) IsInstall(ctx context.Context) bool {
	// 检查ifup/ifdown命令是否存在
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "which", "ifup")
	if err := cmd.Run(); err != nil {
		return false
	}
	cmd = exec.CommandContext(ctx, "which", "ifdown")
	if err := cmd.Run(); err != nil {
		return false
	}

	// 检查网络配置目录是否存在
	if _, err := os.Stat("/etc/sysconfig/network-scripts"); err != nil {
		return false
	}

	return true
}

// Configure 配置Bond接口
func (bi *KylinOSBondIfupdown) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := bi.ConfigureWithCheck(ctx, bondConfig)
	return err
}

// ConfigureWithCheck 配置Bond接口并检查是否有变更
func (bi *KylinOSBondIfupdown) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	utils.Infof("bond", "Configuring KylinOS bond interface %s with ifupdown", bondConfig.Name)

	// 生成配置内容
	configContent, err := bi.generateBondConfig(bondConfig)
	if err != nil {
		return false, fmt.Errorf("failed to generate bond config: %v", err)
	}

	// 配置文件路径
	configPath := fmt.Sprintf("/etc/sysconfig/network-scripts/ifcfg-%s", bondConfig.Name)

	// 检查配置文件是否存在以及内容是否相同
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if string(existingData) == configContent {
			return false, nil // 配置未变更
		}
	}

	// 写入配置文件
	if err := utils.AtomicWriteFile([]byte(configContent), configPath, 0644); err != nil {
		return false, fmt.Errorf("failed to write bond config file: %v", err)
	}

	changed := true

	if changed {
		utils.Infof("bond", "KylinOS ifupdown bond config for %s updated", bondConfig.Name)

		// 重启网络接口
		if err := bi.reloadBondInterface(bondConfig.Name); err != nil {
			utils.Warnf("bond", "Failed to reload bond interface %s: %v", bondConfig.Name, err)
			// 不返回错误，因为配置文件已经更新
		}
	} else {
		utils.Infof("bond", "KylinOS ifupdown bond config for %s unchanged", bondConfig.Name)
	}

	return changed, nil
}



// generateBondConfig 生成Bond配置内容
func (bi *KylinOSBondIfupdown) generateBondConfig(bondConfig types.BondConfig) (string, error) {
	// 读取模板文件
	tmplPath := "/etc/nix-operator/templates/kylinos_bond_ifcfg.tpl"
	tmplContent, err := bi.getTemplateContent(tmplPath)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("kylinos_bond_ifcfg").Funcs(template.FuncMap{
		"splitCIDR": func(cidr string) []string {
			if cidr == "" {
				return []string{"", ""}
			}
			parts := strings.Split(cidr, "/")
			if len(parts) != 2 {
				return []string{cidr, ""}
			}
			return parts
		},
		"cidrToNetmask": func(prefixLen string) string {
			if prefixLen == "" {
				return ""
			}
			len, err := strconv.Atoi(prefixLen)
			if err != nil {
				return ""
			}
			mask := net.CIDRMask(len, 32)
			return net.IP(mask).String()
		},
		"isTruthy": bi.isTruthy,
	}).Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	// 准备模板数据
	templateData := map[string]interface{}{
		"BondConfig": bondConfig,
	}

	// 渲染模板
	var result strings.Builder
	if err := tmpl.Execute(&result, templateData); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return result.String(), nil
}

// getTemplateContent 获取模板内容
func (bi *KylinOSBondIfupdown) getTemplateContent(tmplPath string) (string, error) {
	// 优先从外部文件读取
	if _, err := os.Stat(tmplPath); err == nil {
		content, err := os.ReadFile(tmplPath)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}

	// 如果外部模板文件不存在，使用embed嵌入的模板
	return kylinOSBondIfupdownTemplate, nil
}



// ReloadIfy 重新加载Bond配置
func (bi *KylinOSBondIfupdown) ReloadIfy(ctx context.Context) error {
	// KylinOS特定的Bond网络重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 1. 加载bonding内核模块
	cmd := exec.CommandContext(ctx, "modprobe", "bonding")
	if output, err := cmd.CombinedOutput(); err != nil {
		utils.Warnf("bond", "Failed to load bonding module: %v, output: %s", err, string(output))
	}

	// 2. 重启网络服务
	cmd = exec.CommandContext(ctx, "systemctl", "restart", "network")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 如果network服务不可用，尝试使用NetworkManager
		utils.Warnf("bond", "Failed to restart network service: %v, trying NetworkManager", err)
		cmd = exec.CommandContext(ctx, "systemctl", "restart", "NetworkManager")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to restart network services: %v, output: %s", err, string(output))
		}
	}

	// 3. 等待网络稳定
	time.Sleep(5 * time.Second)

	utils.Infof("bond", "KylinOS Bond Ifupdown configuration applied successfully")
	return nil
}



// reloadBondInterface 重新加载Bond接口
func (bi *KylinOSBondIfupdown) reloadBondInterface(bondName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 先停止接口
	cmd := exec.CommandContext(ctx, "ifdown", bondName)
	if err := cmd.Run(); err != nil {
		utils.Warnf("bond", "Failed to bring down bond interface %s: %v", bondName, err)
	}

	time.Sleep(2 * time.Second)

	// 再启动接口
	cmd = exec.CommandContext(ctx, "ifup", bondName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring up bond interface %s: %v", bondName, err)
	}

	return nil
}

// isTruthy 辅助函数，判断值是否为真
func (bi *KylinOSBondIfupdown) isTruthy(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case int, int8, int16, int32, int64:
		return v != 0
	case uint, uint8, uint16, uint32, uint64:
		return v != 0
	case float32:
		return v != 0.0
	case float64:
		return v != 0.0
	case []string:
		return len(v) > 0
	default:
		return value != nil
	}
}

// ensureTemplateDir 确保模板目录存在
func (bi *KylinOSBondIfupdown) ensureTemplateDir() error {
	templateDir := "/etc/nix-operator/templates"
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return fmt.Errorf("failed to create template directory: %v", err)
	}
	return nil
}

// installTemplate 安装模板文件
func (bi *KylinOSBondIfupdown) installTemplate() error {
	if err := bi.ensureTemplateDir(); err != nil {
		return err
	}

	tmplPath := "/etc/nix-operator/templates/kylinos_bond_ifcfg.tpl"
	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		// 模板文件不存在，创建它
		if err := utils.AtomicWriteFile([]byte(kylinOSBondIfupdownTemplate), tmplPath, 0644); err != nil {
			return fmt.Errorf("failed to write template file: %v", err)
		}
		utils.Infof("bond", "Installed KylinOS bond ifcfg template to %s", tmplPath)
	}
	return nil
}
