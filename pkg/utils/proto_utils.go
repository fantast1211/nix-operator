package utils

import (
	"encoding/json"
	"fmt"
	"reflect"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

func UnmarshalSpec[T proto.Message](anySpec *anypb.Any) (T, error) {
	var zero T
	if anySpec == nil {
		return zero, fmt.Errorf("spec is nil")
	}

	// 创建目标类型的实例
	msg := reflect.New(reflect.TypeOf(zero).Elem()).Interface().(proto.Message)

	// 直接尝试解包为目标类型
	if err := anySpec.UnmarshalTo(msg); err != nil {
		// 如果直接解包失败，尝试通过Struct中转
		var structSpec structpb.Struct
		if err2 := anySpec.UnmarshalTo(&structSpec); err2 != nil {
			return zero, fmt.Errorf("failed to unmarshal Any to target type: %w", err)
		}

		// 将 structpb.Struct 转换为 JSON，并过滤掉 @type 字段
		structMap := structSpec.AsMap()
		// 移除 @type 字段，因为它不属于目标 proto 消息的字段
		delete(structMap, "@type")

		jsonBytes, err := json.Marshal(structMap)
		if err != nil {
			return zero, fmt.Errorf("failed to marshal struct to JSON: %w", err)
		}

		// 使用 protojson 将 JSON 解析为 proto 消息
		if err := protojson.Unmarshal(jsonBytes, msg); err != nil {
			return zero, fmt.Errorf("failed to unmarshal JSON to proto: %w", err)
		}
	}

	return msg.(T), nil
}
