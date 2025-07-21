package kylinos

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/handlers/bond/types"
	"go.xbrother.com/nix-operator/pkg/utils"
)

// KylinOSBondNetworkManager KylinOS NetworkManager Bond管理器
type KylinOSBondNetworkManager struct {
	osInfo   *controller.OSInfo
	testMode bool
}

// NewKylinOSBondNetworkManager 创建KylinOS NetworkManager Bond实例
func NewKylinOSBondNetworkManager(osInfo *controller.OSInfo) *KylinOSBondNetworkManager {
	return &KylinOSBondNetworkManager{
		osInfo:   osInfo,
		testMode: false,
	}
}

// NewKylinOSBondNetworkManagerForTest 创建测试用的KylinOS NetworkManager Bond实例
func NewKylinOSBondNetworkManagerForTest(osInfo *controller.OSInfo) *KylinOSBondNetworkManager {
	return &KylinOSBondNetworkManager{
		osInfo:   osInfo,
		testMode: true,
	}
}

// IsInstall 检查NetworkManager是否已安装
func (nm *KylinOSBondNetworkManager) IsInstall(ctx context.Context) bool {
	if nm.testMode {
		return true
	}

	// 检查NetworkManager服务是否运行
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctxTimeout, "systemctl", "is-active", "NetworkManager")
	if err := cmd.Run(); err != nil {
		return false
	}

	// 检查nmcli命令是否存在
	cmd = exec.CommandContext(ctxTimeout, "which", "nmcli")
	if err := cmd.Run(); err != nil {
		return false
	}

	// 检查NetworkManager配置目录是否存在
	if _, err := os.Stat("/etc/NetworkManager/system-connections"); err != nil {
		return false
	}

	return true
}

// Configure 配置Bond接口
func (nm *KylinOSBondNetworkManager) Configure(ctx context.Context, bondConfig types.BondConfig) error {
	_, err := nm.ConfigureWithCheck(ctx, bondConfig)
	return err
}

// ConfigureWithCheck 配置Bond接口并检查是否有变更
func (nm *KylinOSBondNetworkManager) ConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
	if nm.testMode {
		return nm.mockConfigureWithCheck(ctx, bondConfig)
	}

	utils.Infof("bond", "Configuring KylinOS bond interface %s with NetworkManager", bondConfig.Name)

	// 生成配置内容
	configContent, err := nm.generateBondConfig(bondConfig)
	if err != nil {
		return false, fmt.Errorf("failed to generate bond config: %v", err)
	}

	// 配置文件路径
	configPath := fmt.Sprintf("/etc/NetworkManager/system-connections/%s.nmconnection", bondConfig.Name)

	// 检查配置文件是否存在以及内容是否相同
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		// 文件存在，比较内容
		if string(existingData) == configContent {
			return false, nil // 配置未变更
		}
	}

	// 写入配置文件
	if err := utils.AtomicWriteFile([]byte(configContent), configPath, 0600); err != nil {
		return false, fmt.Errorf("failed to write bond NetworkManager config: %v", err)
	}

	changed := true

	if changed {
		utils.Infof("bond", "KylinOS NetworkManager bond config for %s updated", bondConfig.Name)

		// 重新加载NetworkManager配置
		if err := nm.reloadNetworkManager(ctx); err != nil {
			utils.Warnf("bond", "Failed to reload NetworkManager: %v", err)
			// 不返回错误，因为配置文件已经更新
		}
	} else {
		utils.Infof("bond", "KylinOS NetworkManager bond config for %s unchanged", bondConfig.Name)
	}

	return changed, nil
}

// mockConfigureWithCheck 模拟配置Bond接口（测试模式）
func (nm *KylinOSBondNetworkManager) mockConfigureWithCheck(ctx context.Context, bondConfig types.BondConfig) (bool, error) {
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

	utils.Infof("bond", "[TEST MODE] Configuring KylinOS bond interface %s with NetworkManager", bondConfig.Name)

	// 生成配置内容进行验证
	_, err := nm.generateBondConfig(bondConfig)
	if err != nil {
		return false, fmt.Errorf("failed to generate bond config: %v", err)
	}

	utils.Infof("bond", "[TEST MODE] KylinOS NetworkManager bond config for %s would be updated", bondConfig.Name)
	return true, nil
}

