[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$root = Join-Path $PSScriptRoot '..\..'
$preflight = Join-Path $PSScriptRoot 'preflight.ps1'
$fixtures = Join-Path $root 'docs\evidence\infrastructure\fixtures'

foreach ($fixture in @('expired-plan.json', 'ownership-collision.json')) {
    $failed = $false
    try {
        & $preflight -FixturePath (Join-Path $fixtures $fixture)
    }
    catch {
        $failed = $true
    }
    if (-not $failed) {
        throw "expected $fixture to fail preflight"
    }
}
