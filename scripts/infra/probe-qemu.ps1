[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidatePattern('^i-[0-9a-f]+$')]
    [string]$InstanceId,
    [switch]$Execute
)

$ErrorActionPreference = 'Stop'

if (-not $Execute) {
    Write-Output "Dry run only. Would submit a bounded QEMU capability probe through SSM for $InstanceId."
    exit 0
}

if (-not (Get-Command aws -ErrorAction SilentlyContinue)) {
    throw 'AWS CLI is required.'
}

$command = 'set -eu; command -v qemu-system-x86_64; qemu-system-x86_64 --version | head -n 1'
$response = aws ssm send-command `
    --instance-ids $InstanceId `
    --document-name AWS-RunShellScript `
    --parameters "commands=$command" `
    --timeout-seconds 60 `
    --output json | ConvertFrom-Json

[pscustomobject]@{
    InstanceId = $InstanceId
    CommandId  = $response.Command.CommandId
    Result     = 'submitted'
    Evidence   = 'Capture redacted command status and version result outside this repository.'
} | ConvertTo-Json -Compress
