[CmdletBinding()]
param([string]$Version = "0.1.0")
$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$Dist = Join-Path $ProjectRoot "dist"
New-Item -ItemType Directory -Force -Path $Dist | Out-Null
Push-Location $ProjectRoot
try {
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "go test failed with exit code $LASTEXITCODE" }
    # Win32 GlobalLock returns a raw pointer-sized value. Only vet's unsafeptr
    # heuristic is disabled; all other analyzers remain enabled.
    go vet -unsafeptr=false ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed with exit code $LASTEXITCODE" }
    go build -trimpath -ldflags "-H=windowsgui -s -w -X main.version=$Version" -o (Join-Path $Dist "AirDropPlus-Go.exe") ./cmd/airdropplus-go
    if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
    $FontDist = Join-Path $Dist "fonts"
    New-Item -ItemType Directory -Force -Path $FontDist | Out-Null
    foreach ($FontFile in @("CharterBT-Roman.otf", "CharterBT-Bold.otf", "Charter-LICENSE.txt")) {
        Copy-Item -LiteralPath (Join-Path $ProjectRoot "assets\fonts\$FontFile") -Destination (Join-Path $FontDist $FontFile) -Force
    }
    Get-FileHash -Algorithm SHA256 (Join-Path $Dist "AirDropPlus-Go.exe")
} finally { Pop-Location }
