[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$InputPath,
    [Parameter(Mandatory)]
    [string]$OutputPath
)

$ErrorActionPreference = 'Stop'
$text = Get-Content -LiteralPath $InputPath -Raw
$patterns = @(
    '(?i)(aws_access_key_id|aws_secret_access_key|session_token|password|token|client_secret|database_url)\\s*[:=]\\s*[^\\s,]+',
    '(?i)arn:aws:[^\\s,]+'
)
foreach ($pattern in $patterns) {
    $text = [regex]::Replace($text, $pattern, '[REDACTED]')
}
Set-Content -LiteralPath $OutputPath -Value $text -NoNewline
