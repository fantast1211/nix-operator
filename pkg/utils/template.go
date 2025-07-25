package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

// 外部模板目录，用于高优先级覆盖
const ExternalTemplateDir = "/etc/nix-operator/templates"

// GetTemplateContent 获取模板内容，优先使用外部模板
// 如果外部模板文件存在，则使用外部模板；否则使用默认内容
func GetTemplateContent(templateName, defaultContent string) (string, error) {
	externalPath := filepath.Join(ExternalTemplateDir, templateName)
	if _, err := os.Stat(externalPath); err == nil {
		content, err := os.ReadFile(externalPath)
		if err != nil {
			return "", fmt.Errorf("failed to read external template %s: %v", externalPath, err)
		}
		return string(content), nil
	}
	return defaultContent, nil
}
