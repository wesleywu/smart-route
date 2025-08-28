#!/bin/bash
#
# Smart Route Manager Installation Script for macOS
# This script installs smartroute to $HOME/.local/bin and sets up the system service
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
BINARY_NAME="smartroute"
REPO_URL="https://github.com/wesleywu/smart-route"
SERVICE_NAME="com.smartroute.daemon"
API_URL="https://api.github.com/repos/wesleywu/smart-route"

# Print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running on macOS
check_macos() {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        print_error "This script is only for macOS. Detected: $(uname -s)"
        exit 1
    fi
    print_info "✓ macOS detected"
}

# Check if running with appropriate permissions
check_permissions() {
    if [[ $EUID -eq 0 ]]; then
        # Running as root (via sudo)
        if [[ -z "$SUDO_USER" ]]; then
            print_error "Please run with sudo, not as root user directly"
            exit 1
        fi
        INSTALL_DIR="/usr/local/bin"
        print_info "✓ Running with sudo (installing to system-wide location)"
    else
        print_error "This installation requires sudo access to install the system service"
        print_error "Please run: curl -sSL <script-url> | sudo bash"
        exit 1
    fi
}

# Create installation directory
create_install_dir() {
    if [[ ! -d "$INSTALL_DIR" ]]; then
        print_info "Creating installation directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR"
    fi
    print_info "✓ Installation directory ready"
}

# Detect platform and architecture
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)
    
    # Normalize architecture names
    case "$arch" in
        x86_64|amd64)
            arch="amd64"
            ;;
        arm64|aarch64)
            arch="arm64"
            ;;
        *)
            print_error "Unsupported architecture: $arch"
            exit 1
            ;;
    esac
    
    # Set platform-specific binary name
    case "$os" in
        darwin)
            PLATFORM="darwin"
            BINARY_SUFFIX=""
            ;;
        linux)
            PLATFORM="linux"  
            BINARY_SUFFIX=""
            ;;
        *)
            print_error "Unsupported operating system: $os"
            exit 1
            ;;
    esac
    
    ARCH="$arch"
    BINARY_NAME_PLATFORM="smartroute-${PLATFORM}-${ARCH}${BINARY_SUFFIX}"
    
    print_info "Detected platform: ${PLATFORM}-${ARCH}"
}

# Download precompiled binary
download_binary() {
    print_info "Downloading latest release from GitHub..."
    
    # Get latest release info
    local release_info
    release_info=$(curl -sSL "$API_URL/releases/latest")
    if [ $? -ne 0 ]; then
        print_error "Failed to fetch release information"
        exit 1
    fi
    
    # Extract download URL
    local download_url
    download_url=$(echo "$release_info" | grep -o "\"browser_download_url\":\s*\"[^\"]*${BINARY_NAME_PLATFORM}\"" | cut -d'"' -f4)
    
    if [[ -z "$download_url" ]]; then
        print_error "No precompiled binary found for ${PLATFORM}-${ARCH}"
        exit 1
    fi
    
    # Download binary
    print_info "Downloading: $download_url"
    local temp_binary="$INSTALL_DIR/${BINARY_NAME}.tmp"
    
    if ! curl -sSL -o "$temp_binary" "$download_url"; then
        print_error "Failed to download binary"
        exit 1
    fi
    
    # Move to final location
    mv "$temp_binary" "$INSTALL_DIR/$BINARY_NAME"
    chmod +x "$INSTALL_DIR/$BINARY_NAME"
    
    print_success "✓ Downloaded precompiled binary"
}


# Check PATH
setup_path() {
    # /usr/local/bin is typically in PATH by default on macOS
    if echo "$PATH" | grep -q "/usr/local/bin"; then
        print_info "✓ /usr/local/bin is in PATH"
    else
        print_warning "/usr/local/bin is not in PATH. You may need to add it manually"
    fi
}

# Install system service
install_service() {
    print_info "Installing system service (requires sudo)..."
    
    # Check if service is already installed
    if sudo "$INSTALL_DIR/$BINARY_NAME" status >/dev/null 2>&1; then
        print_warning "Service appears to be already installed"
        read -p "Reinstall service? [y/N]: " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_info "Skipping service installation"
            return
        fi
        
        # Uninstall existing service
        print_info "Uninstalling existing service..."
        sudo "$INSTALL_DIR/$BINARY_NAME" uninstall || true
    fi
    
    # Install new service
    sudo "$INSTALL_DIR/$BINARY_NAME" install
    print_success "✓ System service installed"
    
    # Start service
    print_info "Starting service..."
    sudo launchctl start "$SERVICE_NAME" || {
        print_warning "Failed to start service immediately, but it will start on next boot"
    }
    
    # Check service status
    sleep 2
    local status
    status=$(sudo "$INSTALL_DIR/$BINARY_NAME" status 2>/dev/null || echo "unknown")
    print_info "Service status: $status"
}

# Verify installation
verify_installation() {
    print_info "Verifying installation..."
    
    # Check binary exists and is executable
    if [[ -x "$INSTALL_DIR/$BINARY_NAME" ]]; then
        print_success "✓ Binary installed at $INSTALL_DIR/$BINARY_NAME"
    else
        print_error "Binary not found or not executable"
        exit 1
    fi
    
    # Check version
    local version
    version=$("$INSTALL_DIR/$BINARY_NAME" version 2>/dev/null | head -1 || echo "unknown")
    print_info "Installed version: $version"
    
    # Check if binary is in PATH
    if command -v "$BINARY_NAME" >/dev/null 2>&1; then
        print_success "✓ $BINARY_NAME is available in PATH"
    else
        print_warning "$BINARY_NAME not found in PATH. Restart your terminal or run:"
        print_warning "  source ~/.$(basename "$SHELL")rc"
    fi
}

# Print usage instructions
print_usage() {
    echo
    print_success "🎉 Smart Route Manager installed successfully!"
    echo
    print_info "Usage:"
    echo "  $BINARY_NAME                    # Run once (setup routes)"
    echo "  $BINARY_NAME daemon             # Run as daemon"  
    echo "  $BINARY_NAME status             # Check service status"
    echo "  $BINARY_NAME test               # Test configuration"
    echo "  $BINARY_NAME version            # Show version"
    echo
    print_info "Service Management:"
    echo "  sudo launchctl start $SERVICE_NAME    # Start service"
    echo "  sudo launchctl stop $SERVICE_NAME     # Stop service"
    echo "  sudo $BINARY_NAME uninstall           # Uninstall service"
    echo
    print_info "Logs:"
    echo "  tail -f /var/log/smartroute.out.log   # View service output"
    echo "  tail -f /var/log/smartroute.err.log   # View service errors"
    echo
    if ! command -v "$BINARY_NAME" >/dev/null 2>&1; then
        print_warning "Note: $BINARY_NAME command not found in PATH. You may need to restart your terminal."
    fi
}

# Main installation function
main() {
    echo -e "${BLUE}"
    echo "╔══════════════════════════════════════════════╗"
    echo "║        Smart Route Manager Installer         ║"
    echo "║               macOS Version                  ║"
    echo "╚══════════════════════════════════════════════╝"
    echo -e "${NC}"
    
    print_info "Starting installation..."
    
    check_macos
    check_permissions
    detect_platform
    create_install_dir
    download_binary
    setup_path
    
    install_service
    
    verify_installation
    print_usage
    
    print_success "Installation completed! 🚀"
}

# Trap errors
trap 'print_error "Installation failed at line $LINENO"' ERR

# Run main function
main "$@"