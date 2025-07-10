#!/bin/bash

set -e

backup_dir="/etc/NetworkManager/system-connections.bak-20250710-2130"
target_dir="/etc/NetworkManager/system-connections"

if [ ! -d "$backup_dir" ]; then
    echo "❌ 找不到备份目录: $backup_dir"
    exit 1
fi

echo "🧨 正在清空现有配置..."
rm -f "$target_dir"/*.nmconnection

echo "🔁 恢复备份文件..."
cp -a "$backup_dir"/*.nmconnection "$target_dir"/

echo "🔄 重新加载 NetworkManager 配置..."
nmcli connection reload

echo "✅ 还原完成。"