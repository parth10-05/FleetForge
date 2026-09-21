[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$root = Join-Path $PSScriptRoot '..\..\infra\environments\v1-dev'
Push-Location $root
try {
    terraform fmt -check -recursive ../..
    terraform init -backend=false
    terraform validate
    terraform test
}
finally {
    Pop-Location
}
