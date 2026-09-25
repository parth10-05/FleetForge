[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$SchemaRoot
)

$ErrorActionPreference = 'Stop'
$repositoryRoot = Split-Path -Parent $PSScriptRoot
$manifestPath = Join-Path $repositoryRoot 'test/fixtures/redfishsim/v1/schema-manifest.json'
$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
$head = (& git -C $SchemaRoot rev-parse HEAD).Trim()
if ($head -ne $manifest.source.commit) { throw "schema source commit mismatch: expected $($manifest.source.commit), got $head" }
foreach ($schema in $manifest.schemas) {
    $path = Join-Path $SchemaRoot $schema.path
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "selected schema is missing: $($schema.path)" }
    $actual = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $schema.sha256) { throw "checksum mismatch for $($schema.path): expected $($schema.sha256), got $actual" }
}
$licensePath = Join-Path $SchemaRoot $manifest.source.licensePath
$licenseHash = (Get-FileHash -LiteralPath $licensePath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($licenseHash -ne $manifest.source.licenseSHA256) { throw "license checksum mismatch: expected $($manifest.source.licenseSHA256), got $licenseHash" }
Write-Output "Verified $($manifest.schemas.Count) immutable selected schemas at $head."
