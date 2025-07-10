#!/bin/bash

# 输入参数：接口名（如 eth0）
IFACE="$1"

if [ -z "$IFACE" ]; then
  echo "❌ 请输入接口名称，例如：$0 eth0"
  exit 1
fi

# 获取 IPv4 地址和 CIDR（如 192.168.1.100/24）
IPV4_CIDR=$(ip -4 -o addr show "$IFACE" | awk '{print $4}')
IPV4_ADDR=${IPV4_CIDR%%/*}
PREFIX_LEN=${IPV4_CIDR##*/}

# 将前缀长度转换为子网掩码（如 24 -> 255.255.255.0）
prefix_to_netmask() {
  local i mask=""
  local full_octets=$((PREFIX_LEN / 8))
  local remaining_bits=$((PREFIX_LEN % 8))

  for ((i = 0; i < 4; i++)); do
    if ((i < full_octets)); then
      mask+=255
    elif ((i == full_octets)); then
      mask+=$((256 - 2 ** (8 - remaining_bits)))
    else
      mask+=0
    fi
    [[ $i -lt 3 ]] && mask+=.
  done

  echo "$mask"
}

# 如果获取不到 PREFIX_LEN（接口没配置 IP），NETMASK 置空
if [[ "$PREFIX_LEN" =~ ^[0-9]+$ ]]; then
  NETMASK=$(prefix_to_netmask)
else
  NETMASK=""
fi

# 获取 IPv6 地址（省略 scope link 的）
IPV6_ADDR=$(ip -6 addr show "$IFACE" | grep -v 'scope link' | grep -oP '(?<=inet6\s)[0-9a-fA-F:]+')

# 获取 IPv4 默认网关
IPV4_GATEWAY=$(ip route | grep "^default" | grep "$IFACE" | awk '{print $3}')

# 获取 IPv6 默认网关
IPV6_GATEWAY=$(ip -6 route show default | grep "$IFACE" | awk '{print $3}')

# 获取 MTU
MTU=$(cat "/sys/class/net/$IFACE/mtu")

# 获取 DNS（从 /etc/resolv.conf）
NAMESERVERS=$(grep '^nameserver' /etc/resolv.conf | awk '{print $2}' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+')

# 输出 JSON
echo "{"
echo "  \"name\": \"$IFACE\","
echo "  \"ipAddress\": \"${IPV4_ADDR:-}\","
echo "  \"netmask\": \"${NETMASK:-}\","
echo "  \"ipv6Address\": \"${IPV6_ADDR:-}\","
echo "  \"gateway\": \"${IPV4_GATEWAY:-}\","
echo "  \"ipv6Gateway\": \"${IPV6_GATEWAY:-}\","
echo "  \"mtu\": ${MTU:-0},"
echo -n "  \"nameservers\": ["
FIRST=true
for ns in $NAMESERVERS; do
  if $FIRST; then
    FIRST=false
  else
    echo -n ", "
  fi
  echo -n "\"$ns\""
done
echo "]"
echo "}"
