$ErrorActionPreference = "Stop"

function usage {
    param([string]$this)

    [Console]::Error.WriteLine(@"
${this}: download binaries for OVHcloud CLI

Usage: ${this} [-b <bindir>] [-d] [<tag>]
  -b sets bindir or installation directory, Defaults to ./bin
  -d turns on debug logging
   <tag> is a tag from
   https://github.com/ovh/ovhcloud-cli/releases
   If tag is missing, then the latest will be used.

"@)
    exit 2
}

function parse_args {
    param(
        [string]$this,
        [string[]]$arguments
    )

    if (-not $script:BINDIR) {
        foreach ($dir in @($env:XDG_BIN_HOME, $(if ($HOME) { Join-Path $HOME ".local/bin" }))) {
            if ($dir -and (Test-Path -LiteralPath $dir -PathType Container)) {
                $script:BINDIR = $dir
                break
            }
        }
    }
    if (-not $script:BINDIR) {
        $script:BINDIR = "bin"
    }

    for ($index = 0; $index -lt $arguments.Count; $index++) {
        $argument = $arguments[$index]

        if ($argument -eq "--") {
            $index++
            break
        }

        if (-not $argument.StartsWith("-") -or $argument -eq "-") {
            break
        }

        for ($optionIndex = 1; $optionIndex -lt $argument.Length; $optionIndex++) {
            $option = $argument[$optionIndex]

            switch ($option) {
                "b" {
                    if ($optionIndex + 1 -lt $argument.Length) {
                        $script:BINDIR = $argument.Substring($optionIndex + 1)
                        $optionIndex = $argument.Length
                        continue
                    }

                    $index++
                    if ($index -ge $arguments.Count) {
                        usage $this
                    }

                    $script:BINDIR = $arguments[$index]
                    break
                }
                "d" {
                    log_set_priority 10
                }
                "h" {
                    usage $this
                }
                "?" {
                    usage $this
                }
                "x" {
                    Set-PSDebug -Trace 1
                }
                default {
                    usage $this
                }
            }
        }
    }

    if ($index -lt $arguments.Count) {
        $script:TAG = $arguments[$index]
    }
}

function execute {
    $tmpdir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString("N"))
    log_debug "downloading files into ${tmpdir}"

    New-Item -ItemType Directory -Path $tmpdir | Out-Null
    try {
        if (-not (http_download (Join-Path $tmpdir $script:ARCHIVE) $script:ARCHIVE_URL)) {
            exit 1
        }
        extract_archive (Join-Path $tmpdir $script:ARCHIVE) $tmpdir

        if (-not (Test-Path -LiteralPath $script:BINDIR -PathType Container)) {
            New-Item -ItemType Directory -Path $script:BINDIR -Force | Out-Null
        }

        $binary = $script:BINARY
        if ($script:OS -eq "Windows") {
            $binary = "${binary}.exe"
        }

        Copy-Item -LiteralPath (Join-Path $tmpdir $binary) -Destination (Join-Path $script:BINDIR $binary) -Force
        log_info "installed $((Join-Path $script:BINDIR $binary))"
    } finally {
        if (Test-Path -LiteralPath $tmpdir) {
            Remove-Item -LiteralPath $tmpdir -Recurse -Force
        }
    }
}

function tag_to_version {
    if ([string]::IsNullOrWhiteSpace($script:TAG)) {
        log_info "checking GitHub for latest tag"
    } else {
        log_info "checking GitHub for tag '$($script:TAG)'"
    }

    $realtag = github_release "$($script:OWNER)/$($script:REPO)" $script:TAG
    if ([string]::IsNullOrWhiteSpace($realtag)) {
        log_crit "unable to find '$($script:TAG)' - use 'latest' or see https://github.com/$($script:PREFIX)/releases for details"
        exit 1
    }

    $script:TAG = $realtag
    $script:VERSION = $script:TAG.TrimStart("v")
}

function adjust_format {
    switch ($script:OS) {
        "Windows" { $script:FORMAT = "zip" }
    }
}

