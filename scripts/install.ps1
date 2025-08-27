# Smart Route Manager Installation Script for Windows
# This script downloads and installs smartroute on Windows

param(
    [switch]$Service = $false
)

# Configuration
$InstallDir = "$env:USERPROFILE\.local\bin"
$BinaryName = "smartroute.exe"
$RepoUrl = "https://github.com/wesleywu/smart-route"
$ApiUrl = "https://api.github.com/repos/wesleywu/smart-route"

# Colors
$Red = "`e[31m"
$Green = "`e[32m"
$Yellow = "`e[33m"
$Blue = "`e[34m"
$Reset = "`e[0m"

function Write-Info {
    param($Message)
    Write-Host "${Blue}[INFO]${Reset} $Message"
}

function Write-Success {
    param($Message)
    Write-Host "${Green}[SUCCESS]${Reset} $Message"
}

function Write-Warning {
    param($Message)
    Write-Host "${Yellow}[WARNING]${Reset} $Message"
}

function Write-Error {
    param($Message)
    Write-Host "${Red}[ERROR]${Reset} $Message"
}

function Test-Administrator {
    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentUser)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-Architecture {
    $arch = $env:PROCESSOR_ARCHITECTURE
    if ($arch -eq "AMD64" -or $arch -eq "x86_64") {
        return "amd64"
    }
    elseif ($arch -eq "ARM64") {
        return "arm64"
    }
    else {
        throw "Unsupported architecture: $arch"
    }
}

function Install-SmartRoute {
    Write-Host ""
    Write-Host "╔══════════════════════════════════════════════╗" -ForegroundColor Blue
    Write-Host "║        Smart Route Manager Installer         ║" -ForegroundColor Blue
    Write-Host "║               Windows Version                ║" -ForegroundColor Blue
    Write-Host "╚══════════════════════════════════════════════╝" -ForegroundColor Blue
    Write-Host ""
    
    Write-Info "Starting installation..."
    
    # Check Windows version
    Write-Info "✓ Windows detected"
    
    # Detect architecture
    $arch = Get-Architecture
    $platform = "windows"
    $binaryNamePlatform = "smartroute-${platform}-${arch}.exe"
    
    Write-Info "Detected platform: ${platform}-${arch}"
    
    # Create installation directory
    if (!(Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        Write-Info "Created installation directory: $InstallDir"
    }
    Write-Info "✓ Installation directory ready"
    
    # Download latest release
    Write-Info "Downloading latest release from GitHub..."
    
    try {
        $releaseInfo = Invoke-RestMethod -Uri "$ApiUrl/releases/latest"
        $downloadUrl = $releaseInfo.assets | Where-Object { $_.name -eq $binaryNamePlatform } | Select-Object -ExpandProperty browser_download_url
        
        if (-not $downloadUrl) {
            Write-Warning "No precompiled binary found for ${platform}-${arch}"
            Write-Error "Please download manually from: $RepoUrl/releases"
            exit 1
        }
        
        $targetPath = Join-Path $InstallDir $BinaryName
        $tempPath = "${targetPath}.tmp"
        
        Write-Info "Downloading: $downloadUrl"
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tempPath
        
        Move-Item $tempPath $targetPath -Force
        Write-Success "✓ Downloaded precompiled binary"
        
    }
    catch {
        Write-Error "Failed to download binary: $_"
        exit 1
    }
    
    # Add to PATH
    $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($userPath -notlike "*$InstallDir*") {
        Write-Info "Adding $InstallDir to PATH"
        [Environment]::SetEnvironmentVariable("PATH", "$userPath;$InstallDir", "User")
        Write-Success "✓ Added to PATH (restart terminal to take effect)"
    }
    else {
        Write-Info "✓ $InstallDir already in PATH"
    }
    
    # Verify installation
    Write-Info "Verifying installation..."
    if (Test-Path $targetPath) {
        Write-Success "✓ Binary installed at $targetPath"
        
        # Test version
        try {
            $version = & $targetPath version 2>$null | Select-Object -First 1
            Write-Info "Installed version: $version"
        }
        catch {
            Write-Warning "Could not verify version (this is normal)"
        }
    }
    else {
        Write-Error "Binary not found at expected location"
        exit 1
    }
    
    # Service installation (optional)
    if ($Service) {
        Write-Info "Installing system service..."
        if (Test-Administrator) {
            try {
                & $targetPath install
                Write-Success "✓ System service installed"
            }
            catch {
                Write-Error "Failed to install service: $_"
            }
        }
        else {
            Write-Warning "Administrator privileges required for service installation"
            Write-Info "Run 'smartroute install' as Administrator to install service"
        }
    }
    
    # Print usage
    Write-Host ""
    Write-Success "🎉 Smart Route Manager installed successfully!"
    Write-Host ""
    Write-Info "Usage:"
    Write-Host "  smartroute                    # Run once (setup routes)"
    Write-Host "  smartroute daemon             # Run as daemon"
    Write-Host "  smartroute status             # Check service status"
    Write-Host "  smartroute test               # Test configuration"
    Write-Host "  smartroute version            # Show version"
    Write-Host ""
    Write-Info "Service Management (run as Administrator):"
    Write-Host "  smartroute install            # Install service"
    Write-Host "  smartroute uninstall          # Uninstall service"
    Write-Host ""
    if ($userPath -notlike "*$InstallDir*") {
        Write-Warning "Note: Restart your terminal to use smartroute command"
    }
    
    Write-Success "Installation completed! 🚀"
}

# Main execution
try {
    Install-SmartRoute
}
catch {
    Write-Error "Installation failed: $_"
    exit 1
}