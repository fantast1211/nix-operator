#!/bin/bash

set -e

# 生成 proto 文件，项目的根目录执行
protoc \
  -I api \
  -I api/third_party \
  --go_out=api/system/v1/ \
  --go-grpc_out=api/system/v1/ \
  --grpc-gateway_out=api/system/v1/ \
  api/system/v1/resource.proto

echo "Proto 文件生成完成！"
echo "- Go 代码已生成到 api/system/v1/ 目录"
echo "- gRPC Gateway 代码已生成"