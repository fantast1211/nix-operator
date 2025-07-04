package domain

import (
	"encoding/json"
	"time"
)

// OSInfo 操作系统信息
type OSInfo struct {
	ID         string // 发行版ID，如 "ubuntu", "centos"
	VersionID  string // 发行版版本，如 "20.04", "7"
	KernelName string // 内核名称，如 "Linux"
	KernelVer  string // 内核版本
}

// Resource 资源配置领域模型
type Resource struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   Metadata        `json:"metadata"`
	Spec       json.RawMessage `json:"spec"`
}

// ResourceConfig 资源配置
type ResourceConfig struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   Metadata        `json:"metadata"`
	Spec       json.RawMessage `json:"spec"`
}

// ResourceWithStatus 带状态的资源
type ResourceWithStatus struct {
	Resource *Resource       `json:"resource"`
	Status   *ResourceStatus `json:"status"`
}

// Metadata 元数据
type Metadata struct {
	Name            string            `json:"name"`
	ResourceVersion string            `json:"resourceVersion"`
	Generation      int               `json:"generation"`
	CreationTime    *time.Time        `json:"creationTime,omitempty"`
	DeletionTime    *time.Time        `json:"deletionTime,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
}

// ResourceStatus 资源状态
type ResourceStatus struct {
	Phase   string `json:"phase"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ReconcileResult 调谐结果
type ReconcileResult struct {
	Effective *ResourceConfig `json:"effective"`
	Status    *ResourceStatus `json:"status"`
}

// HostsConfigurationSpec hosts配置规格
type HostsConfigurationSpec struct {
	Hosts []HostEntry `json:"hosts"`
}

// HostEntry 主机条目
type HostEntry struct {
	IP        string   `json:"ip"`
	Hostnames []string `json:"hostnames"`
}

// TimeConfigurationSpec 时间配置规格
type TimeConfigurationSpec struct {
	Timezone string   `json:"timezone"`
	Servers  []string `json:"servers,omitempty"`
}