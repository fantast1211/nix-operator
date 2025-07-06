package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	systemv1 "go.xbrother.com/nix-operator/api/system/v1"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

// AtomicWriteFile 原子性地写入文件
// content: 文件内容
// filename: 目标文件路径
// perm: 文件权限
func AtomicWriteFile(content []byte, filename string, perm os.FileMode) error {
	// 创建临时文件
	swap, err := os.CreateTemp(filepath.Dir(filename), filepath.Base(filename)+".tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	tmpName := swap.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpName) // 如果有错误发生，清理临时文件
		}
	}()

	// 写入临时文件
	if _, err = swap.Write(content); err != nil {
		return fmt.Errorf("failed to write temp file: %v", err)
	}

	// 同步文件内容到磁盘
	if err = swap.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %v", err)
	}

	// 关闭临时文件
	if err = swap.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %v", err)
	}

	// 设置正确的权限
	if err = os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("failed to chmod temp file: %v", err)
	}

	// 原子性地重命名临时文件
	if err = os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("failed to rename temp file: %v", err)
	}

	return nil
}

// isJSONFile 检查文件是否为JSON文件
func IsJSONFile(info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}

	ext := filepath.Ext(info.Name())
	return ext == ".json"
}

func LoadConfigFile(path string) (*systemv1.ResourceConfig, error) {
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
