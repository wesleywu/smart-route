# 智能路由管理工具技术设计文档

## 📋 项目概述

本文档详细描述了基于Go技术栈的智能路由管理工具的技术实现设计，用于解决WireGuard VPN环境下中国大陆IP地址的智能分流问题。

## 🏗️ 整体架构设计

### 系统架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    Smart Route Manager                      │
├─────────────────────────────────────────────────────────────┤
│  CLI Interface (cobra)                                      │
├─────────────────────────────────────────────────────────────┤
│  Config Manager    │  Route Manager    │  Network Monitor   │
│  - IP段文件解析    │  - 路由规则操作   │  - 网络状态监控    │
│  - DNS服务器配置   │  - 批量路由设置   │  - 事件驱动更新    │
│  - 配置文件管理    │  - 路由清理重建   │  - 网关变化检测    │
├─────────────────────────────────────────────────────────────┤
│  System Interface Layer                                     │
│  - BSD Route Socket (macOS)                                │
│  - WinAPI (Windows)                                         │
│  - Netlink (Linux)                                         │
└─────────────────────────────────────────────────────────────┘
```

### 目录结构

```
update-routes-native/
├── cmd/
│   └── main.go                 # 程序入口
├── internal/
│   ├── config/                 # 配置管理
│   │   ├── config.go          # 配置结构和解析
│   │   ├── ipset.go           # IP段文件解析
│   │   └── dns.go             # DNS配置管理
│   ├── network/                # 网络操作
│   │   ├── gateway.go         # 网关检测
│   │   ├── monitor.go         # 网络监控
│   │   └── interface.go       # 网络接口管理
│   ├── routing/                # 路由管理
│   │   ├── route.go           # 路由操作接口
│   │   ├── bsd.go            # BSD系统实现 (macOS)
│   │   ├── windows.go        # Windows实现
│   │   └── linux.go          # Linux实现
│   ├── daemon/                 # 守护进程
│   │   ├── service.go        # 系统服务接口
│   │   ├── launchd.go        # macOS launchd
│   │   └── systemd.go        # Linux systemd
│   └── logger/                 # 日志管理
│       └── logger.go         # 日志配置
├── configs/
│   ├── chnroute.txt          # 中国IP段数据
│   └── chdns.txt             # 中国DNS服务器
├── scripts/
│   ├── install.sh            # 安装脚本
│   └── service/              # 系统服务配置文件
│       ├── com.smartroute.plist    # macOS
│       └── smartroute.service      # Linux
├── go.mod
├── go.sum
└── README.md
```

## 🔧 核心模块设计

### 1. Config Manager (配置管理器)

#### 职责
- 解析和管理配置文件
- 加载IP段数据文件
- 管理DNS服务器列表
- 提供配置热重载功能

#### 关键结构

```go
type Config struct {
    // 基本配置
    LogLevel     string `json:"log_level"`
    SilentMode   bool   `json:"silent_mode"`
    DaemonMode   bool   `json:"daemon_mode"`
    
    // 文件路径
    ChnRouteFile string `json:"chn_route_file"`
    ChnDNSFile   string `json:"chn_dns_file"`
    
    // 网络配置
    MonitorInterval  time.Duration `json:"monitor_interval"`
    RetryAttempts    int          `json:"retry_attempts"`
    RouteTimeout     time.Duration `json:"route_timeout"`
    
    // 性能配置
    ConcurrencyLimit int `json:"concurrency_limit"`
    BatchSize        int `json:"batch_size"`
}

type IPSet struct {
    Networks []net.IPNet
    mutex    sync.RWMutex
}

type DNSServers struct {
    IPs   []net.IP
    mutex sync.RWMutex
}
```

#### 主要方法

```go
func LoadConfig(path string) (*Config, error)
func (c *Config) Validate() error
func LoadChnRoutes(file string) (*IPSet, error)
func LoadChnDNS(file string) (*DNSServers, error)
func (ip *IPSet) Contains(addr net.IP) bool
```

### 2. Network Monitor (网络监控器)

#### 职责
- 实时监控网络接口状态变化
- 检测默认网关变化
- 触发路由规则更新事件
- 提供网络状态查询接口

#### 关键结构

```go
type NetworkMonitor struct {
    gateway      net.IP
    defaultIface string
    routeSocket  int
    eventChan    chan NetworkEvent
    stopChan     chan struct{}
    mutex        sync.RWMutex
}

