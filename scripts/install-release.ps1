param(
    [string]$Repo = $(if ($env:ABOLQASEM_REPO) { $env:ABOLQASEM_REPO } else { $env:AI_AGENT_MANAGER_REPO }),
    [string]$Version = $(if ($env:ABOLQASEM_VERSION) { $env:ABOLQASEM_VERSION } else { $env:AI_AGENT_MANAGER_VERSION }),
    [string]$ReleaseBaseUrl = $(if ($env:ABOLQASEM_RELEASE_BASE_URL) { $env:ABOLQASEM_RELEASE_BASE_URL } else { $env:AI_AGENT_MANAGER_RELEASE_BASE_URL }),
    [string]$BinDir = $env:BIN_DIR
)

$ErrorActionPreference = "Stop"

$App = "abolqasem"
if ([string]::IsNullOrWhiteSpace($Repo)) {
    $Repo = "moosavimaleki/abolqasem"
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = "latest"
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
    if ([string]::IsNullOrWhiteSpace($ReleaseBaseUrl)) {
        $Url = "https://github.com/$Repo/releases/latest/download/$Asset"
    } else {
        $Url = "$ReleaseBaseUrl/latest/download/$Asset"
    }
} else {
    if ([string]::IsNullOrWhiteSpace($ReleaseBaseUrl)) {
        $Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
    } else {
        $Url = "$ReleaseBaseUrl/download/$Version/$Asset"
    }
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
    if (Test-Path $InstallPath) {
        & $InstallPath service stop *> $null
    }
    Copy-Item -Path $Binary.FullName -Destination $InstallPath -Force
    & $InstallPath --help | Out-Null

    Write-Host "Installed $App to $InstallPath"
    if (($env:PATH -split ";") -notcontains $BinDir) {
        Write-Host "PATH notice: add this directory to PATH if needed:"
        Write-Host "  $BinDir"
    }

    Write-Host "Installing persistent service and all agent hooks"
    & $InstallPath install
    if ($LASTEXITCODE -ne 0) {
        throw "$App install failed with exit code $LASTEXITCODE"
    }
}
finally {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
