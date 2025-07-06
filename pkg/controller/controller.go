package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/utils"

	"github.com/fsnotify/fsnotify"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

type OSInfo struct {
	ID         string // 发行版ID，如 "ubuntu", "centos"
	VersionID  string // 发行版版本，如 "20.04", "7"
	KernelName string // 内核名称，如 "Linux"
	KernelVer  string // 内核版本
}

type Controller struct {
	configDir string
	Handlers  map[string]Handler // key 是处理器类型，公开字段供外部访问
	osInfo    OSInfo
	logger    *utils.Logger
}

type Handler interface {
	// Match 检查是否支持该操作系统
	Match(osInfo OSInfo) bool
	// Reconcile 处理配置
	Reconcile(ctx context.Context, config *systemv1.ResourceConfig) (*domain.ReconcileResult, error)
}

var handlerFactories = make(map[string][]Handler)

// RegisterHandler 注册处理器工厂
func RegisterHandler(typeName string, handler Handler) {
	handlerFactories[typeName] = append(handlerFactories[typeName], handler)
}

func NewController(configDir string, logger *utils.Logger) (*Controller, error) {
	osInfo, err := getOSInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get OS info: %v", err)
	}

	handlers := make(map[string]Handler)

	// 为每种类型选择合适的处理器
	requiredTypes := []string{
		"HostsConfiguration",
		"TimeConfiguration",
	}
	for _, typeName := range requiredTypes {
		typedHandlers := handlerFactories[typeName]
		if len(typedHandlers) == 0 {
			logger.Warnf("controller", "No handler registered for type: %s", typeName)
			continue
		}

		// 查找匹配的处理器
		var matched bool
		for _, handler := range typedHandlers {
			if handler.Match(osInfo) {
				handlers[typeName] = handler
				matched = true
				logger.Infof("controller", "Registered handler for type: %s", typeName)
				break
			}
		}

		if !matched {
			logger.Warnf("controller", "No compatible handler found for type %s on OS %s %s",
				typeName, osInfo.ID, osInfo.VersionID)
		}
	}

	logger.Infof("controller", "Controller initialized with OS: %s %s, kernel: %s",
		osInfo.ID, osInfo.VersionID, osInfo.KernelVer)

	return &Controller{
		configDir: configDir,
		Handlers:  handlers,
		osInfo:    osInfo,
		logger:    logger,
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

	c.logger.Info("controller", "Starting file system watcher...")
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					c.logger.Debugf("controller", "Config file changed: %s", event.Name)
					c.reconcile()
				}
			case err := <-watcher.Errors:
				c.logger.Errorf("controller", "File watcher error: %v", err)
			}
		}
	}()

	// 监控配置目录
	err = watcher.Add(c.configDir)
	if err != nil {
		return err
	}
	c.logger.Infof("controller", "Watching config directory: %s", c.configDir)

	// 递归监控子目录
	err = filepath.Walk(c.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			c.logger.Debugf("controller", "Adding directory to watch: %s", path)
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 初始调谐
	c.logger.Info("controller", "Starting initial reconciliation...")
	c.reconcile()

	c.logger.Info("controller", "Controller is running and watching for changes...")
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
			c.logger.Errorf("controller", "Error loading config file %s: %v", path, err)
			return nil
		}

		c.logger.Debugf("controller", "Processing config file: %s (kind: %s)", path, cfg.Kind)

		// 查找对应的处理器
		handler, exists := c.Handlers[cfg.Kind]
		if !exists {
			c.logger.Warnf("controller", "No handler found for kind: %s", cfg.Kind)
			return nil
		}

		// 执行调谐
		result, err := handler.Reconcile(ctx, cfg)
		if err != nil {
			c.logger.Errorf("controller", "Reconciliation error for %s: %v", path, err)
			return nil
		}

		if result != nil && result.Status != nil {
			c.logger.Infof("controller", "Reconciliation completed for %s: %s", path, result.Status.Phase)
		}

		return nil
	})

	if err != nil {
		c.logger.Errorf("controller", "Error walking config directory: %v", err)
	}
}

func loadConfigFile(path string) (*systemv1.ResourceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var tempConfig struct {
		APIVersion string                 `json:"apiVersion"`
		Kind       string                 `json:"kind"`
		Metadata   *systemv1.Metadata     `json:"metadata"`
		Spec       map[string]interface{} `json:"spec"`
	}

	if err := json.Unmarshal(data, &tempConfig); err != nil {
		return nil, err
	}

	cfg := &systemv1.ResourceConfig{
		ApiVersion: tempConfig.APIVersion,
		Kind:       tempConfig.Kind,
		Metadata:   tempConfig.Metadata,
	}

	// 封装为 structpb.Struct，保持为弱类型，延迟处理
	if tempConfig.Spec != nil {
		specStruct, err := structpb.NewStruct(tempConfig.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to convert spec to structpb.Struct: %w", err)
		}
		cfg.Spec, err = anypb.New(specStruct)
		if err != nil {
			return nil, fmt.Errorf("failed to wrap spec struct as Any: %w", err)
		}
	}

	return cfg, nil
}