// generateBondConfig 生成Bond配置内容
func (nm *KylinOSBondNetworkManager) generateBondConfig(bondConfig types.BondConfig) (string, error) {
	// 读取模板文件
	tmplPath := "/etc/nix-operator/templates/kylinos_bond_nmconnection.tpl"
	tmplContent, err := nm.getTemplateContent(tmplPath)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %v", err)
	}

	// 解析模板
	tmpl, err := template.New("kylinos_bond_nmconnection").Funcs(template.FuncMap{
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
		"generateUUID": func() string {
			return uuid.New().String()
		},
		"isTruthy": nm.isTruthy,
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
func (nm *KylinOSBondNetworkManager) getTemplateContent(tmplPath string) (string, error) {
	if nm.testMode {
		// 测试模式下使用内嵌模板
		return nm.getEmbeddedTemplate(), nil
	}

	// 生产模式下从文件读取
	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		// 如果模板文件不存在，使用内嵌模板
		return nm.getEmbeddedTemplate(), nil
	}

	content, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// getEmbeddedTemplate 获取内嵌模板
func (nm *KylinOSBondNetworkManager) getEmbeddedTemplate() string {
	return `# KylinOS NetworkManager Bond Configuration for {{.BondConfig.Name}}
# Generated by nix-operator
# Template: kylinos_bond_nmconnection.tpl

[connection]
id={{.BondConfig.Name}}
uuid={{generateUUID}}
type=bond
interface-name={{.BondConfig.Name}}
autoconnect=true

[bond]
mode={{.BondConfig.Mode}}
{{if .BondConfig.Miimon}}miimon={{.BondConfig.Miimon}}{{end}}
{{range $key, $value := .BondConfig.Options.ExtraOptions}}{{$key}}={{$value}}
{{end}}

{{if .BondConfig.Network.IP}}[ipv4]
method=manual
{{$ipParts := splitCIDR .BondConfig.Network.IP}}address1={{index $ipParts 0}}/{{index $ipParts 1}}
{{if .BondConfig.Network.Gateway}}gateway={{.BondConfig.Network.Gateway}}{{end}}
{{if .BondConfig.Network.DNSServers}}dns={{range $i, $dns := .BondConfig.Network.DNSServers}}{{if $i}};{{end}}{{$dns}}{{end}};{{end}}
{{else}}[ipv4]
method=auto
{{end}}

[ipv6]
method=ignore

{{if .BondConfig.Network.MTU}}[ethernet]
mtu={{.BondConfig.Network.MTU}}
{{end}}
`
}

// ReloadIfy 重新加载Bond配置
func (nm *KylinOSBondNetworkManager) ReloadIfy(ctx context.Context) error {
	if nm.testMode {
		return nm.mockReloadIfy()
	}

	// KylinOS特定的Bond NetworkManager重载逻辑
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 1. 加载bonding内核模块
	cmd := exec.CommandContext(ctx, "modprobe", "bonding")
	if output, err := cmd.CombinedOutput(); err != nil {
		utils.Warnf("bond", "Failed to load bonding module: %v, output: %s", err, string(output))
	}

	// 2. 重新加载NetworkManager配置
	if err := nm.reloadNetworkManager(ctx); err != nil {
		return fmt.Errorf("failed to reload NetworkManager: %v", err)
	}

	// 3. 等待网络稳定
	time.Sleep(5 * time.Second)

	utils.Infof("bond", "KylinOS Bond NetworkManager configuration applied successfully")
	return nil
}

// mockReloadIfy 模拟重载Bond配置（测试模式）
func (nm *KylinOSBondNetworkManager) mockReloadIfy() error {
	utils.Infof("bond", "[TEST MODE] Reloading KylinOS bond NetworkManager configuration")
	return nil
}

// reloadNetworkManager 重新加载NetworkManager
func (nm *KylinOSBondNetworkManager) reloadNetworkManager(ctx context.Context) error {
	// 重新加载NetworkManager配置
	cmd := exec.CommandContext(ctx, "nmcli", "connection", "reload")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to reload NetworkManager connections: %v", err)
	}

	return nil
}

// isTruthy 辅助函数，判断值是否为真
func (nm *KylinOSBondNetworkManager) isTruthy(value interface{}) bool {
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
func (nm *KylinOSBondNetworkManager) ensureTemplateDir() error {
	templateDir := "/etc/nix-operator/templates"
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return fmt.Errorf("failed to create template directory: %v", err)
	}
	return nil
}

// installTemplate 安装模板文件
func (nm *KylinOSBondNetworkManager) installTemplate() error {
	if err := nm.ensureTemplateDir(); err != nil {
		return err
	}

	tmplPath := "/etc/nix-operator/templates/kylinos_bond_nmconnection.tpl"
	if _, err := os.Stat(tmplPath); os.IsNotExist(err) {
		// 模板文件不存在，创建它
		tmplContent := nm.getEmbeddedTemplate()
		if err := utils.AtomicWriteFile([]byte(tmplContent), tmplPath, 0644); err != nil {
			return fmt.Errorf("failed to write template file: %v", err)
		}
		utils.Infof("bond", "Installed KylinOS bond NetworkManager template to %s", tmplPath)
	}
	return nil
}
