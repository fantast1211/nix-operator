# KylinOS 网络配置处理器

## 概述

KylinOS 网络配置处理器是专为 KylinOS 系统设计的网络配置管理组件，实现了"服务化"方式的网络配置能力。该处理器支持多种网络管理器，并能够根据系统环境自动选择最适合的网络管理器。

## 功能特性

### 支持的网络管理器

1. **ifupdown**（优先级：1 - 最高）
   - 传统的 Linux 网络配置方式
   - 使用 `/etc/network/interfaces` 配置文件
   - 适用于传统的 KylinOS 系统

2. **NetworkManager**（优先级：2 - 中等）
   - 现代化的网络管理服务
   - 使用 `.nmconnection` 配置文件
   - 支持动态网络管理

3. **Netplan**（优先级：3 - 最低）
   - 基于 YAML 的网络配置
   - 支持 systemd-networkd 和 NetworkManager 作为后端
   - 适用于较新的 KylinOS 版本

### 网络管理器选择策略

处理器按照以下优先级自动检测和选择网络管理器：

```
ifupdown (1) > NetworkManager (2) > Netplan (3)
```

- 系统会依次检查每个网络管理器是否已安装和可用
- 一旦找到可用的网络管理器，就会使用该管理器进行配置
- 如果所有网络管理器都不可用，会返回错误

### 模板渲染功能

- **编译时嵌入**：模板文件在编译时通过 `embed` 指令嵌入到二进制文件中
- **外部模板支持**：支持从外部文件加载模板，具有更高的优先级
- **模板路径**：
  - ifupdown: `/etc/nix-operator/templates/kylinos_ifupdown.tpl`
  - NetworkManager: `/etc/nix-operator/templates/kylinos_nmconnection.tpl`
  - Netplan: `/etc/nix-operator/templates/kylinos_netplan.tpl`

## 支持的网络配置

### 基本网络接口配置

- IPv4 静态地址配置
- IPv6 静态地址配置
- DHCP 自动配置
- 网关设置
- DNS 服务器配置
- MTU 设置

### 高级网络功能

- **网络绑定（Bonding）**：支持主备模式的网络绑定
- **多接口配置**：可同时配置多个网络接口
- **混合协议栈**：支持 IPv4 和 IPv6 双栈配置

## 配置示例

### 基本静态 IP 配置

```yaml
apiVersion: system.nix-operator.io/v1
kind: ResourceConfig
metadata:
  name: kylin-network-basic
  namespace: default
spec:
  interfaces:
    - name: eth0
      ipv4Address: "192.168.1.100/24"
      ipv4Gateway: "192.168.1.1"
      mtu: 1500
      nameservers:
        - "8.8.8.8"
        - "8.8.4.4"
```

### IPv6 双栈配置

```yaml
apiVersion: system.nix-operator.io/v1
kind: ResourceConfig
metadata:
  name: kylin-network-ipv6
  namespace: default
spec:
  interfaces:
    - name: eth0
      ipv4Address: "192.168.1.100/24"
      ipv6Address: "2001:db8::100/64"
      ipv4Gateway: "192.168.1.1"
      ipv6Gateway: "2001:db8::1"
      mtu: 1500
      nameservers:
        - "8.8.8.8"
        - "2001:4860:4860::8888"
```

### 网络绑定配置

```yaml
apiVersion: system.nix-operator.io/v1
kind: ResourceConfig
metadata:
  name: kylin-network-bonding
  namespace: default
spec:
  interfaces:
    - name: eth0
      bondingSlave:
        enabled: true
        master: bond0
    - name: eth1
      bondingSlave:
        enabled: true
        master: bond0
```

### 多接口配置

```yaml
apiVersion: system.nix-operator.io/v1
kind: ResourceConfig
metadata:
  name: kylin-network-multi
  namespace: default
spec:
  interfaces:
    - name: eth0
      ipv4Address: "192.168.1.100/24"
      ipv4Gateway: "192.168.1.1"
      mtu: 1500
    - name: eth1
      ipv4Address: "10.0.0.100/8"
      ipv4Gateway: "10.0.0.1"
      mtu: 9000
```

## 测试模式

### 测试设计原则

- **无真实变更**：测试模式下不会真实修改网络配置，避免 SSH 连接断开
- **模拟环境**：通过模拟系统环境和网络管理器行为完成测试
- **独立测试**：测试不依赖真实运行环境，可在任何环境下执行

