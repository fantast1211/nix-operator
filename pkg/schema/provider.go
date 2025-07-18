package schema

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
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

// SchemaMetadata 用于解析JSON schema中的元数据
type SchemaMetadata struct {
	Definitions map[string]struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"definitions"`
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

	// 动态加载所有以 Spec.json 结尾的文件
	err := fs.WalkDir(schemaFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 只处理以 Spec.json 结尾的文件
		if !d.IsDir() && strings.HasSuffix(path, "Spec.json") {
			// 从文件名提取 Kind（去掉 Spec.json 后缀）
			fileName := filepath.Base(path)
			kind := strings.TrimSuffix(fileName, "Spec.json")

			// 读取文件内容
			if schemaContent, readErr := schemaFiles.ReadFile(path); readErr == nil {
				// 从JSON中提取显示名称
				displayName := extractDisplayNameFromJSON(schemaContent, kind)

				schemas[kind] = ResourceSchemaInfo{
					Kind:        kind,
					DisplayName: displayName,
					Version:     "v1",
					JSONSchema:  string(schemaContent),
				}
			}
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error loading schemas: %v\n", err)
	}

	return schemas
}

// extractDisplayNameFromJSON 从JSON schema中提取显示名称
func extractDisplayNameFromJSON(schemaContent []byte, kind string) string {
	var metadata SchemaMetadata
	if err := json.Unmarshal(schemaContent, &metadata); err != nil {
		// 如果解析失败，返回默认名称
		return kind + "配置"
	}

	// 查找对应的定义
	for defName, def := range metadata.Definitions {
		// 匹配定义名称（通常是 KindSpec 格式）
		if strings.Contains(defName, kind) {
			if def.Title != "" {
				return def.Title
			}
			if def.Description != "" {
				// 如果没有title，使用description的第一部分
				parts := strings.Split(def.Description, " ")
				if len(parts) > 0 {
					return parts[0]
				}
			}
		}
	}

	// 如果都没找到，返回默认名称
	return kind + "配置"
}
