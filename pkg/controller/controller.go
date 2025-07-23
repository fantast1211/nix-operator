package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"go.xbrother.com/nix-operator/pkg/domain"
	"go.xbrother.com/nix-operator/pkg/repository"
	"go.xbrother.com/nix-operator/pkg/utils"

	"github.com/fsnotify/fsnotify"
)

type OSInfo struct {
	ID         string // 发行版ID，如 "ubuntu", "centos"
	VersionID  string // 发行版版本，如 "20.04", "7"
	KernelName string // 内核名称，如 "Linux"
	KernelVer  string // 内核版本
}

type Controller struct {
	configDir  string
	nodeDir    string
	Handlers   map[string]Handler // key 是处理器类型，公开字段供外部访问
	osInfo     OSInfo
	statusRepo repository.StatusRepository
	logger     *utils.Logger
}

type Handler interface {
	// Match 检查是否支持该操作系统
	Match(osInfo OSInfo) bool
	// Reconcile 处理配置
	Reconcile(ctx context.Context, config []*systemv1.ResourceConfig) ([]*domain.ReconcileResult, error)
}

var handlerFactories = make(map[string][]Handler)

// RegisterHandler 注册处理器工厂
func RegisterHandler(typeName string, handler Handler) {
	handlerFactories[typeName] = append(handlerFactories[typeName], handler)
}

