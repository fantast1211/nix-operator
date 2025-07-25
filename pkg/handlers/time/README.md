# Time Handler

## 概述

Time Handler 是 nix-operator 中用于管理系统时间配置的处理器。它支持在不同操作系统上配置时区和NTP服务，采用分层架构设计以适配不同操作系统的特殊需求。

## 支持的操作系统

- **Ubuntu**: 专用处理器，使用 `/etc/chrony/chrony.conf` 配置路径
- **通用Linux**: 兜底处理器，使用 `/etc/chrony.conf` 配置路径（适用于 OpenEuler、CentOS 等大多数Linux发行版）

## 架构设计

### 处理器层次结构

```
TimeConfiguration
├── UbuntuTimeHandler (Ubuntu 专用)
└── LinuxTimeHandler (通用 Linux 兜底处理器)
```

### 组件说明

1. **UbuntuTimeHandler**: Ubuntu专用处理器，处理Ubuntu系统的特殊配置路径
2. **LinuxTimeHandler**: 通用Linux处理器，适用于大多数Linux发行版
3. **init.go**: 统一的处理器注册管理

## 功能特性

### 1. 时区配置
- 支持标准时区设置（如 Asia/Shanghai）
- 使用 `timedatectl` 命令或符号链接方式
- 自动更新 `/etc/localtime` 和 `/etc/timezone`

### 2. NTP 配置
- 支持多个NTP服务器配置
- 自动生成 chrony 配置文件
- 支持服务重启和配置重载

### 3. 操作系统适配
- **Ubuntu**: 使用 `/etc/chrony/chrony.conf` 配置路径
- **其他Linux**: 使用 `/etc/chrony.conf` 配置路径
- 自动检测并选择合适的处理器

## 配置示例

### 基本时间配置

```json
{
  "apiVersion": "sysconfig.operator/v1",
  "kind": "TimeConfiguration",
  "metadata": {
    "name": "time-config"
  },
  "spec": {
    "@type": "type.googleapis.com/xtopus.api.system.v1.TimeConfigurationSpec",
    "timezone": "Asia/Shanghai",
    "ntp": {
      "enable": true,
      "servers": [
        "ntp1.aliyun.com",
        "ntp2.aliyun.com",
        "ntp3.aliyun.com"
      ]
    }
  }
}
```

### 仅时区配置

```json
{
  "apiVersion": "sysconfig.operator/v1",
  "kind": "TimeConfiguration",
  "metadata": {
    "name": "timezone-only"
  },
  "spec": {
    "@type": "type.googleapis.com/xtopus.api.system.v1.TimeConfigurationSpec",
    "timezone": "UTC"
  }
}
```

## 使用方法

### 1. 配置文件放置

将时间配置文件放置在 `etc/cr.d/` 目录下：

```bash
# 示例配置文件
cp time-config.json etc/cr.d/
```

### 2. 自动检测和应用

nix-operator 会自动：
1. 检测操作系统类型
2. 选择合适的时间处理器
3. 生成相应的配置文件
4. 重新加载时间服务

### 3. 验证配置

```bash
# 检查时区设置
timedatectl status

# 检查NTP同步状态
chronyc sources
chronyc tracking

# 检查配置文件
# Ubuntu:
cat /etc/chrony/chrony.conf
# 其他Linux:
cat /etc/chrony.conf
```

## 故障排除

### 常见问题

1. **时区设置失败**
   - 检查时区名称是否正确
   - 确认 `/usr/share/zoneinfo/` 目录存在
   - 查看系统日志

2. **NTP同步失败**
   - 验证NTP服务器地址
   - 检查网络连接
   - 确认chronyd服务状态

3. **配置文件未生成**
   - 检查文件权限
   - 确认chrony软件包已安装
   - 查看nix-operator日志

### 日志查看

```bash
# 查看 nix-operator 日志
journalctl -u nix-operator -f

# 查看 chronyd 日志
journalctl -u chronyd -f

# 查看系统时间日志
dmesg | grep -i time
```

## 扩展性

### 添加新操作系统支持

1. 创建新的处理器目录：`pkg/handlers/time/centos/`
2. 实现时间处理器接口
3. 在 `init.go` 中注册处理器

### 处理器优先级

处理器按注册顺序匹配，优先级从高到低：
1. Ubuntu专用处理器
2. 通用Linux处理器

## 安全考虑

- 配置文件权限应设置为 `644`
- NTP服务器地址应使用可信来源
- 定期检查时间同步状态
- 监控时间偏移和同步质量

## 性能优化

- 选择地理位置较近的NTP服务器
- 配置合适的轮询间隔
- 监控网络延迟和抖动
- 使用多个NTP服务器提高可靠性

## 测试

### 运行单元测试

```bash
# 测试Ubuntu处理器
go test -v ./pkg/handlers/time/ubuntu/

# 测试通用Linux处理器
go test -v ./pkg/handlers/time/

# 测试所有时间处理器
go test -v ./pkg/handlers/time/...
```

### 集成测试

```bash
# 启动nix-operator
./nix-operator -base-dir /path/to/config

# 应用测试配置
cp test-time-config.json etc/cr.d/

# 验证配置应用
timedatectl status
chronyc sources
```

## 更新日志

### v1.1.0
- 添加Ubuntu专用时间处理器
- 修复chrony配置文件路径问题
- 重构处理器注册机制
- 改进错误处理和日志记录

### v1.0.0
- 初始版本，支持通用Linux时间配置
- 实现时区和NTP配置功能
- 支持chrony服务管理