type NetworkEvent struct {
    Type      EventType
    Interface string
    Gateway   net.IP
    Timestamp time.Time
}

type EventType int
const (
    GatewayChanged EventType = iota
    InterfaceUp
    InterfaceDown
    AddressChanged
)
```

#### 核心算法

```go
func (nm *NetworkMonitor) Start() error {
    // 创建PF_ROUTE socket (macOS/BSD)
    sock, err := unix.Socket(unix.AF_ROUTE, unix.SOCK_RAW, unix.AF_UNSPEC)
    if err != nil {
        return err
    }
    nm.routeSocket = sock
    
    go nm.monitorLoop()
    return nil
}

func (nm *NetworkMonitor) monitorLoop() {
    buffer := make([]byte, 4096)
    for {
        select {
        case <-nm.stopChan:
            return
        default:
            n, err := unix.Read(nm.routeSocket, buffer)
            if err != nil {
                continue
            }
            
            if event := nm.parseRouteMessage(buffer[:n]); event != nil {
                nm.eventChan <- *event
            }
        }
    }
}
```

### 3. Route Manager (路由管理器)

#### 职责
- 执行路由规则的增删改操作
- 批量处理路由规则以提高性能
- 提供跨平台路由操作抽象
- 实现路由规则的原子性操作

#### 接口定义

```go
type RouteManager interface {
    AddRoute(network *net.IPNet, gateway net.IP) error
    DeleteRoute(network *net.IPNet, gateway net.IP) error
    BatchAddRoutes(routes []Route) error
    BatchDeleteRoutes(routes []Route) error
    GetDefaultGateway() (net.IP, string, error)
    ListRoutes() ([]Route, error)
    FlushRoutes(gateway net.IP) error
}

type Route struct {
    Network *net.IPNet
    Gateway net.IP
    Interface string
    Metric  int
}
```

#### BSD实现 (macOS)

```go
type BSDRouteManager struct {
    socket int
    mutex  sync.Mutex
}

func (rm *BSDRouteManager) AddRoute(network *net.IPNet, gateway net.IP) error {
    // 构造RTM_ADD消息
    msg := &routeMessage{
        Type:    RTM_ADD,
        Flags:   RTF_UP | RTF_GATEWAY | RTF_STATIC,
        Network: network,
        Gateway: gateway,
    }
    
    return rm.sendRouteMessage(msg)
}

func (rm *BSDRouteManager) BatchAddRoutes(routes []Route) error {
    // 使用goroutine池并发处理
    semaphore := make(chan struct{}, rm.concurrencyLimit)
    var wg sync.WaitGroup
    errChan := make(chan error, len(routes))
    
    for _, route := range routes {
        wg.Add(1)
        go func(r Route) {
            defer wg.Done()
            semaphore <- struct{}{}
            defer func() { <-semaphore }()
            
            if err := rm.AddRoute(r.Network, r.Gateway); err != nil {
                errChan <- err
            }
        }(route)
    }
    
    wg.Wait()
    close(errChan)
    
    // 收集错误
    var errors []error
    for err := range errChan {
        errors = append(errors, err)
    }
    
    if len(errors) > 0 {
        return fmt.Errorf("batch operation failed: %d errors", len(errors))
    }
    
    return nil
}
```

### 4. Service Manager (服务管理器)

#### 职责
- 支持以系统服务方式运行
- 管理进程生命周期
- 处理系统信号
- 提供优雅关闭机制

#### 结构设计

```go
type ServiceManager struct {
    config    *Config
    monitor   *NetworkMonitor
    router    RouteManager
    logger    *slog.Logger
    stopChan  chan os.Signal
    doneChan  chan struct{}
}

