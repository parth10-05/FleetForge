[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$UserPoolId,
    [Parameter(Mandatory)]
    [string]$WebClientId,
    [switch]$Execute
)

$ErrorActionPreference = 'Stop'

if (-not $Execute) {
    Write-Output 'Dry run only. Set GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET in the process environment, then re-run with -Execute.'
    exit 0
}

foreach ($name in @('GOOGLE_OAUTH_CLIENT_ID', 'GOOGLE_OAUTH_CLIENT_SECRET')) {
    if ([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name))) {
        throw "$name must be supplied through the process environment. Do not add it to a tfvars file, command line, or repository file."
    }
}

if (-not (Get-Command aws -ErrorAction SilentlyContinue)) {
    throw 'AWS CLI is required.'
}

aws cognito-idp create-identity-provider `
    --user-pool-id $UserPoolId `
    --provider-name Google `
    --provider-type Google `
    --provider-details "client_id=$env:GOOGLE_OAUTH_CLIENT_ID,client_secret=$env:GOOGLE_OAUTH_CLIENT_SECRET,authorize_scopes=openid email profile" `
    --attribute-mapping email=email

aws cognito-idp update-user-pool-client `
    --user-pool-id $UserPoolId `
    --client-id $WebClientId `
    --supported-identity-providers COGNITO Google

Write-Output 'Google federation configured. Record only the command outcome and redacted provider metadata as evidence.'
