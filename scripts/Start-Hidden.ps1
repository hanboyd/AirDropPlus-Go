[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Exe = Join-Path $ProjectRoot "dist\AirDropPlus-Go.exe"
if (-not (Test-Path -LiteralPath $Exe)) { throw "请先运行 scripts\Build.ps1" }
Start-Process -FilePath $Exe -WorkingDirectory $ProjectRoot -WindowStyle Hidden
Write-Host "AirDropPlus-Go 已在后台启动。"
