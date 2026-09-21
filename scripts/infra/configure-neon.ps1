[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateSet('development', 'staging', 'demo')]
    [string]$Environment,
    [switch]$Execute
)

$ErrorActionPreference = 'Stop'

if (-not $Execute) {
    [pscustomobject]@{
        Result      = 'dry-run'
        Environment = $Environment
        Required    = @('NEON_API_KEY', 'NEON_PROJECT_ID')
        Policy      = 'Create separate application, migration, and read-only roles; store DSNs only in the designated secret manager during the credentialed run.'
    } | ConvertTo-Json -Compress
    exit 0
}

foreach ($name in @('NEON_API_KEY', 'NEON_PROJECT_ID')) {
    if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name))) {
        throw "$name must be supplied through the process environment; never use Terraform variables, tfvars, or committed configuration."
    }
}

if (-not (Get-Command neonctl -ErrorAction SilentlyContinue)) {
    throw 'neonctl is required for execution. Install it separately and authenticate only through the process environment.'
}

Write-Output "Prepare $Environment database roles using the approved Neon project. This helper intentionally does not print or persist connection strings."
throw 'Execution is intentionally gated until the reviewed Neon role-creation command set is recorded with the selected project and retention tier.'
