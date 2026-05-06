# lazyport installer for Windows (PowerShell 5.1+).
#
# Usage:
#   irm https://raw.githubusercontent.com/papasaidfine/lazyport/main/install.ps1 | iex
#
# Environment overrides (set before running):
#   $env:VERSION      — release tag to install (default: latest, including prereleases)
#   $env:INSTALL_DIR  — where to drop the binary (default: %LOCALAPPDATA%\Programs\lazyport)

#Requires -Version 5.1

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$Repo = 'papasaidfine/lazyport'
$Bin  = 'lazyport'

$Version = if ($env:VERSION) { $env:VERSION } else { 'latest' }

if ($env:INSTALL_DIR) {
    $InstallDir = $env:INSTALL_DIR
} else {
    $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\$Bin"
}

function Write-Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }
function Write-Warn($msg) { Write-Host "!!! $msg" -ForegroundColor Yellow }
function Die($msg)        { Write-Host "xxx $msg" -ForegroundColor Red; exit 1 }

function Get-Arch {
    $a = $env:PROCESSOR_ARCHITECTURE
    switch ($a) {
        'AMD64' { return 'amd64' }
        'ARM64' {
            # We only ship windows/amd64; ARM64 will run it under emulation,
            # but warn the user.
            Write-Warn "Windows ARM64 detected — installing the amd64 build (runs under x64 emulation)."
            return 'amd64'
        }
        default { Die "unsupported arch: $a" }
    }
}

function Resolve-Version($v) {
    if ($v -ne 'latest') { return $v }
    # /releases/latest excludes prereleases — we want any most-recent release.
    $list = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases" `
                              -Headers @{ 'User-Agent' = "$Bin-installer" }
    if (-not $list -or $list.Count -eq 0) { Die "no releases found in $Repo" }
    return $list[0].tag_name
}

function Verify-Sha256($file, $expected) {
    $hash = (Get-FileHash -Algorithm SHA256 -Path $file).Hash.ToLower()
    if ($hash -ne $expected.ToLower()) {
        Die "checksum mismatch for $(Split-Path -Leaf $file): got $hash, expected $expected"
    }
}

function Add-ToUserPath($dir) {
    $current = [Environment]::GetEnvironmentVariable('PATH', 'User')
    if (-not $current) { $current = '' }
    $entries = $current -split ';' | Where-Object { $_ -ne '' }
    if ($entries -contains $dir) { return $false }
    $new = if ($current -eq '') { $dir } else { "$current;$dir" }
    [Environment]::SetEnvironmentVariable('PATH', $new, 'User')
    return $true
}

# --- main ---

$arch    = Get-Arch
$version = Resolve-Version $Version
$stripped = $version -replace '^v', ''
$asset   = "${Bin}_${stripped}_windows_${arch}.zip"
$base    = "https://github.com/$Repo/releases/download/$version"

Write-Step "installing $Bin $version for windows/$arch"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    $assetPath = Join-Path $tmp $asset
    $sumPath   = Join-Path $tmp 'checksums.txt'

    Write-Step "downloading $asset"
    Invoke-WebRequest -Uri "$base/$asset" -OutFile $assetPath -UseBasicParsing

    Write-Step "downloading checksums.txt"
    Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile $sumPath -UseBasicParsing

    # checksums.txt format: "<sha256>  <filename>"
    $expected = (Get-Content $sumPath |
                 Where-Object { $_ -match "\s$([regex]::Escape($asset))\s*$" } |
                 ForEach-Object { ($_ -split '\s+')[0] } |
                 Select-Object -First 1)
    if (-not $expected) { Die "no checksum entry for $asset" }
    Verify-Sha256 $assetPath $expected
    Write-Step 'checksum OK'

    Expand-Archive -Path $assetPath -DestinationPath $tmp -Force
    $exePath = Join-Path $tmp "$Bin.exe"
    if (-not (Test-Path $exePath)) { Die "archive did not contain $Bin.exe" }

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    $dest = Join-Path $InstallDir "$Bin.exe"
    Copy-Item -Path $exePath -Destination $dest -Force

    Write-Step "installed → $dest"

    if (Add-ToUserPath $InstallDir) {
        Write-Step "added $InstallDir to your user PATH (open a new terminal to pick it up)"
    } else {
        $sessionPath = $env:Path -split ';'
        if ($sessionPath -notcontains $InstallDir) {
            Write-Warn "$InstallDir is on your user PATH but not the current shell — open a new terminal."
        }
    }

    & $dest --version
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
