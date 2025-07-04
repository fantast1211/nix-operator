# Xtopus 日志工具使用指南

## 概述

本日志工具为 xtopus agent 提供统一的日志能力，支持同时写入文件和控制台输出，并将错误日志单独分离存储。

## 特性

- **双重输出**: 日志同时写入文件和控制台
- **日志分离**: 普通日志和错误日志分别存储
- **模块化**: 支持按模块名称区分日志来源
- **多级别**: 支持 DEBUG、INFO、WARN、ERROR、FATAL 五个级别
- **格式化**: 支持格式化字符串输出
- **全局访问**: 提供全局日志实例，便于在各模块中使用

## 日志文件位置

- **普通日志**: `/opt/log/xtopus.log` (INFO、DEBUG、WARN 级别)
- **错误日志**: `/opt/log/xtopus_error.log` (ERROR、FATAL 级别)

## 日志格式

```
[时间戳] [日志级别] [模块名称] 日志内容
```

示例:
```
[2025-07-04 12:00:00] [INFO] [webServer] Server started at port 8080
[2025-07-04 12:00:01] [ERROR] [database] Connection failed
```

## 使用方法

### 1. 初始化日志系统

在 main 函数中初始化日志系统：

```go
package main

import (
    "go.xbrother.com/nix-operator/pkg/utils"
)

func main() {
    // 初始化日志系统
    if err := utils.InitLogger("xtopus"); err != nil {
        log.Fatalf("Failed to initialize logger: %v", err)
    }
    defer utils.CloseLogger()
    
    // 你的应用代码...
}
```

### 2. 在模块中使用日志

#### 基本日志函数

```go
// 调试日志
utils.Debug("moduleName", "Debug message")

// 信息日志
utils.Info("moduleName", "Info message")

// 警告日志
utils.Warn("moduleName", "Warning message")

// 错误日志
utils.Error("moduleName", "Error message")

// 致命错误日志（会退出程序）
utils.Fatal("moduleName", "Fatal error message")
```

#### 格式化日志函数

```go
// 格式化调试日志
utils.Debugf("moduleName", "Debug: %s = %d", "value", 42)

// 格式化信息日志
utils.Infof("moduleName", "Server listening on %s:%d", "localhost", 8080)

// 格式化警告日志
utils.Warnf("moduleName", "Cache hit ratio: %.2f%%", 85.5)

// 格式化错误日志
utils.Errorf("moduleName", "HTTP %d: %s", 500, "Internal Server Error")

// 格式化致命错误日志
utils.Fatalf("moduleName", "Failed to connect to %s", "database")
```

### 3. 模块示例

#### Web 服务器模块

```go
func StartWebServer() {
    moduleName := "webServer"
    
    utils.Info(moduleName, "Initializing web server...")
    utils.Infof(moduleName, "Server listening on %s:%d", "localhost", 8080)
    
    // 请求处理
    utils.Debugf(moduleName, "Processing request from %s", clientIP)
    
    // 错误处理
    if err != nil {
        utils.Errorf(moduleName, "Request failed: %v", err)
    }
}
```

#### 控制器模块

```go
func (c *Controller) ProcessConfig() {
    moduleName := "controller"
    
    utils.Info(moduleName, "Starting configuration processing")
    utils.Debugf(moduleName, "Loading config from: %s", configPath)
    
    if err := c.loadConfig(); err != nil {
        utils.Errorf(moduleName, "Failed to load config: %v", err)
        return
    }
    
    utils.Info(moduleName, "Configuration processed successfully")
}
```

#### 处理器模块

```go
func (h *HostsHandler) Reconcile() {
    moduleName := "hostsHandler"
    
    utils.Info(moduleName, "Starting hosts reconciliation")
    
    for _, host := range hosts {
        utils.Debugf(moduleName, "Processing host: %s", host)
        
        if host.Domain == ".local" {
            utils.Warnf(moduleName, "Host %s uses .local domain", host.Name)
        }
        
        utils.Infof(moduleName, "Host configured: %s", host.Name)
    }
    
    utils.Info(moduleName, "Hosts reconciliation completed")
}
```

## 高级用法

### 1. 创建独立的日志实例

如果需要创建独立的日志实例（而不使用全局实例）：

```go
logger, err := utils.NewLogger("myService")
if err != nil {
    return err
}
defer logger.Close()

logger.Info("module", "Using independent logger instance")
```

### 2. 优雅关闭

在应用关闭时确保日志文件正确关闭：

```go
// 设置信号处理
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
go func() {
    <-sigChan
    utils.Info("main", "Received shutdown signal")
    utils.CloseLogger()
    os.Exit(0)
}()
```

## 最佳实践

1. **模块命名**: 使用清晰的模块名称，如 "webServer"、"controller"、"hostsHandler" 等
2. **日志级别**: 
   - DEBUG: 详细的调试信息
   - INFO: 一般信息，如启动、完成等
   - WARN: 警告信息，不影响正常运行
   - ERROR: 错误信息，需要关注但不致命
   - FATAL: 致命错误，会导致程序退出
3. **错误处理**: 在记录错误日志后，确保有适当的错误处理逻辑
4. **性能考虑**: 避免在高频调用的代码中使用过多的 DEBUG 日志
5. **敏感信息**: 不要在日志中记录密码、密钥等敏感信息

## 测试

运行测试程序验证日志功能：

```bash
go run test_logger.go
```

查看生成的日志文件：

```bash
# 查看普通日志
sudo cat /opt/log/xtopus.log

# 查看错误日志
sudo cat /opt/log/xtopus_error.log
```

## 故障排除

1. **权限问题**: 确保应用有权限写入 `/opt/log/` 目录
2. **磁盘空间**: 确保有足够的磁盘空间存储日志文件
3. **文件锁定**: 确保没有其他进程占用日志文件

## 扩展功能

未来可以考虑添加的功能：
- 日志轮转（按大小或时间）
- JSON 格式输出
- 远程日志传输
- 日志压缩
- 配置文件支持