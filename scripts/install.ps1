# Windows installer for ai-agent-manager.

[CmdletBinding()]
param(
    [string]$BinDir = "$env:LOCALAPPDATA\ai-agent-manager\bin",
    [switch]$NoBuild,
    [switch]$BuildAll,
    [switch]$Hooks = $true,
    [switch]$AllAgents,
    [string]$Agent = "",
    [ValidateSet("user", "project")]
    [string]$Scope = "user"
)

$ErrorActionPreference = "Stop"

$App = "ai-agent-manager"
$Pkg = ".\cmd\ai-agent-manager"
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

function Install-GoIfMissing {
    if (Get-Command go -ErrorAction SilentlyContinue) {
        return
    }

    Write-Host "Go was not found in PATH. Attempting automatic installation."
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        winget install --id GoLang.Go --exact --accept-source-agreements --accept-package-agreements
    } elseif (Get-Command choco -ErrorAction SilentlyContinue) {
        choco install golang -y
    } elseif (Get-Command scoop -ErrorAction SilentlyContinue) {
        scoop install go
    } else {
        throw "Automatic Go installation requires winget, choco, or scoop"
    }

    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if (-not $goCommand) {
        $Candidate = Join-Path ${env:ProgramFiles} "Go\bin\go.exe"
        if (Test-Path $Candidate) {
            $env:PATH = (Split-Path $Candidate) + ";" + $env:PATH
            $goCommand = Get-Command go -ErrorAction SilentlyContinue
        }
    }
    if (-not $goCommand) {
        throw "Go installation finished but 'go' is still not available in PATH"
    }
}

function Write-TrustNotice {
    Write-Host ""
    Write-Host "Next step:"
    Write-Host "  Open Codex, Claude Code, and Gemini CLI once."
    Write-Host "  If any of them asks you to trust or approve the installed hook command, accept it."
    Write-Host "  No manual config editing should be needed."
}

Install-GoIfMissing

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

$Agents = if ($Agent) { @($Agent) } else { @("codex", "claude", "gemini") }
foreach ($Name in $Agents) {
    Write-Host "Installing $Name hook with scope=$Scope"
    & $InstallPath install --agent $Name --scope $Scope
}

Write-TrustNotice
