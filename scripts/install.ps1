# TuiSense installer for Windows (PowerShell 5.1+ or PowerShell 7).
#
# From Command Prompt (cmd), PowerShell or Windows Terminal:
#
#   powershell -NoProfile -Command "irm https://raw.githubusercontent.com/Phobby/TuiSense/main/scripts/install.ps1 | iex"
#
# Inside PowerShell only:
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
# Windows PowerShell 5.1 downloads many times slower while drawing progress.
$ProgressPreference = 'SilentlyContinue'
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
# The OS architecture, not the PowerShell process's: 32-bit (x86) PowerShell on
# 64-bit Windows reports x86 in PROCESSOR_ARCHITECTURE.
$osArch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) { $osArch = $env:PROCESSOR_ARCHITEW6432 }
try { $osArch = [string][System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture } catch { }
$arch = switch ($osArch.ToUpperInvariant()) {
    'AMD64' { 'amd64' }
    'X64'   { 'amd64' }
    'ARM64' { 'arm64' }
    default { Fail "unsupported CPU architecture '$osArch'. TuiSense is published for 64-bit Windows (x64 and ARM64)." }
}
$keygen = Get-Command ssh-keygen -ErrorAction SilentlyContinue
if (-not $keygen) {
    Fail ("ssh-keygen (Windows' OpenSSH Client) is required to verify the release signature, and it was not found. " +
        "Add it in Settings > System > Optional features > OpenSSH Client (or install Git for Windows, which includes it), open a new terminal and run the installer again. " +
        "Alternatively follow the manual download and verification steps in the README.")
}
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
        try {
            $meta = Invoke-RestMethod -UseBasicParsing -Uri "$ApiUrl/repos/$Repo/releases/latest" -Headers @{ 'User-Agent' = 'tuisense-installer' }
        } catch {
            Fail "no published TuiSense release was found (or GitHub could not be reached). See https://github.com/$Repo/releases"
        }
        $Version = $meta.tag_name
    }
    $Version = $Version.TrimStart('v')
    if ($Version -notmatch '^[0-9A-Za-z.\-]+$') { Fail "invalid version '$Version'" }
    $archive = "tuisense_${Version}_windows_${arch}.zip"
    $base = "$DownloadUrl/v$Version"
    Say "Installing TuiSense v$Version for windows/$arch"
    foreach ($name in @('SHA256SUMS', 'SHA256SUMS.sig', $archive)) {
        try {
            Invoke-WebRequest -UseBasicParsing -Uri "$base/$name" -OutFile (Join-Path $tmp $name)
        } catch {
            Fail "could not download $name from $base ($($_.Exception.Message)). Check the internet connection, proxy or firewall, and that release v$Version has a windows/$arch build. Nothing was installed."
        }
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
    if ($proc.ExitCode -ne 0) {
        $why = (Get-Content -Raw -ErrorAction SilentlyContinue (Join-Path $tmp 'verify.err'))
        if (-not $why) { $why = "exit code $($proc.ExitCode)" }
        Fail ("the release signature could not be verified, so nothing was installed. ssh-keygen said: $($why.Trim()). " +
            "If it reports an unknown option, this OpenSSH is older than 8.1; update OpenSSH Client or install Git for Windows.")
    }
    $expected = $null
    foreach ($line in Get-Content (Join-Path $tmp 'SHA256SUMS')) {
        $parts = $line -split '\s+'
        if ($parts.Count -ge 2 -and $parts[1].TrimStart('*') -eq $archive) { $expected = $parts[0].ToLower() }
    }
    if (-not $expected) { Fail "$archive is not listed in the signed checksums" }
    # .NET directly: Get-FileHash and Expand-Archive live in script modules that
    # fail to load when Windows PowerShell 5.1 is started from PowerShell 7
    # (it inherits PowerShell 7's PSModulePath).
    $stream = [IO.File]::OpenRead((Join-Path $tmp $archive))
    try { $hash = [Security.Cryptography.SHA256]::Create().ComputeHash($stream) } finally { $stream.Dispose() }
    $actual = ([BitConverter]::ToString($hash) -replace '-', '').ToLower()
    if ($actual -ne $expected) { Fail "checksum mismatch for $archive; nothing was installed" }
    Say 'Signature and checksum verified.'

    try {
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        [IO.Compression.ZipFile]::ExtractToDirectory((Join-Path $tmp $archive), (Join-Path $tmp 'x'))
    } catch {
        Fail "the verified archive could not be extracted ($($_.Exception.Message)); nothing was installed"
    }
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
            Say "Added $InstallDir to your user PATH."
        }
        # Usable in this window too; other open terminals need a restart.
        if (-not ($env:Path.Split(';') -contains $InstallDir)) { $env:Path = "$env:Path;$InstallDir" }
    }
    & $Target --version
    Say 'Run: tuisense   (help: tuisense --help, checks: tuisense doctor)'
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
