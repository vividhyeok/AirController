param(
    [string]$SourceExe = (Join-Path $PSScriptRoot '..\dist\AirMouse.exe'),
    [string]$OutputMsi = (Join-Path $PSScriptRoot '..\dist\AirMouse-Setup-x64.msi'),
    [string]$ProductVersion = '1.1.0',
    [string]$WixBin = $env:WIX_BIN
)

$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$sourcePath = (Resolve-Path $SourceExe).Path
$sourceDir = Split-Path -Parent $sourcePath
$wxsPath = Join-Path $repoRoot 'installer\Product.wxs'
$outputPath = [IO.Path]::GetFullPath($OutputMsi)
$outputDir = Split-Path -Parent $outputPath
$intermediateDir = Join-Path $outputDir 'installer-obj'

if ($ProductVersion -notmatch '^\d+\.\d+\.\d+$') {
    throw "ProductVersion must use major.minor.patch format: $ProductVersion"
}

if (-not $WixBin) {
    $candleCommand = Get-Command candle.exe -ErrorAction SilentlyContinue
    $lightCommand = Get-Command light.exe -ErrorAction SilentlyContinue
    $darkCommand = Get-Command dark.exe -ErrorAction SilentlyContinue
    if (-not $candleCommand -or -not $lightCommand -or -not $darkCommand) {
        throw 'WiX 3.14.1 was not found. Set WIX_BIN to the folder containing candle.exe, light.exe, and dark.exe.'
    }
    $candle = $candleCommand.Source
    $light = $lightCommand.Source
    $dark = $darkCommand.Source
} else {
    $resolvedWixBin = (Resolve-Path $WixBin).Path
    $candle = Join-Path $resolvedWixBin 'candle.exe'
    $light = Join-Path $resolvedWixBin 'light.exe'
    $dark = Join-Path $resolvedWixBin 'dark.exe'
}

foreach ($tool in @($candle, $light, $dark)) {
    if (-not (Test-Path -LiteralPath $tool -PathType Leaf)) {
        throw "Required WiX tool was not found: $tool"
    }
}

New-Item -ItemType Directory -Force -Path $outputDir, $intermediateDir | Out-Null
$wixObject = Join-Path $intermediateDir 'Product.wixobj'

& $candle -nologo -arch x64 "-dSourceDir=$sourceDir" "-dProductVersion=$ProductVersion" -out $wixObject $wxsPath
if ($LASTEXITCODE -ne 0) {
    throw "candle.exe failed with exit code $LASTEXITCODE"
}

# Some locked-down Windows environments do not expose the Windows Installer
# validation service to non-elevated build processes. WiX still creates the
# database deterministically; CI validates it by decompiling the finished MSI.
& $light -nologo -sval -out $outputPath $wixObject
if ($LASTEXITCODE -ne 0) {
    throw "light.exe failed with exit code $LASTEXITCODE"
}

$decompiledWxs = Join-Path $intermediateDir 'Decompiled.wxs'
$decompiledFiles = Join-Path $intermediateDir 'decompiled-files'
& $dark -nologo -x $decompiledFiles -o $decompiledWxs $outputPath
if ($LASTEXITCODE -ne 0) {
    throw "dark.exe could not read the generated MSI (exit code $LASTEXITCODE)"
}
$decompiledContent = Get-Content -Raw -LiteralPath $decompiledWxs
if ($decompiledContent -notmatch 'Name="Air Mouse"' -or $decompiledContent -notmatch 'AirMouse\.exe') {
    throw 'The generated MSI did not contain the expected product metadata or executable.'
}

Get-Item -LiteralPath $outputPath | Select-Object FullName, Length, LastWriteTime
