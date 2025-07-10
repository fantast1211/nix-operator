#!/bin/bash

set -e

src_dir="/etc/NetworkManager/system-connections"
backup_dir="/etc/NetworkManager/system-connections.bak-$(date +%Y%m%d-%H%M%S)"

echo "🔄 正在备份 NetworkManager 配置文件..."
if [ ! -d "$src_dir" ]; then
    echo "❌ 目录不存在: $src_dir"
    exit 1
fi

cp -a "$src_dir" "$backup_dir"
echo "✅ 备份完成：$backup_dir"
