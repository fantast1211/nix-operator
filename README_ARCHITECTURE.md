# Nix-Operator 三层架构实现

## 概述

本项目基于现有的 nix-operator 实现了一套完整的三层后端架构，旨在提供业务友好的 API 接口，同时保持与现有文件监控机制的完全兼容性。

## 架构设计

### 核心理念

- **职责分离**：Controller 负责接口适配，Service 负责业务编排，Repository 负责数据操作
- **插件化**：支持通过注册机制扩展新的配置类型
- **类型安全**：使用强类型的 domain 模型，避免 proto 类型污染业务逻辑
- **向后兼容**：通过适配器模式集成现有处理器，保持现有功能不变

### 层次结构

```
┌─────────────────────────────────────────────────────────────┐
│                    Controller 层                            │
│  ┌─────────────────┐  ┌─────────────────┐  ┌──────────────┐ │
│  │ ResourceController │  │  SpecValidator  │  │ ConfigHandler│ │
│  │   (gRPC/REST)    │  │   (校验器)      │  │   (处理器)   │ │
│  └─────────────────┘  └─────────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Service 层                             │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │              ResourceService                            │ │
│  │           (业务流程调度)                                │ │
│  └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   Repository 层                            │
│  ┌─────────────────┐           ┌─────────────────────────┐   │
│  │ ConfigRepository│           │  ReconcileRepository    │   │
│  │   (配置文件)    │           │     (调谐操作)          │   │
│  └─────────────────┘           └─────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## 实现文件结构

```
pkg/
├── domain/
│   └── types.go                    # 领域模型定义
├── controller/
│   ├── interfaces.go               # Controller 层接口
│   ├── resource_controller.go      # gRPC + REST 控制器实现
│   └── controller.go               # 原有文件监控控制器
├── service/
│   ├── interfaces.go               # Service 层接口
│   └── resource_service.go         # 资源服务实现
├── repository/
│   ├── interfaces.go               # Repository 层接口
│   ├── config_repository.go        # 配置文件操作
│   └── reconcile_repository.go     # 调谐操作
├── validator/
│   ├── interfaces.go               # 校验器接口
│   ├── hosts_validator.go          # hosts 配置校验
│   └── time_validator.go           # time 配置校验
├── adapter/
│   └── handler_adapter.go          # 现有处理器适配器
└── handlers/                       # 现有处理器（保持不变）
    ├── hosts/
    └── time/
```

## 核心特性

### 1. 接口解耦

所有层次都通过接口定义交互，便于测试和扩展：

```go
// Controller 层接口
type ResourceController interface {
    ListResourceConfigs(ctx context.Context, req *ListResourceConfigsRequest) (*ListResourceConfigsResponse, error)
    GetResourceConfig(ctx context.Context, req *GetResourceConfigRequest) (*Resource, error)
    UpdateResourceConfig(ctx context.Context, req *UpdateResourceConfigRequest) (*Resource, error)
}

// Service 层接口
type ResourceService interface {
    ListResources(ctx context.Context, kind string) ([]*domain.Resource, error)
    GetResource(ctx context.Context, name string) (*domain.Resource, error)
    UpdateResource(ctx context.Context, resource *domain.Resource) (*domain.Resource, error)
    DeleteResource(ctx context.Context, name string) error
}
```

### 2. 插件机制

支持通过注册机制扩展新的配置类型：

```go
// 校验器插件
type SpecValidator interface {
    Validate(spec *anypb.Any) error
    GetKind() string
}

// 处理器插件
type ConfigHandler interface {
    Reconcile(ctx context.Context, config *domain.ResourceConfig) (*status.ReconcileResult, error)
    Match(osInfo domain.OSInfo) bool
}
```

### 3. 类型系统

使用强类型的 domain 模型：

```go
// 领域模型
type Resource struct {
    APIVersion string          `json:"apiVersion"`
    Kind       string          `json:"kind"`
    Metadata   Metadata        `json:"metadata"`
    Spec       json.RawMessage `json:"spec"`
    Status     *ResourceStatus `json:"status,omitempty"`
}

// 具体的配置类型
type HostsConfigurationSpec struct {
    Hosts []HostEntry `json:"hosts"`
}
```

### 4. 向后兼容

通过适配器模式集成现有处理器：

```go
type HandlerAdapter struct {
    legacyHandler controller.Handler
}

func (a *HandlerAdapter) Reconcile(ctx context.Context, config *domain.ResourceConfig) (*status.ReconcileResult, error) {
    // 转换类型并调用现有处理器
    legacyConfig := convertToLegacyConfig(config)
    return a.legacyHandler.Reconcile(ctx, legacyConfig)
}
```

## 使用方式

### 1. 初始化应用

```go
// 在 main.go 中
app, err := initializeApplication(configDir, logger)
if err != nil {
    logger.Fatal("main", "Failed to initialize application: "+err.Error())
}
```

### 2. 使用 API

```go
// 通过 Controller 层（适用于 gRPC/REST 接口）
req := &controller.ListResourceConfigsRequest{Kind: "HostsConfiguration"}
resp, err := app.ResourceController.ListResourceConfigs(ctx, req)

// 通过 Service 层（适用于内部调用）
resources, err := app.ResourceService.ListResources(ctx, "HostsConfiguration")

// 通过 Repository 层（适用于底层操作）
config := &domain.ResourceConfig{...}
err := app.ConfigRepo.Save(ctx, config)
```

### 3. 扩展新配置类型

添加新的配置类型只需要：

1. 在 `domain/types.go` 中定义 Spec 结构
2. 创建对应的校验器实现 `SpecValidator` 接口
3. 创建对应的处理器实现 `ConfigHandler` 接口
4. 在 `main.go` 中注册新组件

## 示例代码

完整的使用示例请参考：
- `examples/api_usage.go` - API 使用示例
- `doc/three-tier-architecture.md` - 详细架构文档

## 兼容性保证

- ✅ 现有的文件监控机制继续工作
- ✅ 现有的处理器无需修改
- ✅ 现有的配置文件格式保持不变
- ✅ 可以逐步迁移到新的 API 接口

## 优势总结

1. **职责清晰**：读操作与写操作行为分离，避免重复调谐
2. **可扩展性强**：插件机制支持按 kind 注册 handler/validator
3. **代码复用**：充分复用现有调谐逻辑，不重复造轮子
4. **类型安全**：domain 层定义内部结构，避免 proto 类型入侵
5. **易于测试**：每层都有明确的接口，便于单元测试和集成测试
6. **部署简洁**：支持 HTTP+JSON 方式，无需部署 gRPC 通道

## 下一步计划

1. 实现 gRPC 服务器，暴露 ResourceController 接口
2. 添加 HTTP REST API 映射（通过 grpc-gateway）
3. 完善错误处理和日志记录
4. 添加更多配置类型的校验器和处理器
5. 实现配置的版本管理和回滚功能
6. 添加监控和指标收集

## 贡献指南

要添加新的配置类型，请遵循以下步骤：

1. Fork 项目并创建特性分支
2. 在 `pkg/domain/types.go` 中添加新的 Spec 类型定义
3. 在 `pkg/validator/` 中创建对应的校验器
4. 在 `pkg/handlers/` 中创建对应的处理器
5. 更新 `main.go` 中的注册逻辑
6. 添加单元测试和集成测试
7. 更新文档
8. 提交 Pull Request

---

这个三层架构为 nix-operator 提供了一个坚实的基础，既保持了现有功能的稳定性，又为未来的扩展提供了灵活性。