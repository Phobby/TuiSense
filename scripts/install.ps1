# TuiSense installer for Windows (PowerShell 5.1+ or PowerShell 7).
#
#   irm https://raw.githubusercontent.com/Phobby/TuiSense/main/scripts/install.ps1 | iex
#
# Safer: download, read, then run it:
#
#   irm https://raw.githubusercontent.com/Phobby/TuiSense/main/scripts/install.ps1 -OutFile install-tuisense.ps1
#   notepad install-tuisense.ps1
#   powershell -ExecutionPolicy Bypass -File install-tuisense.ps1
#
# It downloads the official release zip from GitHub, verifies the signed
# checksum list with OpenSSH's ssh-keygen (included in Windows 10 1809+ and
# Windows 11) against the key pinned below, checks the zip's SHA-256, and
# installs tuisense.exe into %LOCALAPPDATA%\Programs\TuiSense\bin for the
# current user (no administrator rights). Settings and profiles are kept.
#
# Parameters: -Version X.Y.Z, -Uninstall, -Purge, -NoModifyPath
[CmdletBinding()]
param(
    [string]$Version = "",
    [switch]$Uninstall,
    [switch]$Purge,
    [switch]$NoModifyPath
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 3

$Repo = 'Phobby/TuiSense'
$Namespace = 'tuisense-release'
# Pinned release signing keys (ssh-ed25519). Keep in sync with
# internal/update/trusted_keys.txt and scripts/install.sh.
$TrustedKeys = @('ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILqV1bC0kFEN1Vxj3JnEp5ZieGsstZf0Atekky5WXFyv tuisense-release')

$ApiUrl = if ($env:TUISENSE_API_URL) { $env:TUISENSE_API_URL } else { 'https://api.github.com' }
$DownloadUrl = if ($env:TUISENSE_DOWNLOAD_URL) { $env:TUISENSE_DOWNLOAD_URL } else { "https://github.com/$Repo/releases/download" }
$InstallDir = if ($env:TUISENSE_INSTALL_DIR) { $env:TUISENSE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\TuiSense\bin' }
$Target = Join-Path $InstallDir 'tuisense.exe'

function Say([string]$Message) { Write-Host "[tuisense] $Message" }
function Fail([string]$Message) { throw "[tuisense] error: $Message" }

function Set-UserPath([bool]$Add) {
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    $parts = @()
    if ($current) { $parts = $current.Split(';') | Where-Object { $_ -and ($_ -ne $InstallDir) } }
    if ($Add) { $parts += $InstallDir }
    [Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')
}

if ($Uninstall) {
    foreach ($f in @($Target, "$Target.old", "$Target.new")) { if (Test-Path $f) { Remove-Item -Force $f } }
    if (-not $NoModifyPath) { Set-UserPath $false }
    Say "Removed $Target"
    if ($Purge) {
        foreach ($d in @((Join-Path $env:APPDATA 'tuisense'), (Join-Path $env:USERPROFILE '.local\share\tuisense'), (Join-Path $env:LOCALAPPDATA 'tuisense'))) {
            if (Test-Path $d) { Remove-Item -Recurse -Force $d }
        }
        Say 'Removed settings, profiles and data.'
    } else {
        Say "Kept settings and profiles in $(Join-Path $env:APPDATA 'tuisense')"
    }
    return
}

# --- platform ---------------------------------------------------------------
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { Fail "unsupported CPU architecture '$($env:PROCESSOR_ARCHITECTURE)'" }
}
$keygen = Get-Command ssh-keygen -ErrorAction SilentlyContinue
if (-not $keygen) { Fail 'ssh-keygen (OpenSSH Client) is required to verify the release signature. Enable it in Settings > System > Optional features.' }
if ($TrustedKeys.Count -eq 0 -and -not $env:TUISENSE_TRUSTED_KEYS_FILE) {
    Fail 'this installer has no pinned release signing key yet, so it cannot verify a download. Build from source instead (see README).'
}
$keys = @($TrustedKeys)
if ($env:TUISENSE_TRUSTED_KEYS_FILE) { $keys += Get-Content $env:TUISENSE_TRUSTED_KEYS_FILE | Where-Object { $_ -match 'ssh-ed25519' } }

[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("tuisense-" + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    # --- release ------------------------------------------------------------
    if (-not $Version) {
        $meta = Invoke-RestMethod -UseBasicParsing -Uri "$ApiUrl/repos/$Repo/releases/latest" -Headers @{ 'User-Agent' = 'tuisense-installer' }
        $Version = $meta.tag_name
    }
    $Version = $Version.TrimStart('v')
    if ($Version -notmatch '^[0-9A-Za-z.\-]+$') { Fail "invalid version '$Version'" }
    $archive = "tuisense_${Version}_windows_${arch}.zip"
    $base = "$DownloadUrl/v$Version"
    Say "Installing TuiSense v$Version for windows/$arch"
    foreach ($name in @('SHA256SUMS', 'SHA256SUMS.sig', $archive)) {
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$name" -OutFile (Join-Path $tmp $name)
    }

    # --- verification -------------------------------------------------------
    $signers = Join-Path $tmp 'allowed_signers'
    $lines = foreach ($k in $keys) {
        if ($k -match '(ssh-ed25519 [A-Za-z0-9+/=]+)') { "$Namespace $($Matches[1])" }
    }
    [IO.File]::WriteAllLines($signers, [string[]]$lines)
    $proc = Start-Process -FilePath $keygen.Source -NoNewWindow -Wait -PassThru `
        -ArgumentList @('-Y', 'verify', '-f', "`"$signers`"", '-I', $Namespace, '-n', $Namespace, '-s', "`"$(Join-Path $tmp 'SHA256SUMS.sig')`"") `
        -RedirectStandardInput (Join-Path $tmp 'SHA256SUMS') `
        -RedirectStandardOutput (Join-Path $tmp 'verify.out') -RedirectStandardError (Join-Path $tmp 'verify.err')
    if ($proc.ExitCode -ne 0) { Fail 'the checksum list is not signed by a trusted TuiSense key; nothing was installed' }
    $expected = $null
    foreach ($line in Get-Content (Join-Path $tmp 'SHA256SUMS')) {
        $parts = $line -split '\s+'
        if ($parts.Count -ge 2 -and $parts[1].TrimStart('*') -eq $archive) { $expected = $parts[0].ToLower() }
    }
    if (-not $expected) { Fail "$archive is not listed in the signed checksums" }
    $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLower()
    if ($actual -ne $expected) { Fail "checksum mismatch for $archive; nothing was installed" }
    Say 'Signature and checksum verified.'

    Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath (Join-Path $tmp 'x')
    $bin = Join-Path $tmp "x\tuisense_${Version}_windows_${arch}\tuisense.exe"
    if (-not (Test-Path $bin)) { Fail 'the package does not contain tuisense.exe' }
    $out = & $bin --version
    if ($out -notmatch [regex]::Escape("v$Version")) { Fail 'the downloaded executable did not start correctly; nothing was installed' }

    # --- install (a running tuisense.exe can be renamed, not overwritten) -----
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    Copy-Item $bin "$Target.new" -Force
    if (Test-Path $Target) {
        if (Test-Path "$Target.old") { Remove-Item -Force "$Target.old" -ErrorAction SilentlyContinue }
        Move-Item -Force $Target "$Target.old"
    }
    Move-Item -Force "$Target.new" $Target
    Say "Installed $Target"
    if (-not $NoModifyPath) {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (-not $userPath -or -not ($userPath.Split(';') -contains $InstallDir)) {
            Set-UserPath $true
            Say "Added $InstallDir to your user PATH (open a new terminal)."
        }
    }
    & $Target --version
    Say 'Run: tuisense   (help: tuisense --help, checks: tuisense doctor)'
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
