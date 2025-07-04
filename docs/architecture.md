# 系统配置后端三层架构设计

## 概述

本项目实现了一个支持插件化扩展的三层架构系统配置后端服务，将底层的系统配置抽象为业务友好的 API 接口。

## 架构设计

### 整体架构

```
┌─────────────────┐
│   Controller    │  HTTP请求处理、类型校验、路由
│     Layer       │
└─────────────────┘
         │
         ▼
┌─────────────────┐
│    Service      │  业务逻辑处理、协调调用
│     Layer       │
└─────────────────┘
         │
         ▼
┌─────────────────┐
│   Repository    │  数据存储、调谐协调
│     Layer       │  • ConfigRepository: 配置文件操作
└─────────────────┘  • ReconcileRepository: 调谐操作
```

### 核心设计理念

1. **职责分离**：查询操作触发调谐获取实时状态，修改操作直接保存文件依赖监控触发调谐
2. **接口驱动**：三层之间通过接口解耦，便于测试和扩展
3. **插件化**：新增配置类型只需最小化修改
4. **类型安全**：强类型校验和编译时检查

## 包结构

```
pkg/
├── domain/                  # 领域模型
│   └── types.go            # 核心数据结构定义
├── interfaces/             # 接口定义
│   └── interfaces.go       # 所有层间接口
├── webcontroller/          # HTTP控制器层
│   ├── interfaces.go       # Controller接口定义
│   └── resource_controller.go # HTTP请求处理
├── webservice/             # 业务服务层
│   ├── interfaces.go       # Service接口定义
│   ├── resource_service.go # 业务逻辑处理
│   └── resource_service_test.go # 单元测试
├── webrepository/          # 数据仓储层
│   ├── interfaces.go       # Repository接口定义
│   ├── config_repository.go # 配置文件操作
│   └── reconcile_repository.go # 调谐操作
├── validator/              # 类型校验器
│   ├── interfaces.go       # 校验器接口
│   ├── spec_validators.go  # 具体校验器实现
│   └── spec_validators_test.go # 校验器测试
├── registry/               # 组件注册表
│   └── registry.go         # 插件注册机制
└── controller/             # 保持现有调谐逻辑
    └── controller.go       # 文件监控和调谐
```

## 核心接口

### Controller层接口

```go
type ResourceController interface {
    ListResourceConfigs(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error)
    GetResourceConfig(ctx context.Context, name string) (*domain.ResourceWithStatus, error)
    UpdateResourceConfig(ctx context.Context, resource *domain.Resource) (*domain.Resource, error)
    DeleteResourceConfig(ctx context.Context, name string) error
}
```

### Service层接口

```go
type ResourceService interface {
    ListResources(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error)
    GetResource(ctx context.Context, name string) (*domain.ResourceWithStatus, error)
    UpdateResource(ctx context.Context, resource *domain.Resource) (*domain.Resource, error)
    DeleteResource(ctx context.Context, name string) error
}
```

### Repository层接口

```go
// ConfigRepository 配置文件操作接口
type ConfigRepository interface {
    // 查询操作 - 通过调谐获取实时状态
    List(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error)
    Get(ctx context.Context, name string) (*domain.ResourceWithStatus, error)
    
    // 修改操作 - 直接操作配置文件
    Save(ctx context.Context, config *domain.ResourceConfig) error
    Delete(ctx context.Context, name string) error
}

// ReconcileRepository 调谐操作接口
type ReconcileRepository interface {
    // 执行调谐获取实时状态（供查询操作使用）
    Reconcile(ctx context.Context, cfg *domain.ResourceConfig) (*domain.ReconcileResult, error)
    // 批量调谐（供文件监控使用）
    ReconcileAll(ctx context.Context) error
    // 列出所有配置并执行调谐
    ListResourceConfigs(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error)
}
```

## 关键特性

### 1. Repository层职责分离

**查询操作**：
- `List()` 和 `Get()` 方法调用 `ReconcileRepository` 执行调谐
- 获取配置的实时状态
- 确保返回的状态反映系统当前实际情况

**修改操作**：
- `Save()` 和 `Delete()` 方法直接操作配置文件
- 不立即执行调谐，依赖文件监控机制触发
- 避免重复调谐，保持现有文件监控逻辑

### 2. 类型校验框架

```go
type SpecValidator interface {
    Validate(spec []byte) error
    GetKind() string
}
```

支持的配置类型：
- `HostsConfiguration`：主机名配置
- `TimeConfiguration`：时间同步配置

### 3. 插件注册机制

```go
type ComponentRegistry struct {
    handlers          map[string]controller.Handler
    validatorRegistry validator.ValidatorRegistry
    osInfo            domain.OSInfo
    logger            *utils.Logger
}
```