$script:_logp = 6
function echoerr {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$message)
    [Console]::Error.WriteLine(($message -join " "))
}
function log_set_priority {
    param([int]$priority)
    $script:_logp = $priority
}
function log_priority {
    param([int]$priority)
    return $priority -le $script:_logp
}
function log_tag {
    param([int]$priority)
    switch ($priority) {
        0 { return "emerg" }
        1 { return "alert" }
        2 { return "crit" }
        3 { return "err" }
        4 { return "warning" }
        5 { return "notice" }
        6 { return "info" }
        7 { return "debug" }
        default { return [string]$priority }
    }
}
function log_debug {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$message)
    if (log_priority 7) {
        echoerr (log_prefix) (log_tag 7) $message
    }
}
function log_info {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$message)
    if (log_priority 6) {
        echoerr (log_prefix) (log_tag 6) $message
    }
}
function log_err {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$message)
    if (log_priority 3) {
        echoerr (log_prefix) (log_tag 3) $message
    }
}
function log_crit {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$message)
    if (log_priority 2) {
        echoerr (log_prefix) (log_tag 2) $message
    }
}

function uname_os {
    return "Windows"
}

function uname_os_check {
    param([string]$os)

    switch ($os) {
        "Darwin" { return }
        "Linux" { return }
        "Windows" { return }
    }

    log_crit "uname_os_check '$os' is not a GOOS value."
    exit 1
}

function extract_archive {
    param(
        [string]$archive,
        [string]$destination
    )

    switch -Wildcard ($archive) {
        "*.zip" {
            Expand-Archive -LiteralPath $archive -DestinationPath $destination -Force
            return
        }
        default {
            log_err "extract_archive unknown archive format for $archive"
            exit 1
        }
    }
}

function http_download {
    param(
        [string]$local_file,
        [string]$source_url,
        [string]$header
    )

    log_debug "http_download $source_url"

    $headers = @{ "User-Agent" = $script:PREFIX }
    if ($header) {
        $name, $value = $header -split ":", 2
        if ($name -and $value) {
            $headers[$name.Trim()] = $value.Trim()
        }
    }

    try {
        Invoke-WebRequest -Uri $source_url -OutFile $local_file -Headers $headers | Out-Null
    } catch {
        log_err "http_download received an error while downloading $source_url"
        return $false
    }

    return $true
}

function http_copy {
    param(
        [string]$source_url,
        [string]$header
    )

    $tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString("N"))
    if (-not (http_download $tmp $source_url $header)) {
        return ""
    }

    try {
        return Get-Content -LiteralPath $tmp -Raw
    } finally {
        if (Test-Path -LiteralPath $tmp) {
            Remove-Item -LiteralPath $tmp -Force
        }
    }
}

function github_release {
    param(
        [string]$owner_repo,
        [string]$version
    )

    if ([string]::IsNullOrWhiteSpace($version)) {
        $version = "latest"
    }

    $giturl = "https://github.com/${owner_repo}/releases/${version}"
    $json = http_copy $giturl "Accept:application/json"
    if ([string]::IsNullOrWhiteSpace($json)) {
        return ""
    }

    try {
        $release = $json | ConvertFrom-Json
    } catch {
        return ""
    }

    if (-not $release.tag_name) {
        return ""
    }

    return [string]$release.tag_name
}

$script:OWNER = "ovh"
$script:REPO = "ovhcloud-cli"
$script:BINARY = "ovhcloud"
$script:FORMAT = "tar.gz"
$script:OS = uname_os
$script:ARCH = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() | ForEach-Object {
    switch ($_) {
        "X64" { "x86_64" }
        "X86" { "i386" }
        "Arm64" { "arm64" }
        "Arm" { "arm" }
        default { throw "unsupported architecture: $_" }
    }
}
$script:PREFIX = "$($script:OWNER)/$($script:REPO)"

function log_prefix {
    return $script:PREFIX
}

$script:GITHUB_DOWNLOAD = "https://github.com/$($script:OWNER)/$($script:REPO)/releases/download"

uname_os_check $script:OS

$scriptName = "install.ps1"
if ($PSCommandPath) {
    $scriptName = Split-Path -Leaf $PSCommandPath
}

parse_args $scriptName $args
tag_to_version
adjust_format

log_info "found version: $($script:VERSION) for $($script:TAG)/$($script:OS)/$($script:ARCH)"

$script:NAME = "$($script:REPO)_$($script:OS)_$($script:ARCH)"
$script:ARCHIVE = "$($script:NAME).$($script:FORMAT)"
$script:ARCHIVE_URL = "$($script:GITHUB_DOWNLOAD)/$($script:TAG)/$($script:ARCHIVE)"

execute
