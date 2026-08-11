#Requires -RunAsAdministrator
[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$RuleName = "AirDropPlus-Go Private LAN"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Exe = [IO.Path]::GetFullPath((Join-Path $ProjectRoot "dist\AirDropPlus-Go.exe"))
$ConfigFile = Join-Path $ProjectRoot "dist\data\config.json"
if (-not (Test-Path -LiteralPath $Exe)) { throw "请先运行 scripts\Build.ps1" }
$Port = 53317
if (Test-Path -LiteralPath $ConfigFile) {
    $Config = Get-Content -Raw -LiteralPath $ConfigFile | ConvertFrom-Json
    if ($Config.listen -notmatch ':(\d+)$') { throw "无法从 listen 配置解析端口。" }
    $Port = [int]$Matches[1]
}
$Existing = Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue
if ($null -ne $Existing) {
    if (@($Existing).Count -ne 1) { throw "发现多个同名防火墙规则，已保持现状。" }
    $App = $Existing | Get-NetFirewallApplicationFilter
    $PortFilter = $Existing | Get-NetFirewallPortFilter
    if ([IO.Path]::GetFullPath($App.Program) -ne $Exe -or [string]$PortFilter.Protocol -notin @("TCP", "6") -or [int]$PortFilter.LocalPort -ne $Port -or [string]$Existing.Direction -ne "Inbound" -or [string]$Existing.Action -ne "Allow" -or [string]$Existing.Profile -ne "Private") {
        throw "同名防火墙规则的目标不匹配，已保持现状。"
    }
    Enable-NetFirewallRule -DisplayName $RuleName | Out-Null
    Write-Host "专用网络防火墙规则已存在并启用。"
    exit 0
}
New-NetFirewallRule -DisplayName $RuleName -Direction Inbound -Action Allow -Profile Private -Protocol TCP -LocalPort $Port -Program $Exe | Out-Null
Write-Host "已添加仅限专用网络的 TCP $Port 入站规则。"
