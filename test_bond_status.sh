#!/bin/bash

# Bond接口状态测试脚本
# 这是一个非侵入性测试，只读取当前系统状态，不修改任何配置

echo "=== Bond接口状态测试开始 ==="
echo "时间: $(date)"
echo

# 检查当前网络接口
echo "1. 当前网络接口状态:"
ip link show 2>/dev/null || echo "无法获取网络接口信息"
echo

# 检查bond接口
echo "2. 当前Bond接口:"
ip link show type bond 2>/dev/null || echo "未发现Bond接口"
echo

# 检查bonding模块
echo "3. Bonding内核模块状态:"
if lsmod | grep -q bonding; then
    echo "Bonding模块已加载"
    lsmod | grep bonding
else
    echo "Bonding模块未加载"
fi
echo

# 检查NetworkManager状态
echo "4. NetworkManager服务状态:"
if systemctl is-active NetworkManager >/dev/null 2>&1; then
    echo "NetworkManager服务运行中"
    systemctl status NetworkManager --no-pager -l | head -10
else
    echo "NetworkManager服务未运行"
fi
echo

# 检查传统网络服务
echo "5. 传统网络服务状态:"
if systemctl is-active network >/dev/null 2>&1; then
    echo "Network服务运行中"
else
    echo "Network服务未运行或不存在"
fi
echo

# 检查网络配置目录
echo "6. 网络配置目录:"
if [ -d "/etc/sysconfig/network-scripts" ]; then
    echo "传统网络脚本目录存在: /etc/sysconfig/network-scripts"
    ls -la /etc/sysconfig/network-scripts/ifcfg-bond* 2>/dev/null || echo "未发现bond配置文件"
else
    echo "传统网络脚本目录不存在"
fi
echo

if [ -d "/etc/NetworkManager/system-connections" ]; then
    echo "NetworkManager连接目录存在: /etc/NetworkManager/system-connections"
    ls -la /etc/NetworkManager/system-connections/*bond* 2>/dev/null || echo "未发现bond连接文件"
else
    echo "NetworkManager连接目录不存在"
fi
echo

# 检查路由表
echo "7. 当前路由表:"
ip route show 2>/dev/null || echo "无法获取路由信息"
echo

# 检查DNS配置
echo "8. DNS配置:"
if [ -f "/etc/resolv.conf" ]; then
    echo "DNS配置文件内容:"
    cat /etc/resolv.conf | grep -v "^#" | grep -v "^$"
else
    echo "DNS配置文件不存在"
fi
echo

# 检查可用的网络管理工具
echo "9. 可用的网络管理工具:"
echo -n "nmcli: "
if command -v nmcli >/dev/null 2>&1; then
    echo "可用 ($(nmcli --version | head -1))"
else
    echo "不可用"
fi

echo -n "ifup/ifdown: "
if command -v ifup >/dev/null 2>&1 && command -v ifdown >/dev/null 2>&1; then
    echo "可用"
else
    echo "不可用"
fi

echo -n "ip命令: "
if command -v ip >/dev/null 2>&1; then
    echo "可用 ($(ip -V 2>&1))"
else
    echo "不可用"
fi
echo

# 检查系统信息
echo "10. 系统信息:"
echo "操作系统: $(cat /etc/os-release | grep PRETTY_NAME | cut -d'=' -f2 | tr -d '"')"
echo "内核版本: $(uname -r)"
echo

echo "=== Bond接口状态测试完成 ==="
echo "注意: 此测试为只读测试，未修改任何系统配置"