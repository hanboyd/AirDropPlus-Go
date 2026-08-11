[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Exe = Join-Path $ProjectRoot "dist\AirDropPlus-Go.exe"
$ConfigPath = Join-Path $ProjectRoot "dist\data\config.json"
$ReceivedRoot = [IO.Path]::GetFullPath((Join-Path $ProjectRoot "dist\received"))
if (-not (Test-Path -LiteralPath $Exe)) { throw "Missing build: $Exe" }

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
        ProcessId = $Proc.Id
    }
} finally {
    if (-not $Proc.HasExited) { Stop-Process -Id $Proc.Id; $Proc.WaitForExit(5000) | Out-Null }
}
