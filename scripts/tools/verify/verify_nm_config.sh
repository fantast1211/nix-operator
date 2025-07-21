#!/bin/bash

set -e

IFACE_NAME="$1"
CONFIG_FILE="/etc/NetworkManager/system-connections/nix-operator-${IFACE_NAME}.nmconnection"

echo "🔍 正在验证 NetworkManager 配置：接口 = ${IFACE_NAME}"

# 检查配置文件是否存在
if [ ! -f "$CONFIG_FILE" ]; then
  echo "❌ 配置文件不存在: $CONFIG_FILE"
  exit 1
fi

echo "✅ 配置文件存在: $CONFIG_FILE"

# 显示文件内容（可选）
echo "📄 配置文件内容:"
cat "$CONFIG_FILE"
echo ""

# 检查连接是否被 NetworkManager 识别
if ! nmcli connection show | grep -q "nix-operator-${IFACE_NAME}"; then
  echo "❌ NetworkManager 未识别该连接: nix-operator-${IFACE_NAME}"
  echo "📦 尝试重新加载配置..."
  nmcli connection reload
fi

# 再次检查
if ! nmcli connection show | grep -q "nix-operator-${IFACE_NAME}"; then
  echo "❌ 重载后仍未识别该连接"
  exit 1
fi

echo "✅ 连接已被 NetworkManager 识别"

# 激活连接
echo "🚀 激活连接..."
if ! nmcli connection up "nix-operator-${IFACE_NAME}"; then
  echo "❌ 激活失败"
  exit 1
fi

echo "✅ 连接已激活"

# 检查接口状态
echo "📡 接口状态:"
nmcli device status | grep "$IFACE_NAME"

# 检查 IP 地址是否设置成功
IP=$(ip -4 addr show "$IFACE_NAME" | grep -oP '(?<=inet\s)\d+(\.\d+){3}' || true)
if [ -z "$IP" ]; then
  echo "❌ 接口 $IFACE_NAME 未分配 IPv4 地址"
  exit 1
fi

echo "✅ 接口 $IFACE_NAME 已分配 IP: $IP"

# 测试网络连通性
echo "🌐 测试外网连通性 (ping 8.8.8.8)..."
if ping -c 3 -W 2 8.8.8.8 >/dev/null; then
  echo "✅ 能够访问外部网络"
else
  echo "⚠️ 无法访问外部网络，可能是网关或 DNS 设置问题"
fi

# 查看日志
echo "📜 最近 NetworkManager 日志:"
journalctl -u NetworkManager -n 20 --no-pager

echo "🎉 验证完成"