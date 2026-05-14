param(
    [string]$Repo = $env:AI_SESSION_VIEWER_REPO,
    [string]$Version = $env:AI_SESSION_VIEWER_VERSION,
    [string]$BinDir = $env:BIN_DIR,
    [string]$Scope = $env:AI_SESSION_VIEWER_HOOK_SCOPE,
    [string]$Agents = $env:AI_SESSION_VIEWER_AGENTS,
    [switch]$Hooks
)

$ErrorActionPreference = "Stop"

$App = "ai-session-viewer"
if ([string]::IsNullOrWhiteSpace($Repo)) {
    $Repo = "h-mousavi/codex-rtl-plugin"
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = "latest"
}
if ([string]::IsNullOrWhiteSpace($Scope)) {
    $Scope = "user"
}
if ([string]::IsNullOrWhiteSpace($Agents)) {
    $Agents = "all"
}
if ($env:AI_SESSION_VIEWER_INSTALL_HOOKS -eq "1") {
    $Hooks = $true
}

$Arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
switch ($Arch) {
    "x64" { $TargetArch = "amd64" }
    "arm64" { $TargetArch = "arm64" }
    default { throw "Unsupported architecture: $Arch" }
}

$TargetOS = "windows"
$Asset = "$App-$TargetOS-$TargetArch.zip"
if ($Version -eq "latest") {
    $Url = "https://github.com/$Repo/releases/latest/download/$Asset"
} else {
    $Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
}

if ([string]::IsNullOrWhiteSpace($BinDir)) {
    $BinDir = Join-Path $env:LOCALAPPDATA "$App\bin"
}

$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) "$App-$([System.Guid]::NewGuid())"
$Archive = Join-Path $TempDir $Asset
$ExtractDir = Join-Path $TempDir "extract"

New-Item -ItemType Directory -Path $ExtractDir -Force | Out-Null
try {
    Write-Host "Downloading $Url"
    Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $Archive
    Expand-Archive -Path $Archive -DestinationPath $ExtractDir -Force

    $Binary = Get-ChildItem -Path $ExtractDir -Recurse -Filter "$App.exe" | Select-Object -First 1
    if (-not $Binary) {
        throw "Binary not found in $Asset"
    }

    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    $InstallPath = Join-Path $BinDir "$App.exe"
    Copy-Item -Path $Binary.FullName -Destination $InstallPath -Force
    & $InstallPath --help | Out-Null

    Write-Host "Installed $App to $InstallPath"
    if (($env:PATH -split ";") -notcontains $BinDir) {
        Write-Host "PATH notice: add this directory to PATH if needed:"
        Write-Host "  $BinDir"
    }

    if ($Hooks) {
        if ($Agents -eq "all") {
            & $InstallPath install --all --scope $Scope
        } else {
            foreach ($Agent in ($Agents -split "[,\s]+" | Where-Object { $_ })) {
                & $InstallPath install --agent $Agent --scope $Scope
            }
        }
    }

    Write-Host "Run the server with:"
    Write-Host "  $App server"
}
finally {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
