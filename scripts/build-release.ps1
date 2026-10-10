param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$')]
    [string]$Tag
)

$ErrorActionPreference = 'Stop'
$taskRepo = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$taskOutput = Join-Path $taskRepo "dist/release/$Tag"
New-Item -ItemType Directory -Force -Path $taskOutput | Out-Null
$taskOldGoos = $env:GOOS
$taskOldGoarch = $env:GOARCH
$taskOldCgo = $env:CGO_ENABLED
$taskArchives = @()
Push-Location $taskRepo
try {
    $env:CGO_ENABLED = '0'
    $taskTargets = @(
        @('windows', 'amd64'),
        @('darwin', 'amd64'),
        @('darwin', 'arm64'),
        @('linux', 'amd64'),
        @('linux', 'arm64')
    )
    foreach ($taskTarget in $taskTargets) {
        $env:GOOS = $taskTarget[0]
        $env:GOARCH = $taskTarget[1]
        $taskName = "tiscord-$Tag-$env:GOOS-$env:GOARCH"
        $taskStage = Join-Path $taskOutput ".stage/$taskName"
        New-Item -ItemType Directory -Force -Path $taskStage | Out-Null
        $taskBinary = 'tiscord'
        if ($env:GOOS -eq 'windows') { $taskBinary += '.exe' }
        go build -trimpath -ldflags '-s -w' -o (Join-Path $taskStage $taskBinary) .
        if ($LASTEXITCODE -ne 0) { throw "Build failed: $taskName" }
        Copy-Item -LiteralPath (Join-Path $taskRepo 'README.md') -Destination $taskStage
        Copy-Item -LiteralPath (Join-Path $taskRepo 'config.example.json') -Destination $taskStage
        if ($env:GOOS -eq 'windows') {
            $taskArchive = Join-Path $taskOutput "$taskName.zip"
            $taskFiles = @($taskBinary, 'README.md', 'config.example.json') | ForEach-Object { Join-Path $taskStage $_ }
            Compress-Archive -LiteralPath $taskFiles -DestinationPath $taskArchive -Force
        } else {
            $taskArchive = Join-Path $taskOutput "$taskName.tar.gz"
            tar -czf $taskArchive -C $taskStage $taskBinary README.md config.example.json
            if ($LASTEXITCODE -ne 0) { throw "Packaging failed: $taskName" }
        }
        $taskArchives += $taskArchive
        Write-Output $taskArchive
    }
    $taskChecksums = foreach ($taskArchive in $taskArchives) {
        $taskHash = (Get-FileHash -LiteralPath $taskArchive -Algorithm SHA256).Hash.ToLowerInvariant()
        "$taskHash  $([IO.Path]::GetFileName($taskArchive))"
    }
    [IO.File]::WriteAllText((Join-Path $taskOutput 'SHA256SUMS.txt'), ($taskChecksums -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
} finally {
    $env:GOOS = $taskOldGoos
    $env:GOARCH = $taskOldGoarch
    $env:CGO_ENABLED = $taskOldCgo
    Pop-Location
}
