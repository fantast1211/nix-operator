# 系统配置后端三层架构设计

## 架构概览

本项目实现了基于 MVC 思想演化的三层后端架构：

- **Controller 层**：处理 HTTP 请求，进行类型校验、kind 路由与 proto/domain 类型转换
- **Service 层**：实现业务流程调度，协调 ConfigRepository 和 ReconcileRepository
- **Repository 层**：完成配置文件存储（修改操作）和资源调谐（查询操作）分离

## 目录结构

```
pkg/
├── controller/
│   ├── interfaces.go          # Controller 层接口定义
│   ├── resource_controller.go # gRPC + REST 控制器实现
│   └── controller.go          # 原有文件监控控制器（保持兼容）
├── service/
│   ├── interfaces.go          # Service 层接口定义
│   └── resource_service.go    # 资源服务实现
├── repository/
│   ├── interfaces.go          # Repository 层接口定义
│   ├── config_repository.go   # 配置文件操作实现
│   └── reconcile_repository.go # 调谐操作实现
├── domain/
│   └── types.go               # 领域模型定义
├── validator/
│   ├── interfaces.go          # 校验器接口
│   ├── hosts_validator.go     # hosts 配置校验器
│   └── time_validator.go      # time 配置校验器
└── adapter/
    └── handler_adapter.go     # 现有处理器适配器
```

## 核心接口

### Controller 层接口

```go
type ResourceController interface {
    ListResourceConfigs(ctx context.Context, req *ListResourceConfigsRequest) (*ListResourceConfigsResponse, error)
    GetResourceConfig(ctx context.Context, req *GetResourceConfigRequest) (*Resource, error)
    UpdateResourceConfig(ctx context.Context, req *UpdateResourceConfigRequest) (*Resource, error)
}

type SpecValidator interface {
    Validate(spec *anypb.Any) error
    GetKind() string
}

type ConfigHandler interface {
    Reconcile(ctx context.Context, config *domain.ResourceConfig) (*status.ReconcileResult, error)
    Match(osInfo domain.OSInfo) bool
}
```

### Service 层接口

```go
type ResourceService interface {
    ListResources(ctx context.Context, kind string) ([]*domain.Resource, error)
    GetResource(ctx context.Context, name string) (*domain.Resource, error)
    UpdateResource(ctx context.Context, resource *domain.Resource) (*domain.Resource, error)
    DeleteResource(ctx context.Context, name string) error
}
```

### Repository 层接口

```go
type ConfigRepository interface {
    List(ctx context.Context, kind string) ([]*domain.ResourceWithStatus, error)
    Get(ctx context.Context, name string) (*domain.ResourceWithStatus, error)
    Save(ctx context.Context, config *domain.ResourceConfig) error
    Delete(ctx context.Context, name string) error
}

type ReconcileRepository interface {
    Reconcile(ctx context.Context, config *domain.ResourceConfig) (*domain.ReconcileResult, error)
    ReconcileAll(ctx context.Context) error
}
```

## 使用示例

### 1. 初始化应用

```go
// 在 main.go 中
app, err := initializeApplication(configDir, logger)
if err != nil {
    logger.Fatal("main", "Failed to initialize application: "+err.Error())
}
```

### 2. 使用 Controller 层

```go
// 列出所有 hosts 配置
req := &controller.ListResourceConfigsRequest{
    Kind: "HostsConfiguration",
}
resp, err := app.ResourceController.ListResourceConfigs(ctx, req)

// 获取特定配置
getReq := &controller.GetResourceConfigRequest{
    Name: "my-hosts-config",
}
resource, err := app.ResourceController.GetResourceConfig(ctx, getReq)

// 更新配置
updateReq := &controller.UpdateResourceConfigRequest{
    Resource: &controller.Resource{
        APIVersion: "v1",
        Kind:       "HostsConfiguration",
        Metadata: &controller.Metadata{
            Name: "my-hosts-config",
        },
        Spec: map[string]interface{}{
            "hosts": []map[string]interface{}{
                {
                    "ip":        "192.168.1.100",
                    "hostnames": []string{"example.com", "www.example.com"},
                },
            },
        },
    },
}
updatedResource, err := app.ResourceController.UpdateResourceConfig(ctx, updateReq)
```

### 3. 使用 Service 层

```go
// 直接使用服务层（跳过 Controller 层的类型转换）
resources, err := app.ResourceService.ListResources(ctx, "HostsConfiguration")
resource, err := app.ResourceService.GetResource(ctx, "my-hosts-config")
```

### 4. 使用 Repository 层

```go
// 直接操作配置文件
config := &domain.ResourceConfig{
    APIVersion: "v1",
    Kind:       "HostsConfiguration",
    Metadata: domain.Metadata{
        Name: "my-hosts-config",
    },
    Spec: json.RawMessage(`{"hosts":[{"ip":"192.168.1.100","hostnames":["example.com"]}]}`),
}
err := app.ConfigRepo.Save(ctx, config)

// 执行调谐
result, err := app.ReconcileRepo.Reconcile(ctx, config)
```

## 扩展新配置类型

要添加新的配置类型（如 DNSConfiguration），只需：

### 1. 定义 Spec 类型

```go
// 在 pkg/domain/types.go 中添加
type DNSConfigurationSpec struct {
    Nameservers []string `json:"nameservers"`
    SearchDomains []string `json:"searchDomains,omitempty"`
}
```

### 2. 创建校验器

```go
// pkg/validator/dns_validator.go
type dnsValidator struct{}

func NewDNSValidator() SpecValidator {
    return &dnsValidator{}
}

func (v *dnsValidator) GetKind() string {
    return "DNSConfiguration"
}

func (v *dnsValidator) Validate(spec *anypb.Any) error {
    // 实现校验逻辑
    return nil
}
```

### 3. 创建处理器

```go
// pkg/handlers/dns/linux.go
type LinuxDNSHandler struct{}

func init() {
    controller.RegisterHandler("DNSConfiguration", &LinuxDNSHandler{})
}

func (h *LinuxDNSHandler) Match(osInfo controller.OSInfo) bool {
    return osInfo.KernelName == "Linux"
}

func (h *LinuxDNSHandler) Reconcile(ctx context.Context, cfg *config.ResourceConfig) (*status.ReconcileResult, error) {
    // 实现调谐逻辑
    return nil, nil
}
```

### 4. 注册组件

```go
// 在 main.go 的 initializeValidators 中添加
dnsValidator := validator.NewDNSValidator()
validators[dnsValidator.GetKind()] = dnsValidator

// 在 initializeHandlers 的 requiredTypes 中添加
requiredTypes := []string{
    "HostsConfiguration",
    "TimeConfiguration",
    "DNSConfiguration", // 新增
}
```

## 兼容性

新的三层架构与现有的文件监控机制完全兼容：

- 现有的处理器通过适配器模式集成到新架构中
- 文件监控控制器继续工作，保持现有功能
- 可以逐步迁移到新的 API 接口

## 优势

1. **职责清晰**：读操作与写操作行为分离，避免重复调谐
2. **可扩展性强**：插件机制支持按 kind 注册 handler/validator
3. **代码复用**：充分复用现有调谐逻辑，不重复造轮子
4. **类型安全**：domain 层定义内部结构，避免 proto 类型入侵
5. **易于测试**：每层都有明确的接口，便于单元测试和集成测试