param(
    [string]$Repo = $env:AI_AGENT_MANAGER_REPO,
    [string]$Version = $env:AI_AGENT_MANAGER_VERSION,
    [string]$ReleaseBaseUrl = $env:AI_AGENT_MANAGER_RELEASE_BASE_URL,
    [string]$BinDir = $env:BIN_DIR,
    [string]$Scope = $env:AI_AGENT_MANAGER_HOOK_SCOPE,
    [string]$Agents = $env:AI_AGENT_MANAGER_AGENTS,
    [ValidateSet("hook", "service")]
    [string]$Startup = $env:AI_AGENT_MANAGER_STARTUP,
    [switch]$Hooks
)

$ErrorActionPreference = "Stop"

$App = "ai-agent-manager"
if ([string]::IsNullOrWhiteSpace($Repo)) {
    $Repo = "moosavimaleki/ai-agent-manager"
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
if ([string]::IsNullOrWhiteSpace($Startup)) {
    $Startup = "hook"
}
if ($env:AI_AGENT_MANAGER_INSTALL_HOOKS -ne "0") {
    $Hooks = $true
}
if (-not $PSBoundParameters.ContainsKey("Hooks")) {
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
    Copy-Item -Path $Binary.FullName -Destination $InstallPath -Force
    & $InstallPath --help | Out-Null

    Write-Host "Installed $App to $InstallPath"
    if (($env:PATH -split ";") -notcontains $BinDir) {
        Write-Host "PATH notice: add this directory to PATH if needed:"
        Write-Host "  $BinDir"
    }

    if ($Hooks) {
        if ($Agents -eq "all") {
            $env:AI_AGENT_MANAGER_SUPPRESS_TRUST_NOTICE = "1"
            & $InstallPath install --all --scope $Scope --startup $Startup
        } else {
            foreach ($Agent in ($Agents -split "[,\s]+" | Where-Object { $_ })) {
                $env:AI_AGENT_MANAGER_SUPPRESS_TRUST_NOTICE = "1"
                & $InstallPath install --agent $Agent --scope $Scope --startup $Startup
            }
        }
        Remove-Item Env:AI_AGENT_MANAGER_SUPPRESS_TRUST_NOTICE -ErrorAction SilentlyContinue
    } elseif ($Startup -eq "service") {
        & $InstallPath install --startup service --no-hooks
    }

    Write-Host ""
    Write-Host "Next step:"
    Write-Host "  Open Codex, Claude Code, and Gemini CLI once."
    Write-Host "  If any of them asks you to trust or approve the installed hook command, accept it."
    Write-Host "  No manual config editing should be needed."
}
finally {
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