func (sm *ServiceManager) Start() error {
    // 权限检查
    if os.Getuid() != 0 {
        return errors.New("root privileges required")
    }
    
    // 信号处理
    signal.Notify(sm.stopChan, syscall.SIGINT, syscall.SIGTERM)
    
    // 启动网络监控
    if err := sm.monitor.Start(); err != nil {
        return err
    }
    
    // 初始路由设置
    if err := sm.setupInitialRoutes(); err != nil {
        return err
    }
    
    // 主服务循环
    go sm.serviceLoop()
    
    return nil
}
```

## 🚀 性能优化策略

### 1. 并发处理优化

#### Goroutine池设计
```go
type WorkerPool struct {
    workers    int
    jobs       chan RouteJob
    results    chan RouteResult
    wg         sync.WaitGroup
}

type RouteJob struct {
    Network *net.IPNet
    Gateway net.IP
    Action  ActionType
}

func (wp *WorkerPool) Start() {
    for i := 0; i < wp.workers; i++ {
        go wp.worker()
    }
}

func (wp *WorkerPool) worker() {
    for job := range wp.jobs {
        result := RouteResult{
            Job:   job,
            Error: wp.processJob(job),
        }
        wp.results <- result
    }
}
```

#### 批量操作策略
- 将3000+条路由按批次处理（默认批次大小：100）
- 使用信号量控制并发数量（默认：50个goroutine）
- 实现退避重试机制处理临时失败

### 2. 内存优化

#### 对象池复用
```go
var routeMessagePool = sync.Pool{
    New: func() interface{} {
        return &routeMessage{
            buffer: make([]byte, 1024),
        }
    },
}

func (rm *BSDRouteManager) sendRouteMessage(msg *routeMessage) error {
    poolMsg := routeMessagePool.Get().(*routeMessage)
    defer routeMessagePool.Put(poolMsg)
    
    // 重置和复用缓冲区
    poolMsg.reset()
    poolMsg.encode(msg)
    
    return rm.write(poolMsg.buffer)
}
```

#### 内存预分配
```go
func LoadChnRoutes(file string) (*IPSet, error) {
    // 预分配切片容量
    networks := make([]net.IPNet, 0, 8000) // 预估中国IP段数量
    
    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        if network := parseCIDR(scanner.Text()); network != nil {
            networks = append(networks, *network)
        }
    }
    
    return &IPSet{Networks: networks}, nil
}
```

### 3. 系统调用优化

#### 批量系统调用
```go
func (rm *BSDRouteManager) batchSystemCall(messages []*routeMessage) error {
    // 合并多个路由消息到单个系统调用
    totalSize := 0
    for _, msg := range messages {
        totalSize += msg.size()
    }
    
    buffer := make([]byte, totalSize)
    offset := 0
    
    for _, msg := range messages {
        n := msg.writeTo(buffer[offset:])
        offset += n
    }
    
    return unix.Write(rm.socket, buffer)
}
```

## 🔒 错误处理与可靠性

### 1. 错误分类与处理

```go
type RouteError struct {
    Type    ErrorType
    Network *net.IPNet
    Gateway net.IP
    Cause   error
}

type ErrorType int
const (
    ErrPermission ErrorType = iota  // 权限错误
    ErrNetwork                      // 网络错误
    ErrInvalidRoute                 // 无效路由
    ErrSystemCall                   // 系统调用错误
    ErrTimeout                      // 超时错误
)

func (re *RouteError) IsRetryable() bool {
    return re.Type == ErrNetwork || re.Type == ErrTimeout
}
```

### 2. 重试机制

```go
func (rm *BSDRouteManager) addRouteWithRetry(network *net.IPNet, gateway net.IP) error {
    var lastErr error
    
    for attempt := 0; attempt < rm.maxRetries; attempt++ {
        if err := rm.AddRoute(network, gateway); err == nil {
            return nil
        } else if routeErr, ok := err.(*RouteError); ok && !routeErr.IsRetryable() {
            return err // 不可重试错误，直接返回
        } else {
            lastErr = err
            time.Sleep(time.Duration(attempt+1) * time.Second) // 指数退避
        }
    }
    
    return fmt.Errorf("max retries exceeded: %w", lastErr)
}
```

### 3. 事务性操作

```go
func (rm *BSDRouteManager) AtomicUpdateRoutes(oldGateway, newGateway net.IP, networks []*net.IPNet) error {
    // 创建回滚点
    rollback := make([]Route, 0, len(networks))
    
    // Phase 1: 记录现有路由
    for _, network := range networks {
        if route := rm.findRoute(network, oldGateway); route != nil {
            rollback = append(rollback, *route)
        }
    }
    
    // Phase 2: 删除旧路由
    var failed []int
    for i, network := range networks {
        if err := rm.DeleteRoute(network, oldGateway); err != nil {
            failed = append(failed, i)
        }
    }
    
    // Phase 3: 添加新路由
    for i, network := range networks {
        if err := rm.AddRoute(network, newGateway); err != nil {
            // 回滚操作
            rm.rollbackRoutes(rollback)
            return fmt.Errorf("atomic update failed at network %d: %w", i, err)
        }
    }
    
    return nil
}
```

## 📊 监控与日志

### 1. 性能指标收集

```go
type Metrics struct {
    RouteOperations    int64         // 路由操作总数
    SuccessfulOps      int64         // 成功操作数
    FailedOps          int64         // 失败操作数
    AverageOpTime      time.Duration // 平均操作时间
    NetworkChanges     int64         // 网络变化次数
    LastUpdate         time.Time     // 最后更新时间
    MemoryUsage        int64         // 内存使用量
}

