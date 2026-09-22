[CmdletBinding(SupportsShouldProcess)]
param(
    [switch]$Execute,
    [string]$Confirmation
)

$ErrorActionPreference = 'Stop'

if (-not $Execute) {
    Write-Output 'Dry run only. Re-run with -Execute after reviewing a saved destroy plan.'
    exit 0
}

if ($Confirmation -ne 'DESTROY-SYNTHETIC-FLEETFORGE') {
    throw 'Confirmation must exactly equal DESTROY-SYNTHETIC-FLEETFORGE.'
}

$environmentRoot = Join-Path $PSScriptRoot '..\..\infra\environments\v1-dev'
Push-Location $environmentRoot
try {
    terraform init
    terraform plan -destroy -out=destroy.tfplan
    if ($PSCmdlet.ShouldProcess('v1-dev infrastructure', 'destroy')) {
        terraform apply destroy.tfplan
    }
}
finally {
    Pop-Location
}
