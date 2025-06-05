package controller

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"go.xbrother.com/nix-operator/pkg/config"
)

// ReconcileResult 包含调谐的结果
type ReconcileResult struct {
	Effective *config.ResourceConfig
	Status    *config.ResourceStatus
}

// reconcile 执行调谐操作，扫描配置目录中的所有配置文件并应用
func (c *Controller) reconcile() {
	ctx := context.Background()

	// 扫描配置目录中的所有配置文件
	err := filepath.Walk(c.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录和非JSON文件
		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".json" {
			return nil
		}

		// 加载并处理配置文件
		cfg, err := loadConfigFile(path)
		if err != nil {
			log.Printf("Error loading config file %s: %v", path, err)
			return nil
		}

		// 查找对应的处理器
		handler, exists := c.handlers[cfg.Kind]
		if !exists {
			log.Printf("No handler found for kind: %s", cfg.Kind)
			return nil
		}

		// 执行调谐
		result, err := handler.Reconcile(ctx, cfg)
		if err != nil {
			log.Printf("Reconciliation error for %s: %v", path, err)
		}

		if result.Status != nil {
			log.Printf("Reconciliation status for %s: %s", path, result.Status.Phase)
		}

		return nil
	})

	if err != nil {
		log.Printf("Error walking config directory: %v", err)
	}
}

// loadConfigFile 从文件中加载资源配置
func loadConfigFile(path string) (*config.ResourceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg config.ResourceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
