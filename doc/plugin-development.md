# 插件开发指南

本文档介绍如何为 nix-operator 开发新的配置插件。

## 概述

nix-operator 使用自动扫描注册系统来管理配置校验器，这意味着当您添加新的配置类型时，系统会自动发现并注册相应的校验器，无需手动修改代码。

## 开发新插件的步骤

### 1. 定义 Proto 消息类型

在 `api/system/v1/system.proto` 文件中定义您的配置规范。配置类型必须以 `ConfigurationSpec` 结尾：

```protobuf
// 示例：DNS 配置规范
message DnsConfigurationSpec {
  // 节点选择器
  NodeSelector node_selector = 1;
  
  // DNS 服务器列表
  repeated string dns_servers = 2;
  
  // 搜索域列表
  repeated string search_domains = 3;
  
  // DNS 超时设置（秒）
  int32 timeout = 4;
}
```

### 2. 生成 Go 代码

定义完 proto 消息后，运行以下命令生成 Go 代码：

```bash
# 在项目根目录执行
make generate-proto
# 或者手动执行
protoc --go_out=. --go_opt=paths=source_relative api/system/v1/system.proto
```

### 3. 实现处理器（可选）

如果您的配置需要特殊的处理逻辑，可以在 `pkg/handlers/` 目录下创建相应的处理器：

```go
package dns

import (
    "context"
    "fmt"
    
    systemv1 "go.xbrother.com/nix-operator/api/system/v1"
    "go.xbrother.com/nix-operator/pkg/utils"
)

type DnsHandler struct {
    logger *utils.Logger
}

func NewDnsHandler(logger *utils.Logger) *DnsHandler {
    return &DnsHandler{
        logger: logger,
    }
}

func (h *DnsHandler) Apply(ctx context.Context, spec *systemv1.DnsConfigurationSpec) error {
    h.logger.Infof("应用 DNS 配置: %+v", spec)
    
    // 实现您的配置应用逻辑
    // 例如：修改 /etc/resolv.conf 文件
    
    return nil
}

func (h *DnsHandler) Validate(spec *systemv1.DnsConfigurationSpec) error {
    // 实现配置验证逻辑
    if len(spec.DnsServers) == 0 {
        return fmt.Errorf("至少需要配置一个 DNS 服务器")
    }
    
    return nil
}
```

### 4. 自定义校验器（可选）

如果您需要比默认 `ProtoValidator` 更复杂的校验逻辑，可以实现自定义校验器：

```go
package validator

import (
    "fmt"
    "net"
    
    systemv1 "go.xbrother.com/nix-operator/api/system/v1"
    "google.golang.org/protobuf/types/known/anypb"
)

type DnsValidator struct {
    kind string
}

func NewDnsValidator() *DnsValidator {
    return &DnsValidator{
        kind: "DnsConfiguration",
    }
}

func (v *DnsValidator) GetKind() string {
    return v.kind
}

func (v *DnsValidator) Validate(spec *anypb.Any) error {
    var dnsSpec systemv1.DnsConfigurationSpec
    if err := spec.UnmarshalTo(&dnsSpec); err != nil {
        return fmt.Errorf("无法解析 DNS 配置: %w", err)
    }
    
    // 验证 DNS 服务器地址格式
    for _, server := range dnsSpec.DnsServers {
        if net.ParseIP(server) == nil {
            return fmt.Errorf("无效的 DNS 服务器地址: %s", server)
        }
    }
    
    // 验证超时设置
    if dnsSpec.Timeout < 1 || dnsSpec.Timeout > 30 {
        return fmt.Errorf("DNS 超时设置必须在 1-30 秒之间")
    }
    
    return nil
}
```

然后在 `main.go` 的 `initializeValidators` 函数中注册自定义校验器：

```go
func initializeValidators(logger *utils.Logger) map[string]validator.SpecValidator {
    registry := validator.NewAutoValidatorRegistry()
    
    // 注册自定义校验器（会覆盖默认的 ProtoValidator）
    registry.RegisterOverride("DnsConfiguration", validator.NewDnsValidator())
    
    // 执行自动扫描
    if err := registry.AutoScan(); err != nil {
        logger.Errorf("自动扫描校验器失败: %v", err)
        return nil
    }
    
    return registry.GetValidators()
}
```

### 5. 集成到控制器（可选）

如果您需要在 Kubernetes 控制器中处理配置，可以在 `pkg/controller/` 中添加相应的逻辑。