func (m *Metrics) RecordOperation(duration time.Duration, success bool) {
    atomic.AddInt64(&m.RouteOperations, 1)
    if success {
        atomic.AddInt64(&m.SuccessfulOps, 1)
    } else {
        atomic.AddInt64(&m.FailedOps, 1)
    }
    
    // 更新平均时间（使用滑动平均）
    m.updateAverageTime(duration)
}
```

### 2. 结构化日志

```go
func setupLogger(config *Config) *slog.Logger {
    opts := &slog.HandlerOptions{
        Level: parseLogLevel(config.LogLevel),
    }
    
    var handler slog.Handler
    if config.SilentMode {
        handler = slog.NewTextHandler(io.Discard, opts)
    } else {
        handler = slog.NewJSONHandler(os.Stdout, opts)
    }
    
    return slog.New(handler)
}

// 使用示例
logger.Info("route operation completed",
    slog.String("network", network.String()),
    slog.String("gateway", gateway.String()),
    slog.Duration("duration", elapsed),
    slog.Int("batch_size", batchSize))
```

## 🔧 部署与配置

### 1. 配置文件示例

```json
{
    "log_level": "info",
    "silent_mode": false,
    "daemon_mode": true,
    "chn_route_file": "/etc/smartroute/chnroute.txt",
    "chn_dns_file": "/etc/smartroute/chdns.txt",
    "monitor_interval": "5s",
    "retry_attempts": 3,
    "route_timeout": "30s",
    "concurrency_limit": 50,
    "batch_size": 100
}
```

### 2. 系统服务配置

#### macOS (launchd)
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.smartroute.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/smartroute</string>
        <string>--daemon</string>
        <string>--config</string>
        <string>/etc/smartroute/config.json</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

### 3. 安装脚本

```bash
#!/bin/bash
# install.sh

# 检查权限
if [[ $EUID -ne 0 ]]; then
   echo "This script must be run as root" 
   exit 1
fi

# 创建配置目录
mkdir -p /etc/smartroute

# 复制配置文件
cp configs/* /etc/smartroute/

# 安装二进制文件
cp smartroute /usr/local/bin/
chmod +x /usr/local/bin/smartroute

# 安装系统服务
if [[ "$OSTYPE" == "darwin"* ]]; then
    cp scripts/service/com.smartroute.plist /Library/LaunchDaemons/
    launchctl load /Library/LaunchDaemons/com.smartroute.plist
elif [[ -f /etc/systemd/system ]]; then
    cp scripts/service/smartroute.service /etc/systemd/system/
    systemctl enable smartroute
    systemctl start smartroute
fi

echo "Smart Route Manager installed successfully!"
```

## 📈 性能预期

基于设计分析，预期性能指标：

- **路由设置速度**: 3000条路由规则在3-4秒内完成
- **内存占用**: 运行时占用40-60MB
- **CPU使用率**: 正常监控状态下 < 2%
- **网络变化响应**: < 2秒检测并开始处理
- **并发处理能力**: 支持50个并发路由操作
- **错误恢复时间**: < 10秒完成故障恢复

此设计确保了高性能、高可靠性和良好的可维护性，满足所有功能和非功能性需求。