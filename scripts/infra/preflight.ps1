[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateScript({ Test-Path -LiteralPath $_ -PathType Leaf })]
    [string]$FixturePath
)

$ErrorActionPreference = 'Stop'

# This foundation deliberately reads a synthetic JSON fixture only. It does not invoke
# aws, Terraform, network APIs, or any credential source.
$fixture = Get-Content -LiteralPath $FixturePath -Raw | ConvertFrom-Json
$required = @('planState', 'expiresAt', 'accountId', 'region', 'resources')
foreach ($field in $required) {
    if (-not $fixture.PSObject.Properties.Name.Contains($field)) {
        throw "preflight fixture is missing required field '$field'"
    }
}

if ($fixture.planState -ne 'active') {
    throw "preflight rejected plan state '$($fixture.planState)'"
}

$expiresAt = [DateTimeOffset]::Parse($fixture.expiresAt)
if ($expiresAt -le [DateTimeOffset]::UtcNow.AddDays(7)) {
    throw 'preflight rejected an expiry within seven days'
}

if ($fixture.region -ne 'us-east-1') {
    throw "preflight rejected region '$($fixture.region)'"
}

foreach ($resource in $fixture.resources) {
    if ($resource.name -like 'ps-fleetforge-v1-dev-*' -and $resource.owner -ne 'ps') {
        throw "preflight rejected ownership collision for '$($resource.name)'"
    }
}

[pscustomobject]@{
    Result  = 'pass'
    Mode    = 'fixture-only'
    Region  = $fixture.region
    Checked = $required
} | ConvertTo-Json -Compress
