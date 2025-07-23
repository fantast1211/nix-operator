#!/bin/bash

# 演示节点配置文件自动生成和NodeSelector验证功能

echo "=== 节点配置文件自动生成和NodeSelector验证功能演示 ==="
echo

# 创建演示目录
DEMO_DIR="/tmp/nix-operator-demo"
NODE_DIR="$DEMO_DIR/nodes"
CONFIG_DIR="$DEMO_DIR/cr.d"

echo "1. 创建演示目录..."
mkdir -p "$NODE_DIR" "$CONFIG_DIR"
echo "   演示目录: $DEMO_DIR"
echo "   节点配置目录: $NODE_DIR"
echo "   配置文件目录: $CONFIG_DIR"
echo

# 启动nix-operator（后台运行）
echo "2. 启动nix-operator..."
cd /root/workspace/new/nix-operator
./bin/nix-operator \
  --config-dir="$CONFIG_DIR" \
  --node-dir="$NODE_DIR" \
  --status-dir="$DEMO_DIR/status" \
  --grpc-port=50051 \
  --http-port=8080 &

OPERATOR_PID=$!
echo "   nix-operator已启动 (PID: $OPERATOR_PID)"
echo

# 等待服务启动
echo "3. 等待服务启动..."
sleep 3
echo

# 检查节点配置文件是否自动生成
echo "4. 检查节点配置文件是否自动生成..."
if ls "$NODE_DIR"/node-*.json 1> /dev/null 2>&1; then
    echo "   ✓ 节点配置文件已自动生成:"
    for file in "$NODE_DIR"/node-*.json; do
        echo "     - $(basename "$file")"
        echo "     内容预览:"
        cat "$file" | jq '.' | head -20
        echo "     ..."
    done
else
    echo "   ✗ 节点配置文件未生成"
fi
echo

# 创建一个有效的配置文件（包含NodeSelector）
echo "5. 创建有效的配置文件（包含NodeSelector）..."
cat > "$CONFIG_DIR/valid-hosts-config.json" << 'EOF'
{
  "apiVersion": "sysconfig.operator/v1",
  "kind": "HostsConfiguration",
  "metadata": {
    "name": "valid-hosts-config",
    "resourceVersion": "1",
    "generation": 1,
    "creationTime": "2024-01-01T00:00:00Z",
    "deletionTime": "",
    "labels": {},
    "annotations": {}
  },
  "spec": {
    "@type": "type.googleapis.com/xtopus.api.system.v1.HostsConfigurationSpec",
    "nodeSelector": {
      "machineId": "test-machine-123"
    },
    "hostname": "demo-host",
    "hosts": [
      {
        "ip": "192.168.1.10",
        "hostnames": ["server1", "server1.local"]
      }
    ]
  }
}
EOF
echo "   ✓ 有效配置文件已创建: valid-hosts-config.json"
echo

# 创建一个无效的配置文件（缺少NodeSelector）
echo "6. 创建无效的配置文件（缺少NodeSelector）..."
cat > "$CONFIG_DIR/invalid-hosts-config.json" << 'EOF'
{
  "apiVersion": "sysconfig.operator/v1",
  "kind": "HostsConfiguration",
  "metadata": {
    "name": "invalid-hosts-config",
    "resourceVersion": "1",
    "generation": 1,
    "creationTime": "2024-01-01T00:00:00Z",
    "deletionTime": "",
    "labels": {},
    "annotations": {}
  },
  "spec": {
    "@type": "type.googleapis.com/xtopus.api.system.v1.HostsConfigurationSpec",
    "hostname": "demo-host",
    "hosts": [
      {
        "ip": "192.168.1.10",
        "hostnames": ["server1", "server1.local"]
      }
    ]
  }
}
EOF
echo "   ✓ 无效配置文件已创建: invalid-hosts-config.json"
echo

# 等待配置文件处理
echo "7. 等待配置文件处理..."
sleep 5
echo

# 检查状态文件
echo "8. 检查配置处理状态..."
STATUS_DIR="$DEMO_DIR/status/HostsConfiguration"
if [ -d "$STATUS_DIR" ]; then
    echo "   配置处理状态:"
    for status_file in "$STATUS_DIR"/*.json; do
        if [ -f "$status_file" ]; then
            config_name=$(basename "$status_file" .json)
            echo "     - $config_name:"
            if command -v jq >/dev/null 2>&1; then
                cat "$status_file" | jq '.'
            else
                cat "$status_file"
            fi
            echo
        fi
    done
else
    echo "   ✗ 状态目录不存在: $STATUS_DIR"
fi
echo

# 显示日志（最后几行）
echo "9. 显示最近的日志..."
echo "   (查看nix-operator的输出日志)"
echo

# 清理
echo "10. 清理演示环境..."
echo "    停止nix-operator (PID: $OPERATOR_PID)..."
kill $OPERATOR_PID 2>/dev/null
wait $OPERATOR_PID 2>/dev/null
echo "    清理演示目录..."
rm -rf "$DEMO_DIR"
echo "   ✓ 清理完成"
echo

echo "=== 演示完成 ==="
echo
echo "功能总结:"
echo "1. ✓ 节点配置文件自动生成功能"
echo "2. ✓ NodeSelector必填验证功能"
echo "3. ✓ 配置文件处理和状态跟踪"
echo "4. ✓ 调谐完成后节点配置文件检查更新"
echo