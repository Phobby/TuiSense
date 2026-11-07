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
# checksum list (OpenSSH SSHSIG, Ed25519; checked in this script, no extra
# tools needed) against the key pinned below, checks the zip's SHA-256, and
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

# --- signature verification ----------------------------------------------------
# Verifies an OpenSSH "SSHSIG" ssh-ed25519 signature (what `ssh-keygen -Y sign`
# writes) in plain .NET, because Windows 10's bundled OpenSSH (7.7) cannot run
# `ssh-keygen -Y verify`. Ed25519 per RFC 8032 section 5.1.7.
Add-Type -AssemblyName System.Numerics
$script:P = [Numerics.BigInteger]::Pow(2, 255) - 19
$script:L = [Numerics.BigInteger]::Pow(2, 252) + [Numerics.BigInteger]::Parse('27742317777372353535851937790883648493')
function ModP([Numerics.BigInteger]$x) { $r = $x % $script:P; if ($r.Sign -lt 0) { $r += $script:P }; $r }
function InvP([Numerics.BigInteger]$x) { [Numerics.BigInteger]::ModPow($x, $script:P - 2, $script:P) }
$script:D = ModP (-121665 * (InvP 121666))
$script:SqrtM1 = [Numerics.BigInteger]::ModPow(2, ($script:P - 1) / 4, $script:P)
function LeInt([byte[]]$b) { [Numerics.BigInteger]::new([byte[]]($b + [byte]0)) }
function EdAdd($p1, $p2) {
    $a = ModP (($p1[1] - $p1[0]) * ($p2[1] - $p2[0])); $b = ModP (($p1[1] + $p1[0]) * ($p2[1] + $p2[0]))
    $c = ModP (2 * $p1[3] * $p2[3] * $script:D); $dd = ModP (2 * $p1[2] * $p2[2])
    $e = $b - $a; $f = $dd - $c; $g = $dd + $c; $h = $b + $a
    @((ModP ($e * $f)), (ModP ($g * $h)), (ModP ($f * $g)), (ModP ($e * $h)))
}
function EdMul([Numerics.BigInteger]$s, $pt) {
    $q = @([Numerics.BigInteger]::Zero, [Numerics.BigInteger]::One, [Numerics.BigInteger]::One, [Numerics.BigInteger]::Zero)
    while ($s.Sign -gt 0) {
        if (-not $s.IsEven) { $q = EdAdd $q $pt }
        $pt = EdAdd $pt $pt
        $s = $s / 2
    }
    $q
}
function EdDecode([byte[]]$b) {
    if ($b.Length -ne 32) { return $null }
    $c = [byte[]]$b.Clone(); $sign = $c[31] -shr 7; $c[31] = $c[31] -band 0x7f
    $y = LeInt $c
    if ($y -ge $script:P) { return $null }
    $y2 = ModP ($y * $y)
    $x2 = ModP (($y2 - 1) * (InvP (ModP ($script:D * $y2 + 1))))
    $x = [Numerics.BigInteger]::ModPow($x2, ($script:P + 3) / 8, $script:P)
    if ((ModP ($x * $x - $x2)) -ne 0) { $x = ModP ($x * $script:SqrtM1) }
    if ((ModP ($x * $x - $x2)) -ne 0) { return $null }
    if ($x.IsZero -and $sign -eq 1) { return $null }
    if ([int]($x % 2) -ne $sign) { $x = $script:P - $x }
    @($x, $y, [Numerics.BigInteger]::One, (ModP ($x * $y)))
}
function EdVerify([byte[]]$pub, [byte[]]$msg, [byte[]]$sig) {
    if ($pub.Length -ne 32 -or $sig.Length -ne 64) { return $false }
    $pubPt = EdDecode $pub; $rPt = EdDecode ([byte[]]$sig[0..31])
    if ($null -eq $pubPt -or $null -eq $rPt) { return $false }
    $S = LeInt ([byte[]]$sig[32..63])
    if ($S -ge $script:L) { return $false }
    $h = [Security.Cryptography.SHA512]::Create().ComputeHash([byte[]]($sig[0..31] + $pub + $msg))
    $k = (LeInt $h) % $script:L
    $base = EdDecode ([byte[]](0x58, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66))
    $lhs = EdMul $S $base; $rhs = EdAdd $rPt (EdMul $k $pubPt)
    ((ModP ($lhs[0] * $rhs[2] - $rhs[0] * $lhs[2])) -eq 0) -and ((ModP ($lhs[1] * $rhs[2] - $rhs[1] * $lhs[2])) -eq 0)
}
# SSH wire format: uint32 big-endian length, then the bytes.
function SshStrings([byte[]]$b, [int]$at, [int]$count) {
    $out = @()
    for ($i = 0; $i -lt $count; $i++) {
        if ($at + 4 -gt $b.Length) { throw 'truncated signature' }
        $n = ([int]$b[$at] -shl 24) -bor ([int]$b[$at + 1] -shl 16) -bor ([int]$b[$at + 2] -shl 8) -bor [int]$b[$at + 3]
        $at += 4
        if ($n -lt 0 -or $at + $n -gt $b.Length) { throw 'truncated signature' }
        $out += , ([byte[]]$(if ($n) { $b[$at..($at + $n - 1)] } else { @() }))
        $at += $n
    }
    , $out
}
function SshString([byte[]]$b) { [byte[]]([BitConverter]::GetBytes([uint32]$b.Length)[3..0] + $b) }
function Test-SshSig([string]$Armored, [byte[]]$Data, [string[]]$Keys, [string]$Ns) {
    $m = [regex]::Match($Armored, '-----BEGIN SSH SIGNATURE-----(.*?)-----END SSH SIGNATURE-----', 'Singleline')
    if (-not $m.Success) { throw 'not an SSH signature file' }
    $blob = [Convert]::FromBase64String(($m.Groups[1].Value -replace '\s', ''))
    $ascii = [Text.Encoding]::ASCII
    if ($blob.Length -lt 10 -or $ascii.GetString($blob, 0, 6) -ne 'SSHSIG' -or $blob[9] -ne 1) { throw 'unsupported signature format' }
    $f = SshStrings $blob 10 5   # public key, namespace, reserved, hash algorithm, signature
    if ($ascii.GetString($f[1]) -ne $Ns) { throw "wrong signature namespace '$($ascii.GetString($f[1]))'" }
    $hashAlg = $ascii.GetString($f[3])
    $digest = switch ($hashAlg) {
        'sha512' { [Security.Cryptography.SHA512]::Create().ComputeHash($Data) }
        'sha256' { [Security.Cryptography.SHA256]::Create().ComputeHash($Data) }
        default { throw "unsupported hash '$hashAlg'" }
    }
    $sig = SshStrings $f[4] 0 2
    if ($ascii.GetString($sig[0]) -ne 'ssh-ed25519') { throw 'not an Ed25519 signature' }
    $pubKey = $f[0]
    $trusted = $false
    foreach ($k in $Keys) {
        # -cmatch: case-insensitive [A-Za-z] skips 'I' under the Turkish culture.
        if ($k -cmatch 'ssh-ed25519 ([A-Za-z0-9+/=]+)') {
            if ([Convert]::ToBase64String([Convert]::FromBase64String($Matches[1])) -eq [Convert]::ToBase64String($pubKey)) { $trusted = $true }
        }
    }
    if (-not $trusted) { throw 'signed by a key that is not a pinned TuiSense release key' }
    $pk = SshStrings $pubKey 0 2
    $signed = [byte[]]($ascii.GetBytes('SSHSIG') + (SshString $ascii.GetBytes($Ns)) + (SshString @()) + (SshString $ascii.GetBytes($hashAlg)) + (SshString $digest))
    EdVerify $pk[1] $signed $sig[1]
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
    if ($Version -cnotmatch '^[0-9A-Za-z.\-]+$') { Fail "invalid version '$Version'" }
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
    $sigText = [IO.File]::ReadAllText((Join-Path $tmp 'SHA256SUMS.sig'))
    $data = [IO.File]::ReadAllBytes((Join-Path $tmp 'SHA256SUMS'))
    $why = $null
    try { $ok = Test-SshSig $sigText $data $keys $Namespace } catch { $ok = $false; $why = $_.Exception.Message }
    if (-not $ok) {
        if (-not $why) { $why = 'the signature does not match a pinned release key' }
        Fail "the release signature could not be verified ($why), so nothing was installed."
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
