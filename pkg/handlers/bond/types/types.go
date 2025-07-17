package types

import "context"

// IBondManager Bond管理器接口
type IBondManager interface {
	// IsInstall 检查Bond管理器是否已安装并可用
	IsInstall(ctx context.Context) bool
	// Configure 配置Bond接口
	Configure(ctx context.Context, bondConfig BondConfig) error
	// ConfigureWithCheck 配置Bond接口并检查是否有变更
	ConfigureWithCheck(ctx context.Context, bondConfig BondConfig) (bool, error)
	// ReloadIfy 重新加载Bond配置
	ReloadIfy(ctx context.Context) error
}

// BondConfig Bond配置结构
type BondConfig struct {
	Name    string            `json:"name" yaml:"name,omitempty"`       // Bond接口名称（如bond0）
	Mode    int               `json:"mode" yaml:"mode,omitempty"`       // Bond模式（0-7）
	Miimon  int               `json:"miimon" yaml:"miimon,omitempty"`   // 链路监测间隔（毫秒）
	Network BondNetworkConfig `json:"network" yaml:"network,omitempty"` // 网络配置
	Options BondOptions       `json:"options" yaml:"options,omitempty"` // 可选参数
}

// BondNetworkConfig Bond网络配置
type BondNetworkConfig struct {
	IP         string   `json:"ip,omitempty" yaml:"ip,omitempty"`                 // IP地址（支持CIDR格式）
	Gateway    string   `json:"gateway,omitempty" yaml:"gateway,omitempty"`       // 网关地址
	DNSServers []string `json:"dnsServers,omitempty" yaml:"dnsServers,omitempty"` // DNS服务器列表
	MTU        int      `json:"mtu,omitempty" yaml:"mtu,omitempty"`               // MTU设置
}

// BondOptions Bond可选参数
type BondOptions struct {
	ExtraOptions map[string]string `json:"extraOptions,omitempty" yaml:"extraOptions,omitempty"` // 其他可扩展选项
}

// BondModeNames Bond模式名称映射
var BondModeNames = map[int]string{
	0: "balance-rr",    // 轮询模式
	1: "active-backup", // 主备模式
	2: "balance-xor",   // XOR模式
	3: "broadcast",     // 广播模式
	4: "802.3ad",       // LACP模式
	5: "balance-tlb",   // 传输负载均衡模式
	6: "balance-alb",   // 自适应负载均衡模式
	7: "custom",        // 自定义模式
}

// GetBondModeName 获取Bond模式名称
func GetBondModeName(mode int) string {
	if name, exists := BondModeNames[mode]; exists {
		return name
	}
	return "unknown"
}
