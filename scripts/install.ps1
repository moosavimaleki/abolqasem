# Windows install script for ai-session-viewer

$ErrorActionPreference = "Stop"

Write-Host "Installing ai-session-viewer..."

# This script would normally download the latest zip from GitHub Release, extract it,
# and put the binary in $env:LOCALAPPDATA\ai-session-viewer\bin

$InstallDir = "$env:LOCALAPPDATA\ai-session-viewer\bin"
if (!(Test-Path -Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}

Write-Host "Downloading and extracting is stubbed for MVP..."
Write-Host "Please build manually and place ai-session-viewer.exe in $InstallDir"

# Add to PATH logic would go here
Write-Host ""
Write-Host "Please add $InstallDir to your PATH environment variable."
Write-Host "Then run 'ai-session-viewer server' to start the viewer."
