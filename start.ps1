$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    if (Test-Path -LiteralPath '.env.local') {
        Get-Content -LiteralPath '.env.local' | ForEach-Object {
            if ($_ -match '^\s*(OPENAI_[A-Z_]+)\s*=(.*)$') {
                $settingName = $matches[1]
                $settingValue = $matches[2].Trim().Trim('"').Trim("'")
                [Environment]::SetEnvironmentVariable($settingName, $settingValue, 'Process')
            }
        }
    }
    $env:GOCACHE = Join-Path $PSScriptRoot '.gocache'
    go run . @args
    $serverExitCode = $LASTEXITCODE
} finally {
    Pop-Location
}
exit $serverExitCode
