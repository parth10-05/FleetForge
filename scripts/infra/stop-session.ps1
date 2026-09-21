[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidatePattern('^i-[0-9a-f]+$')]
    [string]$InstanceId,
    [switch]$Execute
)

$ErrorActionPreference = 'Stop'

if (-not $Execute) {
    Write-Output "Dry run only. Would stop simulator instance $InstanceId."
    exit 0
}

if (-not (Get-Command aws -ErrorAction SilentlyContinue)) {
    throw 'AWS CLI is required.'
}

aws ec2 stop-instances --instance-ids $InstanceId --output json | Out-Null
Write-Output "Stop request submitted for $InstanceId. Confirm termination of active simulator work before reuse."
