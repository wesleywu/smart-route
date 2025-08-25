# 🚀 智能路由管理工具运行指南

## ✅ 问题已修复！

网关检测问题已经解决，程序现在可以正确识别VPN环境下的物理网关。

## 🎯 当前状态

- **程序版本**: Smart Route Manager v1.0.0
- **检测到的物理网关**: 192.168.32.1 (en0接口)
- **中国路由数量**: 8,690条网络规则
- **中国DNS服务器**: 4个
- **编译状态**: ✅ 成功
- **配置测试**: ✅ 通过

## 🚀 立即开始使用

### 1. 一次性路由设置（推荐先试用）

```bash
sudo ./smartroute
```

这会：
- 为所有8690个中国IP段设置直连路由
- 为4个中国DNS服务器设置直连路由
- 使用检测到的网关 192.168.32.1
- 完成后显示操作结果

### 2. 守护进程模式（自动监控网络变化）

```bash
sudo ./smartroute daemon
```

预期输出示例：
```json
{"time":"2025-08-25T10:35:00.123456+08:00","level":"INFO","msg":"service starting","version":"1.0.0","pid":"12345"}
{"time":"2025-08-25T10:35:00.234567+08:00","level":"INFO","msg":"configuration loaded","config_file":"configs/config.json","chn_routes":8690,"chn_dns":4}
{"time":"2025-08-25T10:35:01.345678+08:00","level":"INFO","msg":"setting up routes","gateway":"192.168.32.1","total":8694}
{"time":"2025-08-25T10:35:05.456789+08:00","level":"INFO","msg":"batch operation completed","action":"add","total":8694,"success":8694,"failed":0,"duration_ms":4111}
{"time":"2025-08-25T10:35:05.567890+08:00","level":"INFO","msg":"network monitor started","poll_interval":"5s"}
```

### 3. 系统服务安装（生产环境推荐）

```bash
# 安装为系统服务
sudo ./smartroute install

# 查看服务状态
./smartroute status

# 如需卸载
sudo ./smartroute uninstall
```

## 📊 验证分流效果

### 检查路由是否生效

```bash
# 查看中国IP路由（应该走en0）
route -n get 223.5.5.5
# 应该显示: gateway: 192.168.32.1, interface: en0

# 查看国外IP路由（应该走VPN）
route -n get 8.8.8.8  
# 应该显示: interface: utun6
```

### 测试网络连接

```bash
# 测试中国网站（应该直连）
curl -s -w "%{time_total}s - %{remote_ip}\n" -o /dev/null https://www.baidu.com

# 测试国外网站（应该走VPN）
curl -s -w "%{time_total}s - %{remote_ip}\n" -o /dev/null https://www.google.com
```

## 🔧 配置文件自定义

编辑 `configs/config.json` 来调整设置：

```json
{
    "log_level": "info",          // debug, info, warn, error
    "silent_mode": false,         // 是否静默运行
    "daemon_mode": false,         // 守护进程模式
    "monitor_interval": "5s",     // 网络监控间隔
    "retry_attempts": 3,          // 重试次数
    "concurrency_limit": 50,      // 并发限制
    "batch_size": 100            // 批处理大小
}
```

## 🎮 高级用法

### 自定义配置运行

```bash
# 使用自定义配置
sudo ./smartroute --config /path/to/config.json

# 静默模式（无输出）
sudo ./smartroute --silent

# 守护进程 + 自定义配置
sudo ./smartroute daemon --config configs/config.json
```

### 使用Makefile（推荐）

```bash
# 一次性设置
make run

# 守护进程模式
make daemon

# 测试配置
make test-config

# 安装服务
make install

# 查看状态
make status
```

## 🚨 重要提示

1. **VPN兼容性**: 程序已优化支持WireGuard等VPN环境
2. **网络中断**: 守护进程模式会自动处理网络变化
3. **权限要求**: 所有路由操作都需要root权限
4. **备份建议**: 首次使用前建议备份路由表

## 📈 性能预期

基于你的网络环境：
- **设置8694条路由规则预计耗时**: 3-5秒
- **内存占用**: 约40-60MB
- **CPU使用**: 设置期间2-5%，监控时<1%

## 🔍 故障排查

如果遇到问题：

```bash
# 1. 检查网络状态
./smartroute version

# 2. 验证配置
./smartroute test

# 3. 查看详细日志
sudo ./smartroute daemon  # 观察输出

# 4. 检查路由表
netstat -rn | head -20
```

## 🎉 准备就绪！

你的智能路由管理工具已经完全配置好，可以开始使用了！

**推荐首次使用步骤**：
1. `sudo ./smartroute` - 一次性设置，验证效果
2. `sudo ./smartroute daemon` - 启动守护进程
3. `sudo ./smartroute install` - 如果效果满意，安装为系统服务

现在就可以享受智能分流带来的网络加速效果了！🚀