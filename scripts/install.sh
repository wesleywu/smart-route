#!/bin/bash

set -e

# 检查权限
if [[ $EUID -ne 0 ]]; then
   echo "❌ This script must be run as root"
   echo "Usage: sudo $0"
   exit 1
fi

# 配置变量
BINARY_NAME="smartroute"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/smartroute"
SERVICE_DIR="/Library/LaunchDaemons"  # macOS
SYSTEMD_DIR="/etc/systemd/system"     # Linux

echo "🚀 Smart Route Manager Installation"
echo "======================================"

# 检测操作系统
OS="unknown"
if [[ "$OSTYPE" == "darwin"* ]]; then
    OS="darwin"
    echo "📱 Detected: macOS"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    OS="linux"
    echo "🐧 Detected: Linux"
else
    echo "❌ Unsupported operating system: $OSTYPE"
    exit 1
fi

# 检查二进制文件
if [[ ! -f "$BINARY_NAME" ]]; then
    echo "❌ Binary file '$BINARY_NAME' not found"
    echo "Please build the project first: go build -o $BINARY_NAME cmd/main.go"
    exit 1
fi

# 创建配置目录
echo "📁 Creating configuration directory..."
mkdir -p "$CONFIG_DIR"

# 复制配置文件
echo "📋 Installing configuration files..."
cp configs/* "$CONFIG_DIR/"
chmod 644 "$CONFIG_DIR"/*

# 安装二进制文件
echo "💾 Installing binary..."
cp "$BINARY_NAME" "$INSTALL_DIR/"
chmod 755 "$INSTALL_DIR/$BINARY_NAME"

# 创建日志目录
mkdir -p /var/log
touch /var/log/smartroute.out.log
touch /var/log/smartroute.err.log
chmod 644 /var/log/smartroute.*.log

# 安装系统服务
echo "⚙️  Installing system service..."
if [[ "$OS" == "darwin" ]]; then
    # macOS launchd
    cat > "$SERVICE_DIR/com.smartroute.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.smartroute.daemon</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/$BINARY_NAME</string>
        <string>daemon</string>
        <string>--config</string>
        <string>$CONFIG_DIR/config.json</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/smartroute.out.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/smartroute.err.log</string>
    <key>WorkingDirectory</key>
    <string>$INSTALL_DIR</string>
</dict>
</plist>
EOF

    chmod 644 "$SERVICE_DIR/com.smartroute.plist"
    
    echo "🔄 Loading launchd service..."
    launchctl load "$SERVICE_DIR/com.smartroute.plist"
    
elif [[ "$OS" == "linux" ]]; then
    # Linux systemd
    cat > "$SYSTEMD_DIR/smartroute.service" << EOF
[Unit]
Description=Smart Route Manager
After=network.target
Wants=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/$BINARY_NAME daemon --config $CONFIG_DIR/config.json
Restart=always
RestartSec=5
User=root
Group=root
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

    chmod 644 "$SYSTEMD_DIR/smartroute.service"
    
    echo "🔄 Enabling systemd service..."
    systemctl daemon-reload
    systemctl enable smartroute
    systemctl start smartroute
fi

echo ""
echo "✅ Installation completed successfully!"
echo ""
echo "📋 Installation Summary:"
echo "  • Binary:       $INSTALL_DIR/$BINARY_NAME"
echo "  • Config:       $CONFIG_DIR/"
echo "  • Logs:         /var/log/smartroute.*.log"

if [[ "$OS" == "darwin" ]]; then
    echo "  • Service:      $SERVICE_DIR/com.smartroute.plist"
    echo ""
    echo "🔧 Service Management (macOS):"
    echo "  • Status:       launchctl list | grep smartroute"
    echo "  • Stop:         sudo launchctl stop com.smartroute.daemon"
    echo "  • Start:        sudo launchctl start com.smartroute.daemon"
    echo "  • Uninstall:    sudo $BINARY_NAME uninstall"
elif [[ "$OS" == "linux" ]]; then
    echo "  • Service:      $SYSTEMD_DIR/smartroute.service"
    echo ""
    echo "🔧 Service Management (Linux):"
    echo "  • Status:       sudo systemctl status smartroute"
    echo "  • Stop:         sudo systemctl stop smartroute"
    echo "  • Start:        sudo systemctl start smartroute"
    echo "  • Logs:         sudo journalctl -u smartroute -f"
    echo "  • Uninstall:    sudo $BINARY_NAME uninstall"
fi

echo ""
echo "⚡ Quick Test:"
echo "  • Test config:  sudo $BINARY_NAME test"
echo "  • One-time run: sudo $BINARY_NAME"
echo "  • Show version: $BINARY_NAME version"
echo ""
echo "🎉 Smart Route Manager is now installed and running!"