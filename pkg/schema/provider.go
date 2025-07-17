package schema

import (
	"embed"
)

//go:embed *.json
var schemaFiles embed.FS

// ResourceSchemaInfo 包含资源 schema 的元信息
type ResourceSchemaInfo struct {
	Kind        string
	DisplayName string
	Version     string
	JSONSchema  string
}

// Provider 提供 JSON Schema 服务
type Provider interface {
	GetSchemas(kind string) ([]ResourceSchemaInfo, error)
	GetAllSchemas() ([]ResourceSchemaInfo, error)
}

// schemaProvider 实现 Provider 接口
type schemaProvider struct {
	schemas map[string]ResourceSchemaInfo
}

// NewProvider 创建新的 schema provider
func NewProvider() Provider {
	return &schemaProvider{
		schemas: initSchemas(),
	}
}

// GetSchemas 根据 kind 获取 schemas
func (p *schemaProvider) GetSchemas(kind string) ([]ResourceSchemaInfo, error) {
	if kind == "" {
		return p.GetAllSchemas()
	}

	schema, exists := p.schemas[kind]
	if !exists {
		return []ResourceSchemaInfo{}, nil
	}

	return []ResourceSchemaInfo{schema}, nil
}

// GetAllSchemas 获取所有 schemas
func (p *schemaProvider) GetAllSchemas() ([]ResourceSchemaInfo, error) {
	result := make([]ResourceSchemaInfo, 0, len(p.schemas))
	for _, schema := range p.schemas {
		result = append(result, schema)
	}
	return result, nil
}

// initSchemas 初始化 schemas 映射
func initSchemas() map[string]ResourceSchemaInfo {
	schemas := make(map[string]ResourceSchemaInfo)

	// 读取 HostsConfigurationSpec schema
	if hostsSchema, err := schemaFiles.ReadFile("HostsConfigurationSpec.json"); err == nil {
		schemas["HostsConfiguration"] = ResourceSchemaInfo{
			Kind:        "HostsConfiguration",
			DisplayName: "主机配置",
			Version:     "v1",
			JSONSchema:  string(hostsSchema),
		}
	}

	// 读取 TimeConfigurationSpec schema
	if timeSchema, err := schemaFiles.ReadFile("TimeConfigurationSpec.json"); err == nil {
		schemas["TimeConfiguration"] = ResourceSchemaInfo{
			Kind:        "TimeConfiguration",
			DisplayName: "时间配置",
			Version:     "v1",
			JSONSchema:  string(timeSchema),
		}
	}

	// 读取 NetworkConfigurationSpec schema
	if networkSchema, err := schemaFiles.ReadFile("NetworkConfigurationSpec.json"); err == nil {
		schemas["NetworkConfiguration"] = ResourceSchemaInfo{
			Kind:        "NetworkConfiguration",
			DisplayName: "网络配置",
			Version:     "v1",
			JSONSchema:  string(networkSchema),
		}
	}

	// 读取 BondConfigurationSpec schema
	if networkSchema, err := schemaFiles.ReadFile("BondConfigurationSpec.json"); err == nil {
		schemas["NetworkConfiguration"] = ResourceSchemaInfo{
			Kind:        "BondConfiguration",
			DisplayName: "Bond配置",
			Version:     "v1",
			JSONSchema:  string(networkSchema),
		}
	}

	return schemas
}
