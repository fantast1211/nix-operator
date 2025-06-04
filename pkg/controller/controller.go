package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go.xbrother.com/nix-operator/pkg/config"

	"github.com/fsnotify/fsnotify"
)

type OSInfo struct {
	ID         string // 发行版ID，如 "ubuntu", "centos"
	VersionID  string // 发行版版本，如 "20.04", "7"
	KernelName string // 内核名称，如 "Linux"
	KernelVer  string // 内核版本
}

type Controller struct {
	configDir string
	handlers  map[string]Handler // key 是处理器类型
	osInfo    OSInfo
}

type ReconcileResult struct {
	Effective *config.ResourceConfig
	Status    *config.ResourceStatus
}

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

func NewController(configDir string) (*Controller, error) {
	osInfo, err := getOSInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get OS info: %v", err)
	}

	handlers := make(map[string]Handler)

	// 为每种类型选择合适的处理器
	requiredTypes := []string{
		"network",
		"HostsConfiguration",
		"time",
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

func getOSInfo() (OSInfo, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return OSInfo{}, err
	}

	info := OSInfo{}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value := strings.Trim(parts[1], "\"")
		switch parts[0] {
		case "ID":
			info.ID = value
		case "VERSION_ID":
			info.VersionID = value
		}
	}

	// 获取内核信息
	kernel, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return OSInfo{}, err
	}
	info.KernelVer = strings.TrimSpace(string(kernel))
	info.KernelName = "Linux" // 可以根据需要扩展

	return info, nil
}

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

func (c *Controller) reconcile() {
	ctx := context.Background()

	// 扫描配置目录中的所有配置文件
	err := filepath.Walk(c.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录和非YAML/JSON文件
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
