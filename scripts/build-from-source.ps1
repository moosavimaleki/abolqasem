# Windows build-from-source installer for abolqasem.

[CmdletBinding()]
param(
    [string]$BinDir = "$env:LOCALAPPDATA\abolqasem\bin",
    [switch]$NoBuild,
    [switch]$BuildAll
)

$ErrorActionPreference = "Stop"

$App = "abolqasem"
$Pkg = ".\cmd\abolqasem"
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

Install-GoIfMissing

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
if (Test-Path $InstallPath) {
    & $InstallPath service stop *> $null
}
Copy-Item -Force $TargetBinary $InstallPath

Write-Host "Installed $App to $InstallPath"

$PathParts = ($env:PATH -split ";") | Where-Object { $_ -ne "" }
if ($PathParts -notcontains $BinDir) {
    Write-Host "PATH notice: add this directory to your user PATH if needed:"
    Write-Host "  $BinDir"
}

Write-Host "Installing persistent service and all agent hooks"
& $InstallPath install
if ($LASTEXITCODE -ne 0) {
    throw "$App install failed with exit code $LASTEXITCODE"
}
