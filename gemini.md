# Smart Route Manager - AI Assistant Guide

## 项目概述

Smart Route Manager 是一个为 macOS 设计的智能路由管理工具，主要解决使用三层 VPN（如 WireGuard、OpenVPN）时访问中国网站速度慢的问题。

### 核心功能
- 智能分流：中国 IP 地址直连，其他流量走 VPN
- 自动适配：WiFi 切换时自动调整路由
- 系统服务：作为 launchd 服务运行，开机自启
- 实时监控：检测网络状态变化并自动响应

## 技术栈

- **语言**: Go 1.23+
- **平台**: macOS (主要), Linux, Windows (部分支持)
- **主要依赖**:
  - `cobra`: CLI 框架
  - `ants`: 并发池
  - `xxhash`: 快速哈希
  - `golang.org/x/sys`: 系统调用

## 项目结构

```
.
├── cmd/
│   └── main.go                 # CLI 入口，定义命令
├── internal/
│   ├── config/                 # 配置管理
│   │   ├── config.go           # 主配置逻辑
│   │   ├── embed.go            # 嵌入的中国IP数据
│   │   ├── ipset.go            # IP集合管理
│   │   ├── dns.go              # DNS配置
│   │   └── gateway_state.go    # 网关状态管理
│   ├── daemon/                 # 系统服务
│   │   ├── service.go          # 服务主逻辑
│   │   ├── launchd.go          # macOS launchd集成
│   │   ├── systemd.go          # Linux systemd集成
│   │   └── service_*.go        # 平台特定实现
│   ├── routing/                # 路由管理核心
│   │   ├── route.go            # 路由操作主逻辑
│   │   ├── switch.go           # 路由切换逻辑
│   │   ├── monitor.go          # 网络监控
│   │   ├── batch/              # 批量操作
│   │   ├── platform/           # 平台特定路由实现
│   │   │   ├── bsd.go          # BSD/macOS路由
│   │   │   ├── linux.go        # Linux路由
│   │   │   └── windows.go      # Windows路由
│   │   └── types/              # 路由类型定义
│   ├── logger/                 # 日志系统
│   └── utils/                  # 工具函数
│       ├── gateway.go          # 网关检测
│       ├── iface.go            # 网络接口管理
│       └── ip.go               # IP地址处理
└── scripts/                    # 安装脚本
    ├── install.sh              # Unix安装脚本
    └── install.ps1             # Windows安装脚本
```

## 核心概念

### 1. 路由管理
- **RouteManager** (`internal/routing/types/route_manager.go`): 路由管理器接口
- **BSDRouteManager** (`internal/routing/platform/bsd.go`): macOS实现
- 使用系统命令 `route` 操作路由表

### 2. 批量操作
- **BatchOperation** (`internal/routing/batch/batch_operation.go`): 并发批量路由操作
- 使用 goroutine 池优化性能，8000+条路由规则在2秒内完成

### 3. 网络监控
- **NetworkMonitor** (`internal/routing/monitor.go`): 监控网关变化
- 定期轮询检测 VPN 状态和网关变化
- WiFi切换时自动触发路由更新

### 4. 服务管理
- **Service** (`internal/daemon/service.go`): 守护进程主逻辑
- **LaunchdService** (`internal/daemon/launchd.go`): macOS服务集成
- 配置文件位置: `/Library/LaunchDaemons/com.smartroute.plist`

## 命令和操作

### CLI 命令
```bash
# 查看版本
smartroute version

# 测试配置
smartroute test

# 运行一次（设置路由后退出）
smartroute

# 守护进程模式
smartroute daemon

# 安装系统服务
sudo smartroute install

# 卸载
sudo smartroute uninstall

# 查看服务状态
sudo smartroute status
```

### 构建和测试
```bash
# 构建
go build -o bin/smartroute ./cmd

# 运行测试
go test ./...

# 跨平台构建（仅用于测试是否可以编译通过，正式release的build通过github action完成）
GOOS=darwin GOARCH=amd64 go build -o /dev/null ./cmd
GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd
```

## 关键文件说明

### 配置数据
- `internal/config/embed.go`: 嵌入的中国IP段数据（8690个网段）
- 数据来源: APNIC, 定期更新

### 日志
- 标准输出: `/var/log/smartroute.out.log`
- 错误输出: `/var/log/smartroute.err.log`
- 格式: JSON结构化日志

### 系统集成
- macOS: 使用 launchd，配置文件在 `/Library/LaunchDaemons/`
- Linux: 使用 systemd，服务文件在 `/etc/systemd/system/`
- Windows: 使用 Windows Service (部分实现)

## 开发指南

### 添加新功能
1. 路由相关: 修改 `internal/routing/`
2. 配置相关: 修改 `internal/config/`
3. CLI命令: 修改 `cmd/main.go`

### 调试技巧
1. 使用 `bin/smartroute test` 检查配置
2. 查看日志: `tail -f /var/log/smartroute.out.log`
3. 手动运行: `sudo bin/smartroute daemon` (前台运行便于调试)

### 注意事项
- 路由操作需要 root 权限
- macOS 上使用 `route` 命令，Linux 使用 `ip route`
- VPN 接口通常以 `utun` (macOS) 或 `tun` (Linux) 开头
- 批量操作使用并发池，注意资源限制

## 常见问题处理

### 服务无法启动
1. 检查权限: 需要 root 权限
2. 检查日志: `/var/log/smartroute.err.log`
3. 手动测试: `sudo bin/smartroute test`

### 路由未生效
1. 确认 VPN 已连接
2. 检查网关: `bin/smartroute test`
3. 查看路由表: `netstat -rn`

### WiFi 切换问题
1. 查看监控日志确认是否检测到变化
2. 检查新网关是否正确识别
3. 手动触发: 重启服务

## 性能优化

- 使用并发池处理批量路由（8个worker）
- IP集合使用哈希表快速查找
- 缓存网关状态避免重复检测
- 监控轮询间隔2秒，平衡响应速度和资源消耗