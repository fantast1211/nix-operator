package utils

import (
	"fmt"
	"os"
	"strings"
)

type NodeSelector struct {
	MachineID string `yaml:"machine-id" json:"machine-id"`
}

func MatchNodeSelector(selector NodeSelector) (bool, error) {
	// 如果没有指定MachineID，则认为匹配所有节点
	if selector.MachineID == "" {
		return true, nil
	}

	// 读取本机的machine-id
	machineID, err := readMachineID()
	if err != nil {
		return false, fmt.Errorf("failed to get machine ID: %v", err)
	}

	// 比较machine-id是否匹配
	return machineID == selector.MachineID, nil
}

// 读取机器ID的函数
func readMachineID() (string, error) {
	// 从/etc/machine-id读取
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return "", fmt.Errorf("failed to read machine ID: %v", err)
	}

	return strings.TrimSpace(string(data)), nil
}
