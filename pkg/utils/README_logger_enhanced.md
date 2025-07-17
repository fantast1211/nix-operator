# 增强版日志系统使用指南

## 概述

本项目的日志系统已经过优化，现在支持日志轮转和可配置的日志级别，提供更好的性能和可维护性。

## 主要特性

### 1. 日志轮转机制
- 使用 `lumberjack` 库实现自动日志轮转
- 支持按文件大小轮转
- 支持保留指定数量的备份文件
- 支持按时间清理旧日志
- 可选的日志压缩功能

### 2. 可配置的日志级别
- 支持通过环境变量设置最低日志级别
- 避免输出不必要的低级别日志，提高性能
- 支持运行时动态调整

### 3. 双重输出
- 同时输出到文件和控制台
- 普通日志和错误日志分离
- 保持向后兼容性

## 环境变量配置

### 日志级别配置
```bash
# 设置最低日志级别（DEBUG, INFO, WARN, ERROR, FATAL）
export LOG_LEVEL=INFO
```

### 日志轮转配置
```bash
# 单个日志文件最大大小（MB），默认10MB
export LOG_MAX_SIZE=10

# 保留的备份文件数量，默认5个
export LOG_MAX_BACKUPS=5

# 日志文件保留天数，默认30天
export LOG_MAX_AGE=30

# 是否压缩旧日志文件，默认false
export LOG_COMPRESS=true
```

## 使用示例

### 基本使用
```go
package main

import (
    "go.xbrother.com/nix-operator/pkg/utils"
)

func main() {
    // 初始化日志系统
    if err := utils.InitLogger("my-service"); err != nil {
        panic(err)
    }
    defer utils.CloseLogger()

    // 使用不同级别的日志
    utils.Debug("module", "调试信息")
    utils.Info("module", "普通信息")
    utils.Warn("module", "警告信息")
    utils.Error("module", "错误信息")

    // 使用格式化日志
    utils.Infof("module", "用户 %s 登录成功，ID: %d", "admin", 1001)
    utils.Warnf("module", "连接超时，重试次数: %d", 3)
}
```

### 生产环境配置示例
```bash
#!/bin/bash
# 生产环境日志配置
export LOG_LEVEL=INFO
export LOG_MAX_SIZE=50      # 50MB per file
export LOG_MAX_BACKUPS=10   # Keep 10 backup files
export LOG_MAX_AGE=7        # Keep logs for 7 days
export LOG_COMPRESS=true    # Compress old logs

# 启动应用
./nix-operator
```

### 开发环境配置示例
```bash
#!/bin/bash
# 开发环境日志配置
export LOG_LEVEL=DEBUG
export LOG_MAX_SIZE=10      # 10MB per file
export LOG_MAX_BACKUPS=3    # Keep 3 backup files
export LOG_MAX_AGE=1        # Keep logs for 1 day
export LOG_COMPRESS=false   # Don't compress for easier debugging

# 启动应用
./nix-operator
```

## 日志文件结构

```
/opt/log/
├── xtopus.log              # 当前日志文件（INFO, WARN, DEBUG）
├── xtopus.log.1            # 备份文件1
├── xtopus.log.2.gz         # 压缩的备份文件2
├── xtopus_error.log        # 当前错误日志文件（ERROR, FATAL）
├── xtopus_error.log.1      # 错误日志备份文件1
└── xtopus_error.log.2.gz   # 压缩的错误日志备份文件2
```

## 日志级别说明

| 级别  | 数值 | 用途 | 示例 |
|-------|------|------|------|
| DEBUG | 0    | 调试信息，详细的程序执行流程 | 函数调用、变量值、执行路径 |
| INFO  | 1    | 一般信息，程序正常运行状态 | 服务启动、配置加载、操作完成 |
| WARN  | 2    | 警告信息，可能的问题但不影响运行 | 配置缺失使用默认值、重试操作 |
| ERROR | 3    | 错误信息，程序遇到错误但可以继续 | 网络连接失败、文件读取错误 |
| FATAL | 4    | 致命错误，程序无法继续运行 | 初始化失败、关键资源不可用 |

## 性能优化

### 日志级别过滤
当设置 `LOG_LEVEL=WARN` 时，DEBUG 和 INFO 级别的日志将被完全跳过，不会进行字符串格式化和文件写入，显著提高性能。

### 建议配置
- **生产环境**: `LOG_LEVEL=INFO` 或 `LOG_LEVEL=WARN`
- **测试环境**: `LOG_LEVEL=DEBUG`
- **开发环境**: `LOG_LEVEL=DEBUG`

## 监控和维护

### 日志文件监控
```bash
# 查看当前日志文件大小
ls -lh /opt/log/

# 实时查看日志
tail -f /opt/log/xtopus.log

# 查看错误日志
tail -f /opt/log/xtopus_error.log

# 统计日志级别分布
grep -c "\[INFO\]" /opt/log/xtopus.log
grep -c "\[WARN\]" /opt/log/xtopus.log
grep -c "\[ERROR\]" /opt/log/xtopus_error.log
```

### 日志清理
日志系统会自动根据配置清理旧文件，但也可以手动清理：
```bash
# 手动清理超过7天的日志
find /opt/log -name "*.log*" -mtime +7 -delete

# 清理压缩日志文件
find /opt/log -name "*.gz" -mtime +30 -delete
```

## 故障排除

### 常见问题

1. **日志文件权限问题**
   ```bash
   sudo chown -R $(whoami):$(whoami) /opt/log
   sudo chmod -R 755 /opt/log
   ```

2. **磁盘空间不足**
   - 检查磁盘使用情况：`df -h`
   - 调整日志保留策略：减少 `LOG_MAX_BACKUPS` 或 `LOG_MAX_AGE`
   - 启用压缩：`LOG_COMPRESS=true`

3. **日志级别不生效**
   - 确认环境变量设置：`echo $LOG_LEVEL`
   - 重启应用程序
   - 检查环境变量格式是否正确

## 最佳实践

1. **合理设置日志级别**：生产环境避免使用 DEBUG 级别
2. **定期监控日志文件大小**：防止磁盘空间耗尽
3. **使用结构化日志**：在日志消息中包含关键信息
4. **错误处理**：重要错误同时记录到错误日志和普通日志
5. **性能考虑**：避免在高频调用的代码中使用低级别日志

## 升级说明

从旧版本升级时需要注意：
- 新增了 `lumberjack` 依赖，需要运行 `go mod tidy`
- 环境变量配置是可选的，不设置时使用默认值
- API 保持向后兼容，现有代码无需修改
- 日志文件结构保持不变，只是增加了轮转功能