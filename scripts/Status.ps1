[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Exe = [IO.Path]::GetFullPath((Join-Path $ProjectRoot "dist\AirDropPlus-Go.exe"))
$PidFile = Join-Path $ProjectRoot "dist\data\airdropplus-go.pid"
$ConfigFile = Join-Path $ProjectRoot "dist\data\config.json"
if (-not (Test-Path -LiteralPath $PidFile)) { Write-Host "AirDropPlus-Go 未运行。"; exit 1 }
$ServicePid = [int](Get-Content -Raw -LiteralPath $PidFile)
$Process = Get-Process -Id $ServicePid -ErrorAction SilentlyContinue
if ($null -eq $Process) { Write-Host "AirDropPlus-Go 未运行（存在残留 PID 文件）。"; exit 1 }
if ([IO.Path]::GetFullPath($Process.Path) -ne $Exe) { throw "PID 文件指向其他程序，已保持现状。" }
$Config = Get-Content -Raw -LiteralPath $ConfigFile | ConvertFrom-Json
if ($Config.listen -notmatch ':(\d+)$') { throw "无法从 listen 配置解析端口。" }
$Health = Invoke-RestMethod -Uri "http://127.0.0.1:$($Matches[1])/healthz" -TimeoutSec 2
[pscustomobject]@{ Running = $Health.success; ProcessId = $ServicePid; Address = $Config.public_url; Version = $Health.data.version }