func NewController(configDir string, nodeDir string, statusRepo repository.StatusRepository, logger *utils.Logger) (*Controller, error) {
	osInfo, err := getOSInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get OS info: %v", err)
	}

	handlers := make(map[string]Handler)

	// 为每种类型选择合适的处理器
	requiredTypes := []string{
		"BondConfiguration",
		"HostsConfiguration",
		"TimeConfiguration",
		"NetworkConfiguration",
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
		configDir:  configDir,
		nodeDir:    nodeDir,
		Handlers:   handlers,
		osInfo:     osInfo,
		statusRepo: statusRepo,
		logger:     logger,
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
				// 只监听配置目录的写入事件，忽略状态目录的变化
				if event.Op&fsnotify.Write == fsnotify.Write && strings.Contains(event.Name, "/cr.d/") {
					c.logger.Debugf("controller", "Config file changed: %s", event.Name)
					time.Sleep(100 * time.Millisecond) // 等待写入完成
					c.reconcile()
				}
			case err := <-watcher.Errors:
				c.logger.Errorf("controller", "File watcher error: %v", err)
			}
		}
	}()

	// 只监控配置目录（cr.d），不监控状态目录
	err = watcher.Add(c.configDir)
	if err != nil {
		return err
	}
	c.logger.Infof("controller", "Watching config directory: %s", c.configDir)

	// 递归监控配置目录的子目录，但排除status目录
	err = filepath.Walk(c.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// 排除status目录，避免状态更新触发配置监听
			if strings.Contains(path, "/status") {
				c.logger.Debugf("controller", "Skipping status directory: %s", path)
				return filepath.SkipDir
			}
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

// GetHandler 获取指定kind的处理器
func (c *Controller) GetHandler(kind string) Handler {
	return c.Handlers[kind]
}

// filterConfigsForReconcile 过滤需要调谐的配置（基于generation判断）
func (c *Controller) filterConfigsForReconcile(ctx context.Context, kind string, configs []*systemv1.ResourceConfig) []*systemv1.ResourceConfig {
	var configsToReconcile []*systemv1.ResourceConfig

	for _, config := range configs {
		resourceName := config.Metadata.Name
		currentGeneration := config.Metadata.Generation

		// 获取当前状态
		status, err := c.statusRepo.GetStatus(ctx, resourceName)
		if err != nil || status == nil {
			// 状态不存在，首次调谐
			c.logger.Debugf("controller", "Config %s/%s: no status found, will reconcile (first time)", kind, resourceName)
			configsToReconcile = append(configsToReconcile, config)
			continue
		}

		// 比较generation
		if currentGeneration > status.ObservedGeneration {
			c.logger.Debugf("controller", "Config %s/%s: generation %d > observed %d, will reconcile",
				kind, resourceName, currentGeneration, status.ObservedGeneration)
			configsToReconcile = append(configsToReconcile, config)
		} else {
			c.logger.Debugf("controller", "Config %s/%s: generation %d <= observed %d, skipping",
				kind, resourceName, currentGeneration, status.ObservedGeneration)
		}
	}

	return configsToReconcile
}

func (c *Controller) reconcile() {
	ctx := context.Background()
	grouped := make(map[string][]*systemv1.ResourceConfig)

	// 先聚合所有配置文件
	filepath.Walk(c.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		cfg, err := utils.LoadConfigFile(path)
		if err != nil {
			c.logger.Errorf("controller", "Failed to load config %s: %v", path, err)
			return nil
		}
		grouped[cfg.Kind] = append(grouped[cfg.Kind], cfg)
		return nil
	})

	// 每种类型只调一次
	for kind, configs := range grouped {
		handler := c.Handlers[kind]
		if handler == nil {
			c.logger.Warnf("controller", "No handler for kind: %s", kind)
			continue
		}

		// 过滤需要调谐的配置（基于generation判断）
		configsToReconcile := c.filterConfigsForReconcile(ctx, kind, configs)
		if len(configsToReconcile) == 0 {
			c.logger.Debugf("controller", "No %s configs need reconciliation (all up-to-date)", kind)
			continue
		}

		c.logger.Infof("controller", "Reconciling %d/%d %s configs", len(configsToReconcile), len(configs), kind)
		results, err := handler.Reconcile(ctx, configsToReconcile)
		if err != nil {
			c.logger.Errorf("controller", "Reconciliation error for kind %s: %v", kind, err)
			// 为所有配置设置错误状态
			for _, config := range configs {
				errorStatus := &systemv1.ResourceStatus{
					Phase:   "Error",
					Reason:  "ReconcileError",
					Message: fmt.Sprintf("Reconciliation failed: %v", err),
				}
				if statusErr := c.statusRepo.SetStatusWithKind(ctx, config.Metadata.Name, kind, errorStatus); statusErr != nil {
					c.logger.Errorf("controller", "Failed to update error status for %s: %v", config.Metadata.Name, statusErr)
				}
			}
		} else if len(results) > 0 {
			// 更新状态缓存
			for _, result := range results {
				if result != nil && result.Status != nil && result.Effective != nil && result.Effective.Metadata != nil {
					resourceName := result.Effective.Metadata.Name

					// 调谐成功时更新ObservedGeneration
					if result.Status.Phase == "Ready" {
						result.Status.ObservedGeneration = result.Effective.Metadata.Generation
						c.logger.Debugf("controller", "Updated ObservedGeneration for %s to %d", resourceName, result.Status.ObservedGeneration)
					}

					if statusErr := c.statusRepo.SetStatusWithKind(ctx, resourceName, kind, result.Status); statusErr != nil {
						c.logger.Errorf("controller", "Failed to update status for %s: %v", resourceName, statusErr)
					} else {
						c.logger.Debugf("controller", "Updated status cache for %s: %s (%s)", resourceName, result.Status.Phase, result.Status.Reason)
					}

					// 记录日志
					if result.Status.Phase == "Error" && result.Status.Message != "" {
						c.logger.Errorf("controller", "Reconciliation result: %s/%s -> %s (Reason: %s, Message: %s)",
							kind, resourceName, result.Status.Phase, result.Status.Reason, result.Status.Message)
					} else {
						c.logger.Infof("controller", "Reconciliation result: %s/%s -> %s (%s)",
							kind, resourceName, result.Status.Phase, result.Status.Reason)
					}
				}
			}
		}
	}

	// 调谐完成后，检查并更新节点配置文件
	if err := utils.CheckAndUpdateNodeConfig(c.nodeDir); err != nil {
		c.logger.Errorf("controller", "Failed to check and update node config: %v", err)
	}
}
