# 网络配置插件

## 概述

网络配置插件为 nix-operator 提供了 Linux 系统网络接口的配置管理功能。该插件支持多种网络管理器，包括 NetworkManager、Netplan 和 ifupdown，能够自动检测系统中可用的网络管理器并选择最合适的进行配置。

## 支持的网络管理器

### 1. NetworkManager
- **优先级**: 最高
- **检测方式**: 检查 NetworkManager 服务是否运行
- **配置文件**: `/etc/NetworkManager/system-connections/nix-operator-{interface}.nmconnection`
- **重载方式**: `nmcli connection reload`

### 2. Netplan
- **优先级**: 中等
- **检测方式**: 检查 `netplan` 命令是否存在
- **配置文件**: `/etc/netplan/50-nix-operator-{interface}.yaml`
- **重载方式**: `netplan apply`

### 3. ifupdown
- **优先级**: 最低
- **检测方式**: 检查 `/etc/network/interfaces` 文件和 `ifup` 命令
- **配置文件**: `/etc/network/interfaces`
- **重载方式**: `systemctl restart networking` 或 `service networking restart`

## 配置规格

### NetworkConfigurationSpec

```json
{
  "nodeSelector": {
    "machineId": "机器ID（可选）",
    "ip": "IP地址（可选）"
  },
  "interfaces": [
    {
      "name": "网络接口名称（必需）",
      "ipv4Address": "IPv4地址/CIDR（可选）",
      "ipv6Address": "IPv6地址/前缀长度（可选）",
      "ipv4Gateway": "IPv4网关（可选）",
      "ipv6Gateway": "IPv6网关（可选）",
      "mtu": "MTU大小（可选，默认1500）",
      "nameservers": ["DNS服务器列表（可选）"]
    }
  ]
}
```

### 字段说明

- **nodeSelector**: 节点选择器，用于指定配置应用的目标节点
  - `machineId`: 机器ID匹配
  - `ip`: IP地址匹配
- **interfaces**: 网络接口配置数组，支持配置多个网络接口
  - **name**: 网络接口名称（如 eth0, enp0s3）
  - **ipv4Address**: IPv4地址，CIDR格式（如 192.168.1.100/24）
  - **ipv6Address**: IPv6地址，带前缀长度（如 2001:db8::1/64）
  - **ipv4Gateway**: IPv4网关地址
  - **ipv6Gateway**: IPv6网关地址
  - **mtu**: 最大传输单元，范围 576-9000
  - **nameservers**: DNS服务器地址列表

## 配置示例

### 单接口静态IP配置

```json
{
  "apiVersion": "system/v1",
  "kind": "NetworkConfiguration",
  "metadata": {
    "name": "eth0-static",
    "namespace": "default"
  },
  "spec": {
    "interfaces": [
      {
        "name": "eth0",
        "ipv4Address": "192.168.1.100/24",
        "ipv4Gateway": "192.168.1.1",
        "nameservers": [
          "8.8.8.8",
          "8.8.4.4"
        ]
      }
    ]
  }
}
```

### 多接口配置

```json
{
  "apiVersion": "system/v1",
  "kind": "NetworkConfiguration",
  "metadata": {
    "name": "multi-interface-config",
    "namespace": "default"
  },
  "spec": {
    "interfaces": [
      {
        "name": "eth0",
        "ipv4Address": "192.168.1.100/24",
        "ipv4Gateway": "192.168.1.1",
        "mtu": 1500,
        "nameservers": [
          "8.8.8.8",
          "8.8.4.4"
        ]
      },
      {
        "name": "eth1",
        "ipv4Address": "10.0.1.100/24",
        "ipv4Gateway": "10.0.1.1",
        "mtu": 9000,
        "nameservers": [
          "10.0.1.1",
          "1.1.1.1"
        ]
      }
    ]
  }
}
```

### 带节点选择器的配置

```json
{
  "apiVersion": "system/v1",
  "kind": "NetworkConfiguration",
  "metadata": {
    "name": "server-network",
    "namespace": "default"
  },
  "spec": {
    "nodeSelector": {
      "machineId": "a1b2c3d4e5f6"
    },
    "interfaces": [
      {
        "name": "eth0",
        "ipv4Address": "10.0.1.10/24",
        "ipv4Gateway": "10.0.1.1",
        "mtu": 9000,
        "nameservers": [
          "10.0.1.1",
          "8.8.8.8"
        ]
      }
    ]
  }
}
```

### IPv6配置

```json
{
  "apiVersion": "system/v1",
  "kind": "NetworkConfiguration",
  "metadata": {
    "name": "ipv6-config",
    "namespace": "default"
  },
  "spec": {
    "interfaces": [
      {
        "name": "eth0",
        "ipv4Address": "192.168.1.100/24",
        "ipv6Address": "2001:db8::100/64",
        "ipv4Gateway": "192.168.1.1",
        "ipv6Gateway": "2001:db8::1",
        "nameservers": [
          "8.8.8.8",
          "2001:4860:4860::8888"
        ]
      }
    ]
  }
}
```

## 模板系统

插件使用 Go 模板系统生成配置文件，支持外部模板覆盖：

### 内置模板
- `netplan.tpl`: Netplan 配置模板
- `nmconnection.tpl`: NetworkManager 连接模板
- `ifupdown.tpl`: ifupdown 接口模板

### 外部模板目录
- 路径: `/etc/nix-operator/templates/`
- 优先级: 高于内置模板
- 用途: 支持特殊操作系统的临时配置调整

## 安全特性

1. **原子性写入**: 使用临时文件交换确保配置文件的原子性更新
2. **权限控制**: 配置文件使用适当的权限（NetworkManager 连接文件为 0600）
3. **超时控制**: 所有系统调用和子进程都有超时控制
4. **错误处理**: 完善的错误处理和回滚机制

## 故障排除

### 常见问题

1. **网络管理器检测失败**
   - 检查系统中是否安装了支持的网络管理器
   - 确认服务状态：`systemctl status NetworkManager`

2. **配置应用失败**
   - 检查配置文件语法是否正确
   - 查看系统日志：`journalctl -u nix-operator`

3. **权限问题**
   - 确保 nix-operator 以适当权限运行
   - 检查配置文件目录的写入权限

### 调试命令

```bash
# 检查网络管理器状态
systemctl status NetworkManager
which netplan
ls -la /etc/network/interfaces

# 手动测试配置
netplan try  # Netplan
nmcli connection reload  # NetworkManager
ifup eth0  # ifupdown
```

## 最佳实践

1. **节点选择器使用**: 为不同节点使用不同的配置时，合理使用节点选择器
2. **配置验证**: 在生产环境应用前，先在测试环境验证配置
3. **备份策略**: 在修改网络配置前备份原始配置
4. **监控**: 配置应用后监控网络连接状态
5. **渐进部署**: 对于多节点环境，建议渐进式部署配置

## API 集成

网络配置插件完全集成到 nix-operator 的 REST API 和 gRPC API 中：

- **REST API**: `/api/v1/configs` (GET, POST, PUT, DELETE)
- **gRPC API**: `ResourceService` 服务
- **JSON Schema**: 自动验证配置格式
- **状态报告**: 实时配置状态和错误信息