## 使用示例

### 启动Web服务器

```bash
# 编译
go build -o webserver cmd/webserver/main.go

# 运行
./webserver
```

### API调用示例

**列出所有资源**：
```bash
curl http://localhost:8080/api/v1/resources
```

**获取特定类型资源**：
```bash
curl "http://localhost:8080/api/v1/resources?kind=HostsConfiguration"
```

**获取单个资源**：
```bash
curl "http://localhost:8080/api/v1/resource?name=hosts-config"
```

**更新资源**：
```bash
curl -X PUT http://localhost:8080/api/v1/resource \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "v1",
    "kind": "HostsConfiguration",
    "metadata": {
      "name": "hosts-config"
    },
    "spec": {
      "hosts": [
        {
          "ip": "127.0.0.1",
          "hostnames": ["localhost", "local"]
        }
      ]
    }
  }'
```

## 扩展新配置类型

### 1. 定义新的Spec类型

在 `pkg/domain/types.go` 中添加：

```go
type NetworkConfigurationSpec struct {
    Interface string `json:"interface"`
    Address   string `json:"address"`
    Gateway   string `json:"gateway"`
}
```

### 2. 实现校验器

在 `pkg/validator/spec_validators.go` 中添加：

```go
type NetworkConfigurationValidator struct{}

func (v *NetworkConfigurationValidator) Validate(spec []byte) error {
    var networkSpec domain.NetworkConfigurationSpec
    if err := json.Unmarshal(spec, &networkSpec); err != nil {
        return fmt.Errorf("invalid network configuration spec: %v", err)
    }
    
    // 添加具体校验逻辑
    if networkSpec.Interface == "" {
        return fmt.Errorf("interface is required")
    }
    
    return nil
}

func (v *NetworkConfigurationValidator) GetKind() string {
    return "NetworkConfiguration"
}
```

### 3. 实现处理器

在 `pkg/handlers/network/` 目录下创建处理器：

```go
type LinuxNetworkHandler struct{}

func (h *LinuxNetworkHandler) Match(osInfo controller.OSInfo) bool {
    return osInfo.KernelName == "Linux"
}

func (h *LinuxNetworkHandler) Reconcile(ctx context.Context, cfg *config.ResourceConfig) (*controller.ReconcileResult, error) {
    // 实现网络配置调谐逻辑
    return &controller.ReconcileResult{
        Effective: cfg,
        Status: &config.ResourceStatus{Phase: "Ready"},
    }, nil
}
```

### 4. 注册组件

在 `cmd/webserver/main.go` 中注册：

```go
func registerComponents(componentRegistry *registry.ComponentRegistry) error {
    // 注册校验器
    validator.RegisterDefaultValidators(componentRegistry.GetValidatorRegistry())
    componentRegistry.RegisterValidator(&validator.NetworkConfigurationValidator{})
    
    // 注册处理器
    componentRegistry.RegisterHandler("NetworkConfiguration", &network.LinuxNetworkHandler{})
    
    return nil
}
```

## 测试

### 运行单元测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./pkg/webservice
go test ./pkg/validator

# 运行测试并显示覆盖率
go test -cover ./...
```

### 测试覆盖的场景

1. **Service层测试**：
   - 资源的CRUD操作
   - 错误处理
   - 校验逻辑

2. **Validator测试**：
   - 有效配置校验
   - 无效配置检测
   - 注册表功能

## 性能优化

### 1. 查询优化
- 查询操作触发调谐，确保状态实时性
- 可考虑添加缓存机制减少重复调谐

### 2. 修改优化
- 修改操作不立即调谐，依赖文件监控
- 避免重复调谐，提高性能

### 3. 并发处理
- 支持并发请求处理
- 文件操作使用原子写入

## 安全考虑

1. **输入校验**：所有输入都经过严格的类型校验
2. **文件权限**：配置文件使用适当的权限设置
3. **错误处理**：避免敏感信息泄露
4. **原子操作**：文件写入使用原子操作避免竞态条件

## 监控和日志

- 使用结构化日志记录关键操作
- 支持不同日志级别
- 记录性能指标和错误信息
- 提供健康检查端点

## 总结

本架构设计实现了以下目标：

1. **清晰的职责分离**：查询触发调谐，修改操作文件
2. **良好的扩展性**：插件化架构支持新配置类型
3. **强类型安全**：编译时和运行时类型检查
4. **高性能**：避免重复调谐，优化文件操作
5. **易于测试**：接口驱动设计，便于单元测试
6. **向后兼容**：复用现有controller调谐逻辑

该架构为系统配置管理提供了一个稳定、可扩展、高性能的后端服务框架。