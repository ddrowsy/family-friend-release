param(
    [string]$OutputDirectory = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$installerDirectory = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $installerDirectory "..\..")).Path
$isWindows = [System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT

if (-not $isWindows) {
    throw "WiX MSI builds require Windows-native Windows Installer components. Run this script on Windows, not Linux or WSL."
}

foreach ($tool in @("go", "dotnet")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "Required tool '$tool' was not found on PATH."
    }
}

if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $repoRoot "dist\windows"
} elseif (-not [System.IO.Path]::IsPathRooted($OutputDirectory)) {
    $OutputDirectory = Join-Path $repoRoot $OutputDirectory
}

$OutputDirectory = [System.IO.Path]::GetFullPath($OutputDirectory)
$msiPath = Join-Path $OutputDirectory "FamilyFriend-x64.msi"
$stageDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("familyfriend-msi-" + [System.Guid]::NewGuid().ToString("N"))
$decompiledPath = Join-Path $stageDirectory "FamilyFriend-decompiled.wxs"
$expectedFiles = @(
    "drowsyfriend-session-worker.exe",
    "drowsyfriend-ui.exe",
    "drowsyfriend.exe"
) | Sort-Object

$previousLocation = Get-Location
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$previousCGOEnabled = $env:CGO_ENABLED

function Assert-LastCommandSucceeded {
    param(
        [string]$Description
    )

    if ($LASTEXITCODE -ne 0) {
        throw "$Description failed with exit code $LASTEXITCODE."
    }
}

try {
    New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
    New-Item -ItemType Directory -Force -Path $stageDirectory | Out-Null
    Remove-Item -Force -ErrorAction SilentlyContinue $msiPath

    Set-Location $repoRoot
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    $env:CGO_ENABLED = "1"

    & go build -buildvcs=false -o (Join-Path $stageDirectory "drowsyfriend.exe") ./cmd/drowsyfriend
    Assert-LastCommandSucceeded "building drowsyfriend.exe"

    & go build `
        -buildvcs=false `
        -ldflags "-H=windowsgui" `
        -o (Join-Path $stageDirectory "drowsyfriend-ui.exe") `
        ./cmd/drowsyfriend-ui
    Assert-LastCommandSucceeded "building drowsyfriend-ui.exe"

    & go build `
        -buildvcs=false `
        -ldflags "-H=windowsgui" `
        -o (Join-Path $stageDirectory "drowsyfriend-session-worker.exe") `
        ./cmd/drowsyfriend-session-worker
    Assert-LastCommandSucceeded "building drowsyfriend-session-worker.exe"

    $actualFiles = @(Get-ChildItem -File $stageDirectory | Select-Object -ExpandProperty Name | Sort-Object)
    $unexpectedFiles = @(Compare-Object -ReferenceObject $expectedFiles -DifferenceObject $actualFiles)
    if ($unexpectedFiles.Count -ne 0) {
        throw "Installer staging must contain exactly: $($expectedFiles -join ', ')."
    }

    Push-Location $installerDirectory
    try {
        & dotnet tool restore
        Assert-LastCommandSucceeded "restoring WiX 6.0.2"

        & dotnet tool run wix -- build `
            -arch x64 `
            -d "StageDir=$stageDirectory" `
            -pdbtype none `
            -o $msiPath `
            Package.wxs
        Assert-LastCommandSucceeded "building FamilyFriend-x64.msi"

        & dotnet tool run wix -- msi validate $msiPath
        Assert-LastCommandSucceeded "validating FamilyFriend-x64.msi"

        & dotnet tool run wix -- msi decompile -o $decompiledPath $msiPath
        Assert-LastCommandSucceeded "decompiling FamilyFriend-x64.msi for content verification"
    } finally {
        Pop-Location
    }

    $decompiledSource = Get-Content -Raw $decompiledPath
    $fileElementCount = [System.Text.RegularExpressions.Regex]::Matches($decompiledSource, "<File\b").Count
    if ($fileElementCount -ne $expectedFiles.Count) {
        throw "MSI contains $fileElementCount File entries; expected $($expectedFiles.Count)."
    }

    foreach ($file in $expectedFiles) {
        if (-not $decompiledSource.Contains($file)) {
            throw "MSI content verification did not find $file."
        }
    }

    if ($decompiledSource.Contains("drowsyfriendctl.exe")) {
        throw "MSI must not contain drowsyfriendctl.exe."
    }

    Write-Host "PASS: $msiPath"
} finally {
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    $env:CGO_ENABLED = $previousCGOEnabled
    Set-Location $previousLocation
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $stageDirectory
}
