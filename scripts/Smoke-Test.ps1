[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Exe = Join-Path $ProjectRoot "dist\AirDropPlus-Go.exe"
$ConfigPath = Join-Path $ProjectRoot "dist\data\config.json"
$ReceivedRoot = [IO.Path]::GetFullPath((Join-Path $ProjectRoot "dist\received"))
if (-not (Test-Path -LiteralPath $Exe)) { throw "Missing build: $Exe" }
$PidFile = Join-Path $ProjectRoot "dist\data\airdropplus-go.pid"
if (Test-Path -LiteralPath $PidFile) {
    $StalePid = [int](Get-Content -Raw -LiteralPath $PidFile)
    $StaleProcess = Get-Process -Id $StalePid -ErrorAction SilentlyContinue
    if ($null -ne $StaleProcess) { throw "Refusing to replace PID file for a running process" }
    Remove-Item -LiteralPath $PidFile
}

$Proc = Start-Process -FilePath $Exe -WorkingDirectory $ProjectRoot -WindowStyle Hidden -PassThru
try {
    $Ready = $false
    for ($Attempt = 0; $Attempt -lt 50; $Attempt++) {
        Start-Sleep -Milliseconds 200
        try {
            $Health = Invoke-RestMethod -Uri "http://127.0.0.1:53317/healthz" -TimeoutSec 2
            if ($Health.success) { $Ready = $true; break }
        } catch {}
    }
    if (-not $Ready) { throw "Service did not become ready" }
    $Config = Get-Content -Raw -LiteralPath $ConfigPath | ConvertFrom-Json
    $Headers = @{ Authorization = $Config.token }

    $DuplicateError = Join-Path $ProjectRoot "dist\data\duplicate.stderr.log"
    $Duplicate = Start-Process -FilePath $Exe -WorkingDirectory $ProjectRoot -WindowStyle Hidden -PassThru -RedirectStandardError $DuplicateError
    if (-not $Duplicate.WaitForExit(5000)) { Stop-Process -Id $Duplicate.Id; throw "Duplicate process did not exit" }
    if ($Duplicate.ExitCode -eq 0) { throw "Duplicate process unexpectedly started" }
    if ([int](Get-Content -Raw -LiteralPath $PidFile) -ne $Proc.Id) { throw "Duplicate launch replaced the active PID file" }

    $Unauthorized = $false
    try { Invoke-WebRequest -Uri "http://127.0.0.1:53317/api/v1/clipboard" -SkipHttpErrorCheck -TimeoutSec 3 | ForEach-Object { $Unauthorized = $_.StatusCode -eq 401 } } catch {}
    if (-not $Unauthorized) { throw "Unauthenticated request was not rejected" }

    $Result = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:53317/api/v1/files" -Headers $Headers -Form @{ file = Get-Item -LiteralPath (Join-Path $ProjectRoot "NOTICE") } -TimeoutSec 10
    if (-not $Result.success -or $Result.data.Count -ne 1) { throw "File upload failed" }
    $Saved = [IO.Path]::GetFullPath((Join-Path $ReceivedRoot $Result.data[0]))
    if (-not $Saved.StartsWith($ReceivedRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw "Unexpected upload target" }
    if (-not (Test-Path -LiteralPath $Saved)) { throw "Uploaded file missing" }
    Remove-Item -LiteralPath $Saved

    [pscustomobject]@{
        Health = $Health.data.version
        UnauthorizedRejected = $Unauthorized
        AuthenticatedUpload = $true
        DuplicateRejected = $true
        ProcessId = $Proc.Id
    }
} finally {
    if (-not $Proc.HasExited) { Stop-Process -Id $Proc.Id; $Proc.WaitForExit(5000) | Out-Null }
    if (Test-Path -LiteralPath $PidFile) { Remove-Item -LiteralPath $PidFile }
}
