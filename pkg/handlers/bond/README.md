# Bond Handler

## 概述

Bond Handler 是 nix-operator 中用于管理网络绑定（Bond）配置的处理器。它支持在 CentOS 7.2-7.9 系统上配置网络绑定，提供高可用性和负载均衡功能。

## 支持的操作系统

- **CentOS 7.2-7.9**: 完全支持，包括 NetworkManager 和传统 ifcfg 脚本

## 功能特性

### 1. 多种绑定模式支持
- **Mode 0 (balance-rr)**: 轮询负载均衡
- **Mode 1 (active-backup)**: 主备模式
- **Mode 2 (balance-xor)**: XOR 哈希负载均衡
- **Mode 3 (broadcast)**: 广播模式
- **Mode 4 (802.3ad/LACP)**: 动态链路聚合
- **Mode 5 (balance-tlb)**: 传输负载均衡
- **Mode 6 (balance-alb)**: 自适应负载均衡

### 2. 网络管理器支持
- **NetworkManager**: 现代 CentOS 系统的首选方案
- **传统 ifcfg 脚本**: 兼容旧版本和特殊环境

### 3. 完整的网络配置
- 静态 IP 地址配置
- 网关设置
- DNS 服务器配置
- MTU 设置
- 高级绑定选项

## 配置示例

### 基本配置

```json
{
  "apiVersion": "system/v1",
  "kind": "BondConfiguration",
  "metadata": {
    "name": "bond-config"
  },
  "spec": {
    "name": "bond0",
    "mode": 1,
    "miimon": 100,
    "network": {
      "ip": "192.168.1.100/24",
      "gateway": "192.168.1.1",
      "dnsServers": ["8.8.8.8", "8.8.4.4"],
      "mtu": 1500
    },
    "options": {
      "downdelay": 200,
      "updelay": 200,
      "extraOptions": {
        "primary": "eth0",
        "primary_reselect": "always"
      }
    }
  }
}
```

### LACP 模式配置

```json
{
  "apiVersion": "system/v1",
  "kind": "BondConfiguration",
  "metadata": {
    "name": "bond-lacp"
  },
  "spec": {
    "name": "bond1",
    "mode": 4,
    "miimon": 100,
    "network": {
      "ip": "10.0.1.100/24",
      "gateway": "10.0.1.1",
      "dnsServers": ["10.0.1.1"],
      "mtu": 9000
    },
    "options": {
      "extraOptions": {
        "lacp_rate": "fast",
        "xmit_hash_policy": "layer3+4"
      }
    }
  }
}
```

## 架构设计

### 处理器层次结构

```
BondConfiguration
├── CentOSBondHandler (CentOS 7.2-7.9)
│   ├── CentOSBondNetworkManager
│   └── CentOSBondIfupdown
└── [未来可扩展其他操作系统]
```

### 组件说明

1. **CentOSBondHandler**: 主处理器，负责操作系统匹配和管理器选择
2. **CentOSBondNetworkManager**: NetworkManager 实现，生成 `.nmconnection` 文件
3. **CentOSBondIfupdown**: 传统网络脚本实现，生成 `ifcfg-*` 文件

## 使用方法

### 1. 配置文件放置

将 Bond 配置文件放置在 `/root/workspace/new/nix-operator/etc/cr.d/` 目录下：

```bash
# 示例配置文件
cp bond-config.json /root/workspace/new/nix-operator/etc/cr.d/
```

### 2. 自动检测和应用

nix-operator 会自动：
1. 检测操作系统类型和版本
2. 选择合适的网络管理器
3. 生成相应的配置文件
4. 重新加载网络配置

### 3. 验证配置

```bash
# 检查绑定接口状态
cat /proc/net/bonding/bond0

# 检查网络连接
ip addr show bond0
ping -c 3 192.168.1.1
```

## 故障排除

### 常见问题

1. **绑定接口未创建**
   - 检查配置文件语法
   - 确认物理网卡存在
   - 查看系统日志

2. **网络连接失败**
   - 验证 IP 地址配置
   - 检查网关设置
   - 确认 DNS 配置

3. **绑定模式不工作**
   - 确认交换机支持相应模式
   - 检查 LACP 配置（模式 4）
   - 验证物理连接

### 日志查看

```bash
# 查看 nix-operator 日志
journalctl -u nix-operator -f

# 查看 NetworkManager 日志
journalctl -u NetworkManager -f

# 查看系统网络日志
dmesg | grep -i bond
```

## 扩展性

### 添加新操作系统支持

1. 创建新的处理器目录：`pkg/handlers/bond/ubuntu/`
2. 实现 `IBondManager` 接口
3. 在 `init.go` 中注册处理器

### 添加新的网络管理器

1. 实现 `IBondManager` 接口
2. 添加检测逻辑
3. 更新处理器优先级

## 安全考虑

- 配置文件权限应设置为 `644`
- 避免在配置中包含敏感信息
- 定期检查和更新绑定配置
- 监控网络流量和性能

## 性能优化

- 根据网络环境选择合适的绑定模式
- 调整 `miimon` 值以平衡检测速度和系统负载
- 配置合适的 MTU 值
- 使用 LACP 模式时优化交换机配置