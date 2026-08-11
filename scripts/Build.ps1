[CmdletBinding()]
param([string]$Version = "0.1.0")
$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Dist = Join-Path $ProjectRoot "dist"
New-Item -ItemType Directory -Force -Path $Dist | Out-Null
Push-Location $ProjectRoot
try {
    go test ./...
    # Win32 GlobalLock returns a raw pointer-sized value. Only vet's unsafeptr
    # heuristic is disabled; all other analyzers remain enabled.
    go vet -unsafeptr=false ./...
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o (Join-Path $Dist "AirDropPlus-Go.exe") ./cmd/airdropplus-go
    Get-FileHash -Algorithm SHA256 (Join-Path $Dist "AirDropPlus-Go.exe")
} finally { Pop-Location }
