[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Exe = Join-Path $ProjectRoot "dist\AirDropPlus-Go.exe"
if (-not (Test-Path -LiteralPath $Exe)) { throw "请先运行 scripts\Build.ps1" }
$Exe = [IO.Path]::GetFullPath($Exe)
$DataDir = Join-Path $ProjectRoot "dist\data"
$PidFile = Join-Path $DataDir "airdropplus-go.pid"

if (Test-Path -LiteralPath $PidFile) {
    $ExistingPid = [int](Get-Content -Raw -LiteralPath $PidFile)
    $Existing = Get-Process -Id $ExistingPid -ErrorAction SilentlyContinue
    if ($null -ne $Existing) {
        if ([IO.Path]::GetFullPath($Existing.Path) -ne $Exe) { throw "PID 文件指向其他程序，已保持现状。" }
        Write-Host "AirDropPlus-Go 已在运行，PID: $ExistingPid"
        exit 0
    }
    Remove-Item -LiteralPath $PidFile
}

New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
$StdOut = Join-Path $DataDir "service.stdout.log"
$StdErr = Join-Path $DataDir "service.stderr.log"
$Proc = Start-Process -FilePath $Exe -WorkingDirectory $ProjectRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput $StdOut -RedirectStandardError $StdErr

for ($Attempt = 0; $Attempt -lt 50; $Attempt++) {
    Start-Sleep -Milliseconds 200
    $Proc.Refresh()
    if ($Proc.HasExited) {
        $Details = if (Test-Path -LiteralPath $StdErr) { Get-Content -Raw -LiteralPath $StdErr } else { "无错误日志" }
        throw "AirDropPlus-Go 启动失败（退出码 $($Proc.ExitCode)）：$Details"
    }
    if (-not (Test-Path -LiteralPath $PidFile)) { continue }
    if ([int](Get-Content -Raw -LiteralPath $PidFile) -ne $Proc.Id) { throw "新进程 PID 与 PID 文件不一致，已保持现状。" }
    $Config = Get-Content -Raw -LiteralPath (Join-Path $DataDir "config.json") | ConvertFrom-Json
    if ($Config.listen -notmatch ':(\d+)$') { throw "无法从 listen 配置解析端口。" }
    try {
        $Health = Invoke-RestMethod -Uri "http://127.0.0.1:$($Matches[1])/healthz" -TimeoutSec 2
        if ($Health.success) {
            Write-Host "AirDropPlus-Go 已在后台启动，PID: $($Proc.Id)，地址: $($Config.public_url)"
            exit 0
        }
    } catch {}
}

if (-not $Proc.HasExited) { Stop-Process -Id $Proc.Id }
throw "AirDropPlus-Go 进程已启动，但健康检查未在 10 秒内通过。"
