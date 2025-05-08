<#
.SYNOPSIS
    Builds kubectl-ai for Windows (amd64).
.DESCRIPTION
    This script sets GOOS and GOARCH and runs 'go build'.
.PARAMETER OutputPath
    Output path for the executable (default '.\kubectl-ai.exe').
.EXAMPLE
    .\install.ps1 -OutputPath .\kubectl-ai.exe
#>
param(
    [string]$OutputPath = ".\kubectl-ai.exe"
)

Write-Host "Building kubectl-ai for Windows (GOOS=windows, GOARCH=amd64)..."
$env:GOOS = "windows"
$env:GOARCH = "amd64"

go build -o $OutputPath .
if ($LASTEXITCODE -eq 0) {
    Write-Host "Build succeeded: $OutputPath"
} else {
    Write-Error "Build failed with exit code: $LASTEXITCODE"
    exit 1
}
