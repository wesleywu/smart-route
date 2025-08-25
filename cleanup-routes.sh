#!/bin/bash

echo "🧹 智能路由清理工具"
echo "===================="

# 检查权限
if [[ $EUID -ne 0 ]]; then
   echo "❌ 需要root权限运行此脚本"
   echo "请使用: sudo $0"
   exit 1
fi

# 获取当前网关
echo "📊 检查当前网络状态..."
CURRENT_GW=$(./smartroute version 2>/dev/null | grep "Current Gateway:" | cut -d' ' -f3)
echo "当前网关: $CURRENT_GW"

# 显示现有中国路由数量
echo ""
echo "📈 统计现有路由..."

# 常见网关列表
GATEWAYS=("192.168.1.1" "192.168.0.1" "192.168.2.1" "192.168.10.1" "192.168.31.1" "192.168.32.1" "192.168.100.1" "10.0.0.1" "10.0.1.1" "10.1.1.1" "172.16.0.1" "172.16.1.1")

total_routes=0
for gw in "${GATEWAYS[@]}"; do
    count=$(netstat -rn | grep "$gw" | wc -l | tr -d ' ')
    if [[ $count -gt 0 ]]; then
        echo "网关 $gw: $count 条路由"
        total_routes=$((total_routes + count))
    fi
done

echo "总计: $total_routes 条可能的中国路由"

if [[ $total_routes -eq 0 ]]; then
    echo "✅ 没有发现需要清理的路由"
    exit 0
fi

echo ""
read -p "❓ 是否清理这些旧的路由规则? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "❌ 操作已取消"
    exit 0
fi

echo ""
echo "🧹 开始清理路由..."

# 备份当前路由表
echo "💾 备份当前路由表..."
netstat -rn > "/tmp/routes_backup_$(date +%Y%m%d_%H%M%S).txt"
echo "路由表已备份到: /tmp/routes_backup_$(date +%Y%m%d_%H%M%S).txt"

# 清理每个网关的路由
cleaned_count=0
for gw in "${GATEWAYS[@]}"; do
    echo "清理网关 $gw 的路由..."
    
    # 获取该网关的路由并删除（跳过默认路由）
    routes_to_delete=$(netstat -rn | grep "$gw" | grep -v "default" | awk '{print $1}')
    
    for route in $routes_to_delete; do
        if [[ "$route" != "Destination" && "$route" != "" ]]; then
            # 尝试删除路由，忽略错误
            route delete "$route" "$gw" 2>/dev/null && cleaned_count=$((cleaned_count + 1))
        fi
    done
done

echo ""
echo "✅ 清理完成!"
echo "📊 清理了 $cleaned_count 条路由"
echo ""

# 检查清理后的状态
echo "📈 清理后的路由统计..."
remaining_total=0
for gw in "${GATEWAYS[@]}"; do
    count=$(netstat -rn | grep "$gw" | wc -l | tr -d ' ')
    if [[ $count -gt 0 ]]; then
        echo "网关 $gw: $count 条路由"
        remaining_total=$((remaining_total + count))
    fi
done

echo "剩余路由: $remaining_total 条"
echo ""
echo "🚀 现在可以重新运行智能路由设置:"
echo "   sudo ./smartroute"
echo ""
echo "💡 或者运行守护进程模式:"
echo "   sudo ./smartroute daemon"