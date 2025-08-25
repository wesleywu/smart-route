#!/bin/bash

echo "🧪 智能网关管理测试"
echo "===================="
echo ""

# 检查权限
if [[ $EUID -ne 0 ]]; then
   echo "❌ 需要root权限运行此脚本"
   echo "请使用: sudo $0"
   exit 1
fi

echo "📊 当前网络状态:"
./smartroute version
echo ""

echo "🔍 检查网关状态文件:"
STATE_FILE="/tmp/smartroute_gateway_state.json"
if [[ -f "$STATE_FILE" ]]; then
    echo "网关状态文件存在:"
    cat "$STATE_FILE" | jq . 2>/dev/null || cat "$STATE_FILE"
    echo ""
    
    # 提取之前的网关信息
    PREV_GW=$(cat "$STATE_FILE" | grep '"previous_gateway"' | cut -d'"' -f4 2>/dev/null)
    if [[ -n "$PREV_GW" ]]; then
        echo "之前记录的网关: $PREV_GW"
        
        # 检查是否有指向该网关的路由
        ROUTE_COUNT=$(netstat -rn | grep "$PREV_GW" | grep -v "default" | wc -l | tr -d ' ')
        echo "指向该网关的路由数量: $ROUTE_COUNT"
        echo ""
    fi
else
    echo "网关状态文件不存在 (首次运行)"
    echo ""
fi

echo "🚀 模拟网关管理场景..."
echo ""

echo "📝 场景1: 首次运行"
echo "运行命令: ./smartroute (模拟，显示预期输出)"
echo ""
echo "预期日志输出:"
echo '{"level":"INFO","msg":"Checking gateway state..."}'
echo '{"level":"INFO","msg":"First time setup","gateway":"192.168.32.1","interface":"en0"}'
echo '{"level":"INFO","msg":"Setting up routes","gateway":"192.168.32.1","total":8694}'
echo '{"level":"INFO","msg":"Gateway state saved","gateway":"192.168.32.1","routes":8694}'
echo ""

read -p "按回车键继续查看场景2..."
echo ""

echo "📝 场景2: 网关未变化的情况"
echo "如果你再次运行相同WiFi环境下的 ./smartroute："
echo ""
echo "预期日志输出:"
echo '{"level":"INFO","msg":"Checking gateway state..."}'
echo '{"level":"INFO","msg":"Gateway unchanged","gateway":"192.168.32.1","interface":"en0"}'
echo '{"level":"INFO","msg":"Cleaning up existing routes for consistency..."}'
echo '{"level":"INFO","msg":"Successfully cleaned routes","gateway":"192.168.32.1","count":8694}'
echo '{"level":"INFO","msg":"Setting up routes","gateway":"192.168.32.1","total":8694}'
echo ""

read -p "按回车键继续查看场景3..."
echo ""

echo "📝 场景3: 网关变化的情况"
echo "假设你切换WiFi从 192.168.32.1 到 192.168.1.1："
echo ""
echo "预期日志输出:"
echo '{"level":"INFO","msg":"Checking gateway state..."}'
echo '{"level":"INFO","msg":"Gateway change detected","previous_gateway":"192.168.32.1","current_gateway":"192.168.1.1"}'
echo '{"level":"INFO","msg":"Cleaning up routes for previous gateway","gateway":"192.168.32.1"}'
echo '{"level":"INFO","msg":"Successfully cleaned routes","gateway":"192.168.32.1","count":8694}'
echo '{"level":"INFO","msg":"Setting up routes","gateway":"192.168.1.1","total":8694}'
echo '{"level":"INFO","msg":"Gateway state saved","gateway":"192.168.1.1","routes":8694}'
echo ""

echo "✨ 智能网关管理的优势:"
echo "1. 🧠 记住之前的网关设置"
echo "2. 🔄 自动检测网关变化"
echo "3. 🧹 精确清理旧网关的路由"
echo "4. ⚡ 避免重复和冲突路由"
echo "5. 📊 保存状态供下次使用"
echo ""

echo "🎯 使用建议:"
echo "现在你可以放心地:"
echo "- 切换WiFi热点后直接运行 sudo ./smartroute"
echo "- 使用守护进程模式 sudo ./smartroute daemon 实现全自动管理"
echo "- 不再需要手动清理路由"
echo ""

if [[ -f "$STATE_FILE" ]]; then
    echo "🗂️  当前状态文件内容:"
    cat "$STATE_FILE" | jq . 2>/dev/null || cat "$STATE_FILE"
fi