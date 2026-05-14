# Windows installer for ai-session-viewer.

[CmdletBinding()]
param(
    [string]$BinDir = "$env:LOCALAPPDATA\ai-session-viewer\bin",
    [switch]$NoBuild,
    [switch]$BuildAll,
    [switch]$Hooks,
    [switch]$AllAgents,
    [string]$Agent = "",
    [ValidateSet("user", "project")]
    [string]$Scope = "user"
)

$ErrorActionPreference = "Stop"

$App = "ai-session-viewer"
$Pkg = ".\cmd\ai-session-viewer"
$Dist = "dist"

function Get-TargetArch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

function Build-Target {
    param(
        [string]$Goos,
        [string]$Goarch
    )

    New-Item -ItemType Directory -Force -Path $Dist | Out-Null
    $Suffix = if ($Goos -eq "windows") { ".exe" } else { "" }
    $Out = Join-Path $Dist "$App-$Goos-$Goarch$Suffix"
    Write-Host "Building $Out"

    $env:CGO_ENABLED = "0"
    $env:GOOS = $Goos
    $env:GOARCH = $Goarch
    go build -trimpath -ldflags="-s -w" -o $Out $Pkg
}

function Build-AllTargets {
    if (Test-Path $Dist) {
        Remove-Item -Recurse -Force $Dist
    }
    Build-Target "linux" "amd64"
    Build-Target "linux" "arm64"
    Build-Target "darwin" "amd64"
    Build-Target "darwin" "arm64"
    Build-Target "windows" "amd64"
    Build-Target "windows" "arm64"
}

if (!(Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go 1.22+ is required and was not found in PATH"
}

if ($Agent -and @("codex", "claude", "gemini") -notcontains $Agent) {
    throw "Unsupported agent: $Agent"
}

$Arch = Get-TargetArch
$TargetBinary = Join-Path $Dist "$App-windows-$Arch.exe"

if ($BuildAll) {
    Build-AllTargets
} elseif (!$NoBuild) {
    Build-Target "windows" $Arch
}

if (!(Test-Path $TargetBinary)) {
    throw "Expected binary not found: $TargetBinary"
}

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
$InstallPath = Join-Path $BinDir "$App.exe"
Copy-Item -Force $TargetBinary $InstallPath

Write-Host "Installed $App to $InstallPath"

$PathParts = ($env:PATH -split ";") | Where-Object { $_ -ne "" }
if ($PathParts -notcontains $BinDir) {
    Write-Host "PATH notice: add this directory to your user PATH if needed:"
    Write-Host "  $BinDir"
}

if ($Hooks -or $AllAgents -or $Agent) {
    $Agents = if ($Agent) { @($Agent) } else { @("codex", "claude", "gemini") }
    foreach ($Name in $Agents) {
        Write-Host "Installing $Name hook with scope=$Scope"
        & $InstallPath install --agent $Name --scope $Scope
    }
} else {
    Write-Host "Hook install skipped. To install hooks later:"
    Write-Host "  ai-session-viewer install --all --scope user"
}

Write-Host "Run the server with:"
Write-Host "  ai-session-viewer server"
