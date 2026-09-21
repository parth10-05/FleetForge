[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateSet('v1-dev')]
    [string]$Environment
)

$ErrorActionPreference = 'Stop'

if (-not (Get-Command aws -ErrorAction SilentlyContinue)) {
    throw 'AWS CLI is required for live verification. This script does not accept credentials as arguments.'
}

$identity = aws sts get-caller-identity --output json | ConvertFrom-Json
if (-not $identity.Account) {
    throw 'Unable to confirm the active AWS account.'
}

$prefix = "ps-fleetforge-$Environment-"
$queues = aws sqs list-queues --queue-name-prefix $prefix --output json | ConvertFrom-Json
$repositories = aws ecr describe-repositories --repository-names "${prefix}service" --output json 2>$null

[pscustomobject]@{
    Account          = $identity.Account
    Environment      = $Environment
    QueueUrls        = @($queues.QueueUrls).Count
    RegistryDetected = [bool]$repositories
    Result           = 'manual-review-required'
} | ConvertTo-Json
