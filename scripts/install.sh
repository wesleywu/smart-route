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
INSTALL_DIR="$HOME/.local/bin"
BINARY_NAME="smartroute"
REPO_URL="https://github.com/wesleywu/smart-route"
SERVICE_NAME="com.smartroute.daemon"

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

# Check if running as root (should not be)
check_not_root() {
    if [[ $EUID -eq 0 ]]; then
        print_error "Do not run this script as root. Run as regular user, sudo will be requested when needed."
        exit 1
    fi
    print_info "✓ Running as regular user"
}

# Create installation directory
create_install_dir() {
    if [[ ! -d "$INSTALL_DIR" ]]; then
        print_info "Creating installation directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR"
    fi
    print_info "✓ Installation directory ready"
}

# Build or download binary
build_binary() {
    local current_dir=$(pwd)
    
    # Check if we're in the project directory (has go.mod)
    if [[ -f "go.mod" ]]; then
        print_info "Building from source..."
        go build -o "$INSTALL_DIR/$BINARY_NAME" ./cmd
        print_success "✓ Built from source"
    else
        print_error "go.mod not found. Please run this script from the project root directory."
        print_info "Alternative: Download the binary from $REPO_URL/releases"
        exit 1
    fi
}

# Check and add to PATH
setup_path() {
    local shell_name=$(basename "$SHELL")
    local rc_file=""
    local path_line="export PATH=\"\$HOME/.local/bin:\$PATH\""
    
    # Determine the correct RC file
    case "$shell_name" in
        "zsh")
            rc_file="$HOME/.zshrc"
            ;;
        "bash")
            rc_file="$HOME/.bashrc"
            # On macOS, also check .bash_profile
            if [[ ! -f "$rc_file" && -f "$HOME/.bash_profile" ]]; then
                rc_file="$HOME/.bash_profile"
            fi
            ;;
        *)
            print_warning "Unknown shell: $shell_name. You may need to manually add $INSTALL_DIR to your PATH"
            return
            ;;
    esac
    
    # Check if PATH is already set
    if echo "$PATH" | grep -q "$INSTALL_DIR"; then
        print_info "✓ $INSTALL_DIR already in PATH"
        return
    fi
    
    # Add to RC file if not present
    if [[ -f "$rc_file" ]]; then
        if ! grep -q "$HOME/.local/bin" "$rc_file"; then
            print_info "Adding $INSTALL_DIR to PATH in $rc_file"
            echo "" >> "$rc_file"
            echo "# Added by Smart Route Manager installer" >> "$rc_file"
            echo "$path_line" >> "$rc_file"
            print_success "✓ Added to $rc_file"
        else
            print_info "✓ $INSTALL_DIR already configured in $rc_file"
        fi
    else
        print_info "Creating $rc_file and adding PATH"
        echo "$path_line" > "$rc_file"
        print_success "✓ Created $rc_file with PATH configuration"
    fi
    
    # Export for current session
    export PATH="$HOME/.local/bin:$PATH"
    print_info "✓ PATH updated for current session"
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
        print_warning "Note: Restart your terminal or run 'source ~/.$(basename "$SHELL")rc' to use $BINARY_NAME command"
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
    check_not_root
    create_install_dir
    build_binary
    setup_path
    
    echo
    read -p "Install system service (auto-start on boot)? [Y/n]: " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Nn]$ ]]; then
        print_info "Skipping service installation"
    else
        install_service
    fi
    
    verify_installation
    print_usage
    
    print_success "Installation completed! 🚀"
}

# Trap errors
trap 'print_error "Installation failed at line $LINENO"' ERR

# Run main function
main "$@"