### 测试用例覆盖

1. **系统匹配测试**：验证处理器是否正确识别 KylinOS 系统
2. **网络管理器检测测试**：验证网络管理器的检测和选择逻辑
3. **配置验证测试**：验证网络接口配置的有效性检查
4. **模板渲染测试**：验证各种网络管理器的配置模板渲染
5. **调谐流程测试**：验证完整的网络配置调谐流程

### 运行测试

```bash
# 运行所有测试
go test ./pkg/handlers/network/kylinos/

# 运行特定测试
go test -run TestKylinOSNetworkHandler_Match ./pkg/handlers/network/kylinos/

# 运行示例测试
go test -run Example ./pkg/handlers/network/kylinos/

# 详细输出
go test -v ./pkg/handlers/network/kylinos/
```

## 文件结构

```
kylinos/
├── kylinos.go                    # 主处理器实现
├── kylinos_ifupdown.go           # ifupdown 网络管理器
├── kylinos_ifupdown.tpl          # ifupdown 配置模板
├── kylinos_networkmanager.go     # NetworkManager 网络管理器
├── kylinos_nmconnection.tpl      # NetworkManager 配置模板
├── kylinos_netplan.go            # Netplan 网络管理器
├── kylinos_netplan.tpl           # Netplan 配置模板
├── kylinos_test.go               # 主要测试用例
├── example_test.go               # 示例测试用例
└── README_KYLINOS.md             # 本文档
```

## 配置文件位置

### ifupdown
- 配置目录：`/etc/network/interfaces.d/`
- 配置文件：`/etc/network/interfaces.d/{interface_name}`
- 主配置：`/etc/network/interfaces`

### NetworkManager
- 配置目录：`/etc/NetworkManager/system-connections/`
- 配置文件：`/etc/NetworkManager/system-connections/{interface_name}.nmconnection`
- 权限：`0600`

### Netplan
- 配置目录：`/etc/netplan/`
- 配置文件：`/etc/netplan/50-nix-operator-{interface_name}.yaml`
- 权限：`0644`

## 日志和调试

### 日志级别

- **Info**：正常操作信息
- **Debug**：详细调试信息
- **Error**：错误信息

### 日志示例

```
[INFO] network: KylinOS V10 network handler matched
[INFO] network: Initialized KylinOS V10 network managers with priority: ifupdown > NetworkManager > Netplan
[INFO] network: Selected KylinOS network manager: *kylinos.KylinOSIfupdown on kylin V10
[INFO] network: Configuring KylinOS interface eth0 with ifupdown
[INFO] network: KylinOS ifupdown config for eth0 updated
[INFO] network: KylinOS network configuration changed, reloading network services
```

## 故障排除

### 常见问题

1. **网络管理器未找到**
   - 检查系统是否安装了支持的网络管理器
   - 确认网络管理器服务是否启用

2. **配置验证失败**
   - 检查接口名称格式是否正确
   - 验证 IP 地址和 CIDR 格式
   - 确认 MTU 值在有效范围内（68-9000）

3. **模板渲染错误**
   - 检查模板文件语法
   - 确认模板数据结构正确

4. **网络服务重载失败**
   - 检查系统服务状态
   - 确认有足够的权限执行网络操作

### 调试步骤

1. 启用详细日志输出
2. 检查系统环境和网络管理器状态
3. 验证配置文件内容和权限
4. 测试网络服务重载命令

## 兼容性

### 支持的 KylinOS 版本

- KylinOS V10
- KylinOS Server
- 其他基于 Kylin 的发行版

### 系统要求

- Linux 内核 3.10+
- systemd 支持
- 至少一个支持的网络管理器

## 扩展和定制

### 添加新的网络管理器

1. 实现 `types.INetworkManager` 接口
2. 创建对应的配置模板
3. 在主处理器中注册新的管理器
4. 添加相应的测试用例

### 自定义模板

1. 将自定义模板放置在指定的外部路径
2. 模板会自动覆盖内嵌的默认模板
3. 支持 Go 模板语法和函数

## 贡献指南

1. 遵循现有的代码风格和结构
2. 添加充分的测试用例
3. 更新相关文档
4. 确保向后兼容性