## 自动扫描机制

### 工作原理

自动扫描系统会：

1. **扫描 systemv1 包**：查找所有以 `ConfigurationSpec` 结尾的 proto 消息类型
2. **自动注册**：为每个发现的类型创建 `ProtoValidator` 实例
3. **支持覆盖**：允许注册自定义校验器来覆盖默认行为
4. **零配置**：新增配置类型无需修改任何注册代码

### 命名约定

- **Proto 消息类型**：必须以 `ConfigurationSpec` 结尾
- **校验器 Kind**：自动从类型名生成，去掉 `Spec` 后缀
  - 例如：`DnsConfigurationSpec` → `DnsConfiguration`

### 扫描方法

系统提供两种扫描方法：

1. **预定义扫描** (`AutoScan`)：基于已知的配置类型列表
2. **反射扫描** (`AutoScanWithReflection`)：使用 Go 反射动态发现类型

## 测试

### 单元测试

为您的处理器和校验器编写单元测试：

```go
func TestDnsValidator_Validate(t *testing.T) {
    validator := NewDnsValidator()
    
    // 测试有效配置
    validSpec := &systemv1.DnsConfigurationSpec{
        DnsServers: []string{"8.8.8.8", "8.8.4.4"},
        Timeout:    5,
    }
    
    anySpec, _ := anypb.New(validSpec)
    err := validator.Validate(anySpec)
    assert.NoError(t, err)
    
    // 测试无效配置
    invalidSpec := &systemv1.DnsConfigurationSpec{
        DnsServers: []string{"invalid-ip"},
        Timeout:    5,
    }
    
    anySpec, _ = anypb.New(invalidSpec)
    err = validator.Validate(anySpec)
    assert.Error(t, err)
}
```

### 集成测试

在 `test/integration/` 目录下添加集成测试来验证完整的配置流程。

## 最佳实践

### 1. 错误处理

- 使用结构化日志记录
- 提供清晰的错误消息
- 实现优雅的错误恢复

### 2. 配置验证

- 在校验器中进行输入验证
- 检查配置的逻辑一致性
- 验证外部依赖的可用性

### 3. 幂等性

- 确保配置应用操作是幂等的
- 支持配置的增量更新
- 处理配置回滚场景

### 4. 性能考虑

- 避免在校验器中执行耗时操作
- 使用缓存减少重复计算
- 实现异步处理长时间运行的任务

## 示例：完整的插件开发流程

以下是开发一个 DNS 配置插件的完整示例：

### 1. Proto 定义

```protobuf
// 在 api/system/v1/system.proto 中添加
message DnsConfigurationSpec {
  NodeSelector node_selector = 1;
  repeated string dns_servers = 2;
  repeated string search_domains = 3;
  int32 timeout = 4;
}
```

### 2. 生成代码

```bash
make generate-proto
```

### 3. 实现处理器

```go
// pkg/handlers/dns/dns_handler.go
package dns

// ... 实现代码
```

### 4. 测试

```bash
go test ./pkg/handlers/dns/...
```

### 5. 验证自动注册

```bash
# 编译并运行
go build ./cmd/operator
./operator --help

# 检查日志确认 DnsConfiguration 校验器已注册
```

## 故障排除

### 常见问题

1. **校验器未被发现**
   - 检查配置类型名是否以 `ConfigurationSpec` 结尾
   - 确认 proto 代码已重新生成
   - 查看启动日志中的扫描结果

2. **校验失败**
   - 检查 proto 消息的字段定义
   - 验证 `anypb.Any` 的类型 URL
   - 确认自定义校验逻辑的正确性

3. **性能问题**
   - 使用 `AutoScan` 而不是 `AutoScanWithReflection`
   - 避免在校验器中执行重操作
   - 考虑使用缓存机制

### 调试技巧

- 启用详细日志记录
- 使用 `ListRegisteredKinds()` 查看已注册的类型
- 在测试中使用 `registry.GetValidator()` 验证注册状态

## 总结

通过自动扫描注册系统，开发新的配置插件变得非常简单：

1. 定义 proto 消息类型（以 `ConfigurationSpec` 结尾）
2. 生成 Go 代码
3. 可选：实现自定义校验器或处理器
4. 测试和验证

系统会自动发现并注册您的配置类型，无需手动修改任何注册代码。这大大简化了插件开发流程，提高了系统的可扩展性。