#requires -Version 5.1

[CmdletBinding(SupportsShouldProcess, ConfirmImpact = "Low")]
param(
    [string] $Repository = "https://github.com/coilyco/scoop-bucket"
)

$ErrorActionPreference = "Stop"

function Invoke-CheckedCommand {
    param(
        [Parameter(Mandatory)]
        [string] $Command,

        [string[]] $ArgumentList = @()
    )

    & $Command @ArgumentList
    if ($LASTEXITCODE -ne 0) {
        throw "$Command failed with exit code $LASTEXITCODE"
    }
}

function Normalize-Repository {
    param([Parameter(Mandatory)][string] $Value)

    return (($Value -replace "\.git/?$", "").TrimEnd("/"))
}

if (-not (Get-Command scoop -ErrorAction SilentlyContinue)) {
    throw "Scoop is not available on PATH"
}

$bucketRows = @(scoop bucket list)
if ($LASTEXITCODE -ne 0) {
    throw "scoop bucket list failed with exit code $LASTEXITCODE"
}

$normalizedRepository = Normalize-Repository $Repository
$bucketNames = @(
    $bucketRows |
        Where-Object { (Normalize-Repository ([string] $_.Source)) -eq $normalizedRepository } |
        Select-Object -ExpandProperty Name
)
if ($bucketNames.Count -eq 0) {
    throw "No Scoop bucket points at $Repository"
}

$installedRows = @(scoop list)
if ($LASTEXITCODE -ne 0) {
    throw "scoop list failed with exit code $LASTEXITCODE"
}

$appNames = @(
    $installedRows |
        Where-Object { [string] $_.Source -in $bucketNames } |
        Select-Object -ExpandProperty Name |
        Sort-Object -Unique
)
if ($appNames.Count -eq 0) {
    Write-Output "No installed apps come from Scoop bucket: $($bucketNames -join ', ')"
    return
}

Write-Output "Flight Deck Scoop apps: $($appNames -join ', ')"

if ("ward" -in $appNames) {
    if (-not (Get-Command ward -ErrorAction SilentlyContinue)) {
        throw "ward is installed through Scoop but is not available on PATH"
    }
    if ($PSCmdlet.ShouldProcess("ward", "Run the audited self-upgrade")) {
        Invoke-CheckedCommand ward @("upgrade")
    }
}

if ($PSCmdlet.ShouldProcess("Scoop and its buckets", "Refresh")) {
    Invoke-CheckedCommand scoop @("update")
}

$scoopApps = @($appNames | Where-Object { $_ -ne "ward" })
if (
    $scoopApps.Count -gt 0 -and
    $PSCmdlet.ShouldProcess(($scoopApps -join ", "), "Update through Scoop")
) {
    Invoke-CheckedCommand scoop (@("update") + $scoopApps)
}
