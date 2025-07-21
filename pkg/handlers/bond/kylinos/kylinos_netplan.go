package kylinos

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// KylinOSBondNetplan KylinOS Netplan Bond管理器
type KylinOSBondNetplan struct {
	osInfo   *controller.OSInfo
	testMode bool
}

// NewKylinOSBondNetplan 创建KylinOS Netplan Bond实例
func NewKylinOSBondNetplan(osInfo *controller.OSInfo) *KylinOSBondNetplan {
	return &KylinOSBondNetplan{
		osInfo:   osInfo,
		testMode: false,
	}
}

// NewKylinOSBondNetplanForTest 创建测试用的KylinOS Netplan Bond实例
func NewKylinOSBondNetplanForTest(osInfo *controller.OSInfo) *KylinOSBondNetplan {
	return &KylinOSBondNetplan{
		osInfo:   osInfo,
		testMode: true,
	}
}

// IsInstall 检查netplan是否已安装
func (np *KylinOSBondNetplan) IsInstall(ctx context.Context) bool {
	if np.testMode {
		return true
	}

	// 检查netplan命令是否存在
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "which", "netplan")
	if err := cmd.Run(); err != nil {
		return false
	}

	// 检查netplan配置目录是否存在
	if _, err := os.Stat("/etc/netplan"); err != nil {
		return false
	}

	return true
}

// Configure 配置Bond接口
func (np *KylinOSBondNetplan) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := np.ConfigureWithCheck(ctx, bondConfig)
	return err
}

// ConfigureWithCheck 配置Bond接口并检查是否有变更
func (np *KylinOSBondNetplan) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	if np.testMode {
		return np.mockConfigureWithCheck(ctx, bondConfig)
	}

	utils.Infof("bond", "Configuring KylinOS bond interface %s with Netplan", bondConfig.Name)

	// 生成配置内容
	configContent, err := np.generateBondConfig(bondConfig)
	if err != nil {
		return false, fmt.Errorf("failed to generate bond config: %v", err)
	}

	// 配置文件路径
	configPath := fmt.Sprintf("/etc/netplan/50-nix-operator-bond-%s.yaml", bondConfig.Name)

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
		utils.Infof("bond", "KylinOS Netplan bond config for %s updated", bondConfig.Name)

		// 应用netplan配置
		if err := np.applyNetplanConfig(ctx); err != nil {
			utils.Warnf("bond", "Failed to apply netplan config: %v", err)
			// 不返回错误，因为配置文件已经更新
		}
	} else {
		utils.Infof("bond", "KylinOS Netplan bond config for %s unchanged", bondConfig.Name)
	}

	return changed, nil
}

// mockConfigureWithCheck 模拟配置Bond接口（测试模式）
func (np *KylinOSBondNetplan) mockConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	// 验证Bond名称
	if bondConfig.Name == "" {
		return false, fmt.Errorf("bond name cannot be empty")
	}

	// 验证IPv4地址格式
	if bondConfig.Network.IP != "" {
		if _, _, err := net.ParseCIDR(bondConfig.Network.IP); err != nil {
			return false, fmt.Errorf("invalid IPv4 CIDR format: %s", bondConfig.Network.IP)
		}
	}

	utils.Infof("bond", "[TEST MODE] Configuring KylinOS bond interface %s with Netplan", bondConfig.Name)

	// 生成配置内容进行验证
	_, err := np.generateBondConfig(bondConfig)
	if err != nil {
		return false, fmt.Errorf("failed to generate bond config: %v", err)
	}

	utils.Infof("bond", "[TEST MODE] KylinOS Netplan bond config for %s would be updated", bondConfig.Name)
	return true, nil
}

