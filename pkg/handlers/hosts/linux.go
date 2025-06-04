package hosts

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"go.xbrother.com/nix-operator/pkg/config"
	"go.xbrother.com/nix-operator/pkg/controller"
	"go.xbrother.com/nix-operator/pkg/utils"
)

type Config struct {
	Hosts []hostEntry `json:"hosts"`
}

type HostEntry struct {
	IP        string   `json:"ip"`
	Hostnames []string `json:"hostnames"`
}

type HostsSpec struct {
	Hosts []HostEntry `json:"hosts"`
}

func init() {
	controller.RegisterHandler("HostsConfiguration", &LinuxHostsHandler{})
}

type LinuxHostsHandler struct{}

type hostEntry struct {
	IP        string
	Hostnames []string
}

func (h *LinuxHostsHandler) Match(osInfo controller.OSInfo) bool {
	return osInfo.KernelName == "Linux"
}

func (h *LinuxHostsHandler) Reconcile(ctx context.Context, cfg *config.ResourceConfig) (*controller.ReconcileResult, error) {
	// 读取现有的 hosts 文件
	currentEntries, err := h.getCurrentHosts()
	if err != nil {
		return nil, fmt.Errorf("failed to read current hosts: %v", err)
	}

	var spec HostsSpec
	if err := json.Unmarshal(cfg.Spec, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal hosts spec: %v", err)
	}
	// 转换期望的配置
	desiredEntries := make([]hostEntry, len(spec.Hosts))
	for i, host := range spec.Hosts {
		desiredEntries[i] = hostEntry{
			IP:        host.IP,
			Hostnames: host.Hostnames,
		}
	}

	// 比较现有配置和期望配置
	if h.areHostsEqual(currentEntries, desiredEntries) {
		// 配置一致，无需更新
		return nil, nil
	}

	// 生成新的 hosts 内容
	var content strings.Builder
	content.WriteString(config.CommentHeader)
	content.WriteString("127.0.0.1 localhost\n")
	content.WriteString("::1 localhost ip6-localhost ip6-loopback\n\n")

	for _, host := range desiredEntries {
		content.WriteString(fmt.Sprintf("%s %s\n", host.IP, strings.Join(host.Hostnames, " ")))
	}

	// 原子性写入文件
	utils.AtomicWriteFile([]byte(content.String()), "/etc/hosts", 0644)

	// 构造成功的ReconcileResult
	result := &controller.ReconcileResult{
		Effective: cfg,
		Status: &config.ResourceStatus{
			Phase:   "Ready",
			Reason:  "HostsUpdated",
			Message: "Successfully updated /etc/hosts file",
		},
	}

	return result, nil
}

func (h *LinuxHostsHandler) getCurrentHosts() ([]hostEntry, error) {
	file, err := os.Open("/etc/hosts")
	if err != nil {
		if os.IsNotExist(err) {
			return []hostEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var entries []hostEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 {
			entries = append(entries, hostEntry{
				IP:        fields[0],
				Hostnames: fields[1:],
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (h *LinuxHostsHandler) areHostsEqual(current, desired []hostEntry) bool {
	if len(current) != len(desired) {
		return false
	}

	// 复制切片以避免修改原始数据
	currentCopy := make([]hostEntry, len(current))
	desiredCopy := make([]hostEntry, len(desired))
	copy(currentCopy, current)
	copy(desiredCopy, desired)

	// 对每个条目的主机名进行排序
	for i := range currentCopy {
		slices.Sort(currentCopy[i].Hostnames)
	}
	for i := range desiredCopy {
		slices.Sort(desiredCopy[i].Hostnames)
	}

	// 对条目进行排序（按IP地址）
	slices.SortFunc(currentCopy, func(a, b hostEntry) int {
		return strings.Compare(a.IP, b.IP)
	})
	slices.SortFunc(desiredCopy, func(a, b hostEntry) int {
		return strings.Compare(a.IP, b.IP)
	})

	// 比较每个条目
	for i := range currentCopy {
		if currentCopy[i].IP != desiredCopy[i].IP {
			return false
		}
		if !slices.Equal(currentCopy[i].Hostnames, desiredCopy[i].Hostnames) {
			return false
		}
	}

	return true
}
