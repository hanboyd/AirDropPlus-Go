#Requires -RunAsAdministrator
[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
$RuleName = "AirDropPlus-Go Private LAN"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Exe = [IO.Path]::GetFullPath((Join-Path $ProjectRoot "dist\AirDropPlus-Go.exe"))
$Existing = Get-NetFirewallRule -DisplayName $RuleName -ErrorAction SilentlyContinue
if ($null -eq $Existing) { Write-Host "未发现 AirDropPlus-Go 防火墙规则。"; exit 0 }
if (@($Existing).Count -ne 1) { throw "发现多个同名防火墙规则，已保持现状。" }
$App = $Existing | Get-NetFirewallApplicationFilter
$PortFilter = $Existing | Get-NetFirewallPortFilter
if ([IO.Path]::GetFullPath($App.Program) -ne $Exe -or [string]$PortFilter.Protocol -notin @("TCP", "6") -or [string]$Existing.Direction -ne "Inbound" -or [string]$Existing.Action -ne "Allow" -or [string]$Existing.Profile -ne "Private") { throw "同名规则的目标或范围不匹配，已保持现状。" }
Remove-NetFirewallRule -DisplayName $RuleName
Write-Host "已删除 AirDropPlus-Go 专用网络防火墙规则。"
