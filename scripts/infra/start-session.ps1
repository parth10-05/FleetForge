[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidatePattern('^i-[0-9a-f]+$')]
    [string]$InstanceId,
    [switch]$Execute
)

$ErrorActionPreference = 'Stop'

if (-not $Execute) {
    Write-Output "Dry run only. Would start an SSM session for $InstanceId; no SSH path is supported."
    exit 0
}

if (-not (Get-Command aws -ErrorAction SilentlyContinue)) {
    throw 'AWS CLI with the Session Manager plugin is required.'
}

$identity = aws sts get-caller-identity --output json | ConvertFrom-Json
if ([string]::IsNullOrWhiteSpace($identity.Account)) {
    throw 'Unable to verify AWS identity.'
}

$instance = aws ec2 describe-instances --instance-ids $InstanceId --output json | ConvertFrom-Json
$state = $instance.Reservations[0].Instances[0].State.Name
if ($state -ne 'running') {
    throw "Instance $InstanceId is '$state'; it must be running before an SSM session can start."
}

aws ssm start-session --target $InstanceId
