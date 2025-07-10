#!/bin/bash

echo "🔍 正在检查系统中安装的网络管理工具..."

check_command() {
    local name="$1"
    if command -v "$name" &>/dev/null; then
        echo "✅ $name: 已安装"
        return 0
    else
        echo "❌ $name: 未安装"
        return 1
    fi
}

check_package() {
    local pkg="$1"
    if dpkg -s "$pkg" &>/dev/null || rpm -q "$pkg" &>/dev/null; then
        echo "✅ $pkg: 已安装"
    else
        echo "❌ $pkg: 未安装"
    fi
}

check_enabled() {
    local service="$1"
    if systemctl list-unit-files | grep -q "$service"; then
        if systemctl is-enabled "$service" &>/dev/null; then
            echo "🟢 $service: 已启用（默认）"
        else
            echo "⚪ $service: 未启用"
        fi
    else
        echo "⚫ $service: 未安装或未注册为服务"
    fi
}

echo "==============================="
echo ">> NetworkManager:"
check_command NetworkManager || check_package NetworkManager
check_enabled NetworkManager.service

echo "配置文件检查:"
[[ -f /etc/NetworkManager/NetworkManager.conf ]] && echo "📄 存在: /etc/NetworkManager/NetworkManager.conf" || echo "🛑 缺失: /etc/NetworkManager/NetworkManager.conf"

echo "==============================="
echo ">> Netplan:"
check_command netplan || check_package netplan.io
echo "配置文件检查:"
if ls /etc/netplan/*.yaml &>/dev/null; then
    echo "📄 存在: /etc/netplan/*.yaml"
else
    echo "🛑 缺失: /etc/netplan/*.yaml"
fi

echo "==============================="
echo ">> ifupdown:"
check_command ifup || check_package ifupdown
echo "配置文件检查:"
[[ -f /etc/network/interfaces ]] && echo "📄 存在: /etc/network/interfaces" || echo "🛑 缺失: /etc/network/interfaces"

echo "==============================="
echo ">> systemd-networkd:"
check_enabled systemd-networkd.service
[[ -f /etc/systemd/network/*.network ]] && echo "📄 存在: /etc/systemd/network/*.network" || echo "🛑 缺失: /etc/systemd/network/*.network"

echo "==============================="
echo "✅ 检查完成。"