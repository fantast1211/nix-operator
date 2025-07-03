package controller

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"go.xbrother.com/nix-operator/pkg/config"

	"github.com/fsnotify/fsnotify"
)

// Controller 是系统配置控制器，负责管理和调谐系统配置
type Controller struct {
	configDir string
	handlers  map[string]Handler // key 是处理器类型
	osInfo    OSInfo
}

// Handler 是配置处理器接口，负责处理特定类型的配置
type Handler interface {
	// Match 检查是否支持该操作系统
	Match(osInfo OSInfo) bool
	// Reconcile 处理配置
	Reconcile(ctx context.Context, config *config.ResourceConfig) (*ReconcileResult, error)
}

var handlerFactories = make(map[string][]Handler)

// RegisterHandler 注册处理器工厂
func RegisterHandler(typeName string, handler Handler) {
	handlerFactories[typeName] = append(handlerFactories[typeName], handler)
}

// NewController 创建一个新的控制器实例
func NewController(configDir string) (*Controller, error) {
	osInfo, err := getOSInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get OS info: %v", err)
	}

	handlers := make(map[string]Handler)

	// 为每种类型选择合适的处理器
	requiredTypes := []string{
		"NetworkConfiguration",
		"HostsConfiguration",
		"TimeConfiguration",
		"serial",
		"udev",
	}
	for _, typeName := range requiredTypes {
		typedHandlers := handlerFactories[typeName]
		if len(typedHandlers) == 0 {
			log.Printf("Warning: no handler registered for type: %s", typeName)
			continue
		}

		// 查找匹配的处理器
		var matched bool
		for _, handler := range typedHandlers {
			if handler.Match(osInfo) {
				handlers[typeName] = handler
				matched = true
				break
			}
		}

		if !matched {
			log.Printf("Warning: no compatible handler found for type %s on OS %s %s",
				typeName, osInfo.ID, osInfo.VersionID)
		}
	}

	return &Controller{
		configDir: configDir,
		handlers:  handlers,
		osInfo:    osInfo,
	}, nil
}

// Run 启动控制器，监控配置目录并处理配置变更
func (c *Controller) Run() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					c.reconcile()
				}
			case err := <-watcher.Errors:
				log.Printf("Error: %v", err)
			}
		}
	}()

	// 监控配置目录
	err = watcher.Add(c.configDir)
	if err != nil {
		return err
	}

	// 递归监控子目录
	err = filepath.Walk(c.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 初始调谐
	c.reconcile()

	// 保持运行
	select {}
}
