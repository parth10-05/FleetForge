[CmdletBinding()]
param([Parameter(Mandatory)][string]$RunRoot)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$allowedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.fleetforge\demo-runs'))
$runPath = [IO.Path]::GetFullPath($RunRoot)
if (-not $runPath.StartsWith($allowedRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'run root is outside the FleetForge demo directory' }
$marker = Join-Path $runPath '.fleetforge-demo-owner'
if (-not (Test-Path -LiteralPath $marker -PathType Leaf)) { throw 'run root is not an owned FleetForge demo run' }
$processFile = Join-Path $runPath 'processes.json'
if (Test-Path -LiteralPath $processFile -PathType Leaf) {
    foreach ($entry in (Get-Content -LiteralPath $processFile -Raw | ConvertFrom-Json)) {
        try {
            $process = Get-Process -Id ([int]$entry.pid) -ErrorAction Stop
            $recordedStart = ([datetime]$entry.started_at).ToUniversalTime().ToString('O')
            if ($process.StartTime.ToUniversalTime().ToString('O') -eq $recordedStart) {
                Stop-Process -Id $process.Id -ErrorAction SilentlyContinue
                $null = $process.WaitForExit(5000)
            }
        } catch { }
    }
}
Remove-Item -LiteralPath $runPath -Recurse -Force
