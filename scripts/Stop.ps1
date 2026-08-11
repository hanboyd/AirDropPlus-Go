[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$PidFile = Join-Path $ProjectRoot "dist\data\airdropplus-go.pid"
if (-not (Test-Path -LiteralPath $PidFile)) { Write-Host "未发现正在运行的 AirDropPlus-Go。"; exit 0 }
$ServicePid = [int](Get-Content -Raw -LiteralPath $PidFile)
$Process = Get-Process -Id $ServicePid -ErrorAction SilentlyContinue
if ($null -eq $Process) {
    Remove-Item -LiteralPath $PidFile
    Write-Host "进程已不存在，已清理残留 PID 文件。"
    exit 0
}
$ExpectedExe = [IO.Path]::GetFullPath((Join-Path $ProjectRoot "dist\AirDropPlus-Go.exe"))
if ([IO.Path]::GetFullPath($Process.Path) -ne $ExpectedExe) {
    throw "PID 文件与目标程序不匹配，已保持现状。"
}
Stop-Process -Id $ServicePid
$Process.WaitForExit(5000) | Out-Null
if (Test-Path -LiteralPath $PidFile) { Remove-Item -LiteralPath $PidFile }
Write-Host "AirDropPlus-Go 已停止。"
