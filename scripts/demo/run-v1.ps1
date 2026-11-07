[CmdletBinding()]
param([Parameter(Mandatory)][string]$RunRoot, [Parameter(Mandatory)][string]$Seed)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
if ([string]::IsNullOrWhiteSpace($env:DATABASE_URL) -or [string]::IsNullOrWhiteSpace($env:DATABASE_URL_UNPOOLED)) { throw 'DATABASE_URL and DATABASE_URL_UNPOOLED are required' }
$runPath = [IO.Path]::GetFullPath($RunRoot)
function New-DemoSecret { $bytes = [byte[]]::new(32); [Security.Cryptography.RandomNumberGenerator]::Fill($bytes); [Convert]::ToBase64String($bytes).TrimEnd('=') }
$env:FLEETFORGE_ENV='development'; $env:FLEETFORGE_API_ADDR='127.0.0.1:18080'; $env:FLEETFORGE_OBJECT_STORE_ADDR='127.0.0.1:18083'; $env:FLEETFORGE_SIMULATOR_ADDR='127.0.0.1:18081'; $env:FLEETFORGE_SIMULATOR_ADMIN_ADDR='127.0.0.1:18082'
$env:FLEETFORGE_SIMULATOR_RUN_ROOT=(Join-Path $runPath 'simulator-runs'); $env:FLEETFORGE_SIMULATOR_SCENARIO_PATH=(Join-Path $repoRoot 'test\fixtures\simulator\v1\healthy.json'); $env:FLEETFORGE_OBJECT_STORE_ROOT=(Join-Path $runPath 'objects')
$env:FLEETFORGE_SIMULATOR_USERNAME='demo'; $env:FLEETFORGE_SIMULATOR_PASSWORD=(New-DemoSecret); $env:FLEETFORGE_SIMULATOR_ADMIN_SECRET=(New-DemoSecret); $env:FLEETFORGE_LOCAL_AUTH_ISSUER='fleetforge-demo'; $env:FLEETFORGE_LOCAL_AUTH_AUDIENCE='fleetforge-demo'; $env:FLEETFORGE_LOCAL_AUTH_KEY=(New-DemoSecret); $env:FLEETFORGE_API_CURSOR_HMAC_SECRET=(New-DemoSecret); $env:FLEETFORGE_OBJECT_STORE_HMAC_SECRET=(New-DemoSecret)
$credentialFile = Join-Path $runPath 'synthetic-credentials.json'
$runIDFile = Join-Path $runPath 'simulator-run-id'
& go run ./cmd/fleetforge-demo -run-root $RunRoot -seed $Seed -token-file (Join-Path $runPath 'access.token') -scenario-path $env:FLEETFORGE_SIMULATOR_SCENARIO_PATH -credentials-file $credentialFile -simulator-run-id-file $runIDFile -simulator-username $env:FLEETFORGE_SIMULATOR_USERNAME -simulator-password $env:FLEETFORGE_SIMULATOR_PASSWORD
if ($LASTEXITCODE -ne 0) { throw 'demo initialization failed' }
$credentialJSON = Get-Content -LiteralPath $credentialFile -Raw
$env:FLEETFORGE_RECONCILIATION_JOURNAL_PATH = Join-Path $env:FLEETFORGE_SIMULATOR_RUN_ROOT ((Get-Content -LiteralPath $runIDFile -Raw).Trim() + '\journal.jsonl')
$env:FLEETFORGE_RECONCILIATION_RUN_ID = (Get-Content -LiteralPath $runIDFile -Raw).Trim()
$env:FLEETFORGE_DISCOVERY_ALLOWED_CIDRS='127.0.0.0/8'; $env:FLEETFORGE_DISCOVERY_CREDENTIALS_JSON=$credentialJSON
$env:FLEETFORGE_PRECHECK_ALLOWED_CIDRS='127.0.0.0/8'; $env:FLEETFORGE_PRECHECK_CREDENTIALS_JSON=$credentialJSON
$env:FLEETFORGE_EXECUTION_ALLOWED_CIDRS='127.0.0.0/8'; $env:FLEETFORGE_EXECUTION_CREDENTIALS_JSON=$credentialJSON
$processes = @()
try {
  $priorMigrationTimeout = $env:FLEETFORGE_POSTGRES_TRANSACTION_TIMEOUT
  try {
    # Fresh cloud databases can spend the normal request budget starting compute.
    $env:FLEETFORGE_POSTGRES_TRANSACTION_TIMEOUT = '1m'
    & go run ./cmd/migrate; if ($LASTEXITCODE -ne 0) { throw 'migration failed' }
  } finally {
    if ($null -eq $priorMigrationTimeout) { Remove-Item Env:FLEETFORGE_POSTGRES_TRANSACTION_TIMEOUT -ErrorAction SilentlyContinue } else { $env:FLEETFORGE_POSTGRES_TRANSACTION_TIMEOUT = $priorMigrationTimeout }
  }
  $binaryRoot = Join-Path $runPath 'bin'
  New-Item -ItemType Directory -Path $binaryRoot -Force | Out-Null
  foreach ($command in @('local-object-store','redfish-simulator','orchestrator-worker','discovery-worker','wakeup-worker','precheck-worker','execution-worker','observation-worker','reconciliation-worker','control-api')) {
    $binary = Join-Path $binaryRoot ($command + '.exe')
    & go build -o $binary ("./cmd/" + $command); if ($LASTEXITCODE -ne 0) { throw ($command + ' build failed') }
    $p = Start-Process -FilePath $binary -WorkingDirectory $repoRoot -RedirectStandardOutput (Join-Path $runPath ($command+'.out.log')) -RedirectStandardError (Join-Path $runPath ($command+'.err.log')) -PassThru -WindowStyle Hidden
    $processes += [pscustomobject]@{ pid=$p.Id; started_at=$p.StartTime.ToUniversalTime().ToString('O') }
  }
  $processes | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $runPath 'processes.json') -NoNewline
  $deadline = [datetime]::UtcNow.AddSeconds(30)
  do {
    try { $ready = Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:18080/health/ready' -TimeoutSec 2 } catch { $ready = $null }
    if ($ready -and $ready.StatusCode -eq 200) { break }
    Start-Sleep -Milliseconds 250
  } while ([datetime]::UtcNow -lt $deadline)
  if (-not $ready -or $ready.StatusCode -ne 200) { throw 'control API readiness failed' }
  $token = Get-Content -LiteralPath (Join-Path $runPath 'access.token') -Raw
  $authenticated = Invoke-WebRequest -UseBasicParsing -Uri 'http://127.0.0.1:18080/v1/targets?page_size=1' -Headers @{ Authorization = ('Bearer ' + $token) } -TimeoutSec 5
  if ($authenticated.StatusCode -ne 200) { throw 'authenticated control API check failed' }
  Write-Output ('FleetForge demo started at http://127.0.0.1:18080; run root: '+$runPath)
} catch { & (Join-Path $PSScriptRoot 'stop-v1.ps1') -RunRoot $runPath; throw }