// generateBondConfig 生成Bond配置内容
func (np *KylinOSBondNetplan) generateBondConfig(bondConfig types.BondConfig) (string, error) {
	// 读取模板文件
	tmplPath := "/etc/nix-operator/templates/kylinos_bond_netplan.tpl"
	tmplContent, err := np.getTemplateContent(tmplPath)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("kylinos_bond_netplan").Funcs(template.FuncMap{
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
		"getBondModeString": func(mode int) string {
			modeMap := map[int]string{
				0: "balance-rr",
				1: "active-backup",
				2: "balance-xor",
				3: "broadcast",
				4: "802.3ad",
				5: "balance-tlb",
				6: "balance-alb",
			}
			if modeStr, ok := modeMap[mode]; ok {
				return modeStr
			}
			return "active-backup"
		},
		"isTruthy": np.isTruthy,
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
func (np *KylinOSBondNetplan) getTemplateContent(tmplPath string) (string, error) {
	if np.testMode {
		// 测试模式下使用内嵌模板
		return np.getEmbeddedTemplate(), nil
	}

	// 生产模式下从文件读取
	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		// 如果模板文件不存在，使用内嵌模板
		return np.getEmbeddedTemplate(), nil
	}

	content, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// getEmbeddedTemplate 获取内嵌模板
func (np *KylinOSBondNetplan) getEmbeddedTemplate() string {
	return `# KylinOS Netplan Bond Configuration for {{.BondConfig.Name}}
# Generated by nix-operator
# Template: kylinos_bond_netplan.tpl

network:
  version: 2
  renderer: networkd
  bonds:
    {{.BondConfig.Name}}:
      parameters:
        mode: {{getBondModeString .BondConfig.Mode}}
{{if .BondConfig.Miimon}}        mii-monitor-interval: {{.BondConfig.Miimon}}
{{end}}{{range $key, $value := .BondConfig.Options.ExtraOptions}}        {{$key}}: {{$value}}
{{end}}{{if .BondConfig.Network.IP}}      addresses:
        - {{.BondConfig.Network.IP}}
{{end}}{{if .BondConfig.Network.Gateway}}      gateway4: {{.BondConfig.Network.Gateway}}
{{end}}{{if .BondConfig.Network.DNSServers}}      nameservers:
        addresses:
{{range .BondConfig.Network.DNSServers}}          - {{.}}
{{end}}{{end}}{{if .BondConfig.Network.MTU}}      mtu: {{.BondConfig.Network.MTU}}
{{end}}      dhcp4: {{if .BondConfig.Network.IP}}false{{else}}true{{end}}
      dhcp6: false
`
}

// ReloadIfy 重新加载Bond配置
func (np *KylinOSBondNetplan) ReloadIfy(ctx context.Context) error {
	if np.testMode {
		return np.mockReloadIfy()
	}

	// KylinOS特定的Bond Netplan重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 1. 加载bonding内核模块
	cmd := exec.CommandContext(ctx, "modprobe", "bonding")
	if output, err := cmd.CombinedOutput(); err != nil {
		utils.Warnf("bond", "Failed to load bonding module: %v, output: %s", err, string(output))
	}

	// 应用netplan配置
	if err := np.applyNetplanConfig(ctx); err != nil {
		return fmt.Errorf("failed to apply netplan configuration: %v", err)
	}

	// 3. 等待网络稳定
	time.Sleep(5 * time.Second)

	utils.Infof("bond", "KylinOS Bond Netplan configuration applied successfully")
	return nil
}

// mockReloadIfy 模拟重载Bond配置（测试模式）
func (np *KylinOSBondNetplan) mockReloadIfy() error {
	utils.Infof("bond", "[TEST MODE] Reloading KylinOS bond Netplan configuration")
	return nil
}

// applyNetplanConfig 应用netplan配置
func (np *KylinOSBondNetplan) applyNetplanConfig(ctx context.Context) error {
	// 验证netplan配置
	cmd := exec.CommandContext(ctx, "netplan", "try", "--timeout=10")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to validate netplan config: %v", err)
	}

	// 应用netplan配置
	cmd = exec.CommandContext(ctx, "netplan", "apply")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply netplan config: %v", err)
	}

	return nil
}

// isTruthy 辅助函数，判断值是否为真
func (np *KylinOSBondNetplan) isTruthy(value interface{}) bool {
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
func (np *KylinOSBondNetplan) ensureTemplateDir() error {
	templateDir := "/etc/nix-operator/templates"
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return fmt.Errorf("failed to create template directory: %v", err)
	}
	return nil
}

// installTemplate 安装模板文件
func (np *KylinOSBondNetplan) installTemplate() error {
	if err := np.ensureTemplateDir(); err != nil {
		return err
	}

	tmplPath := "/etc/nix-operator/templates/kylinos_bond_netplan.tpl"
	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		// 模板文件不存在，创建它
		tmplContent := np.getEmbeddedTemplate()
		if err := utils.AtomicWriteFile([]byte(tmplContent), tmplPath, 0644); err != nil {
			return fmt.Errorf("failed to write template file: %v", err)
		}
		utils.Infof("bond", "Installed KylinOS bond Netplan template to %s", tmplPath)
	}
	return nil
}
