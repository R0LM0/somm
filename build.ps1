# build.ps1 - Build somm with version info
# Usage: .\build.ps1 [version]
# Example: .\build.ps1 2.1.9

param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"

# Get commit and date
$Commit = (git rev-parse --short HEAD 2>$null)
if (-not $Commit) { $Commit = "unknown" }

$Date = (Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ")
$Binary = "somm.exe"
$Module = "./cmd/somm"
$Output = "$env:USERPROFILE\go\bin\$Binary"

$LdFlags = "-s -w -X main.version=$Version -X main.commit=$Commit -X main.date=$Date"

Write-Host "Building somm v$Version ($Commit)..." -ForegroundColor Cyan

go build -ldflags $LdFlags -o $Output $Module

if ($LASTEXITCODE -eq 0) {
    Write-Host "✅ Built: $Output" -ForegroundColor Green
    Write-Host ""
    & $Output --version
} else {
    Write-Host "❌ Build failed" -ForegroundColor Red
    exit 1
}
