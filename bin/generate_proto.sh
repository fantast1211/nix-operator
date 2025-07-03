#!/bin/bash

set -e

# 切换到项目根目录
cd /root/workspcae/nix-operator

# 创建 API 文档目录
mkdir -p doc/api

echo "开始生成 proto 文件..."

# 生成 resource.proto
echo "生成 resource.proto 相关文件..."
protoc \
  -I api \
  -I api/third_party \
  --go_out=. --go_opt=module=go.xbrother.com/nix-operator \
  --go-grpc_out=. --go-grpc_opt=module=go.xbrother.com/nix-operator \
  --grpc-gateway_out=. --grpc-gateway_opt=module=go.xbrother.com/nix-operator \
  --openapiv2_out=doc/api \
  --jsonschema_out=./jsonschema \
  api/system/v1/*.proto
# 生成 system.proto
# echo "生成 system.proto 相关文件..."
# protoc \
#   -I api \
#   -I api/third_party \
#   --go_out=. --go_opt=module=go.xbrother.com/nix-operator \
#   --go-grpc_out=. --go-grpc_opt=module=go.xbrother.com/nix-operator \
#   --grpc-gateway_out=. --grpc-gateway_opt=module=go.xbrother.com/nix-operator \
#   --openapiv2_out=doc/api \
#   api/system/v1/system.proto

# 移动生成的文件到正确位置
echo "整理生成的文件..."
if [ -d "go.xbrother.com/nix-operator/api/system/v1" ]; then
  cp go.xbrother.com/nix-operator/api/system/v1/*.pb.go api/system/v1/
  cp go.xbrother.com/nix-operator/api/system/v1/*.pb.gw.go api/system/v1/
  rm -rf go.xbrother.com
fi

echo "Proto 文件生成完成！"
echo "- Go 代码已生成到 api/system/v1/ 目录"
echo "- gRPC Gateway 代码已生成到 api/system/v1/ 目录"
echo "- OpenAPI 文档已生成到 doc/api/ 目录"
echo "- JSON Schema 已生成到 jsonschema/ 目录"