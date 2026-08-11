[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$PidFile = Join-Path $ProjectRoot "dist\data\airdropplus-go.pid"
if (-not (Test-Path -LiteralPath $PidFile)) { Write-Host "未发现正在运行的 AirDropPlus-Go。"; exit 0 }
$ServicePid = [int](Get-Content -Raw -LiteralPath $PidFile)
$Process = Get-Process -Id $ServicePid -ErrorAction SilentlyContinue
if ($null -eq $Process -or $Process.Path -ne (Join-Path $ProjectRoot "dist\AirDropPlus-Go.exe")) {
    throw "PID 文件与目标程序不匹配，已保持现状。"
}
Stop-Process -Id $ServicePid
Write-Host "AirDropPlus-Go 已停止。"
