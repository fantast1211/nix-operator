package utils

import (
	"fmt"
	"net"
	"os"
	"strings"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
)

func MatchNodeSelector(selector *systemv1.NodeSelector) (bool, error) {
	// 如果没有指定MachineID，则认为匹配所有节点
	if selector.MachineId == "" {
		return true, nil
	}

	// 如果指定了MachineID，检查是否匹配
	if selector.MachineId != "" {
		// 读取本机的machine-id
		machineID, err := readMachineID()
		if err != nil {
			return false, fmt.Errorf("failed to get machine ID: %v", err)
		}

		// 如果machine-id匹配，直接返回true
		if machineID == selector.MachineId {
			return true, nil
		}
	}

	// 不匹配machine-id
	return false, nil
}

// 读取机器ID的函数
func readMachineID() (string, error) {
	// 从/etc/machine-id读取
	data, err := os.ReadFile("/etc/xrocket.machine.id")
	if err != nil {
		return "", fmt.Errorf("failed to read machine ID: %v", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// ReadMachineID 导出的读取机器ID函数
func ReadMachineID() (string, error) {
	return readMachineID()
}

// GetLocalIPs 获取本机所有非回环IP地址
func GetLocalIPs() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %v", err)
	}

	var ips []string
	for _, iface := range interfaces {
		// 跳过禁用的接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// 跳过回环接口
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}

			// 跳过IPv6地址
			if ip.To4() == nil {
				continue
			}

			ips = append(ips, ip.String())
		}
	}

	return ips, nil
}

// GetPrimaryIP 获取主要IP地址（通常是第一个非回环IPv4地址）
func GetPrimaryIP() (string, error) {
	ips, err := GetLocalIPs()
	if err != nil {
		return "", err
	}

	if len(ips) > 0 {
		return ips[0], nil
	}

	return "", fmt.Errorf("no available IP addresses found")
}

// GetHostname 获取主机名
func GetHostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %v", err)
	}
	return hostname, nil
}
