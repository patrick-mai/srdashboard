$ErrorActionPreference = "Stop"
$env:GOOS = "linux"
$env:GOARCH = "arm64"
$env:CGO_ENABLED = "0"
go build -o srdashboard-arm64 .
Write-Host "arm64 build OK"
