package common

import (
	"fmt"
	"os"
	"path/filepath"
)

// 外部模板目录，用于高优先级覆盖
const externalTemplateDir = "/etc/nix-operator/templates"

// GetTemplateContent 获取模板内容，优先使用外部模板
func GetTemplateContent(templateName, defaultContent string) (string, error) {
	externalPath := filepath.Join(externalTemplateDir, templateName)
	if _, err := os.Stat(externalPath); err == nil {
		content, err := os.ReadFile(externalPath)
		if err != nil {
			return "", fmt.Errorf("failed to read external template %s: %v", externalPath, err)
		}
		return string(content), nil
	}
	return defaultContent, nil
}
