param(
    [string]$ProductVersion = '1.1.0',
    [string]$WixBin = $env:WIX_BIN
)

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$distDir = Join-Path $repoRoot 'dist'
$exePath = Join-Path $distDir 'AirMouse.exe'
$zipPath = Join-Path $distDir 'AirMouse-windows-amd64.zip'
$msiPath = Join-Path $distDir 'AirMouse-Setup-x64.msi'

New-Item -ItemType Directory -Force -Path $distDir | Out-Null

Push-Location $repoRoot
try {
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw "go test failed with exit code $LASTEXITCODE" }

    go build -ldflags '-s -w' -o $exePath .
    if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }

    Compress-Archive -LiteralPath $exePath -DestinationPath $zipPath -Force
    & (Join-Path $PSScriptRoot 'build-msi.ps1') `
        -SourceExe $exePath `
        -OutputMsi $msiPath `
        -ProductVersion $ProductVersion `
        -WixBin $WixBin
} finally {
    Pop-Location
}

Get-Item -LiteralPath $exePath, $zipPath, $msiPath | Select-Object Name, FullName, Length, LastWriteTime
