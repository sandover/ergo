[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false

$repoRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$extensionRoot = Join-Path $repoRoot 'editors/vscode'
$extensionModules = Join-Path $extensionRoot 'node_modules'
$validationBinary = Join-Path ([System.IO.Path]::GetTempPath()) (
    'ergo-windows-validation-' + [guid]::NewGuid().ToString('N') + '.exe'
)
$script:failureExitCode = 0
$locationDepth = 0

function Invoke-CheckedCommand {
    param(
        [Parameter(Mandatory)]
        [string] $DisplayCommand,

        [Parameter(Mandatory)]
        [string] $Executable,

        [string[]] $Arguments = @()
    )

    Write-Host "`n> $DisplayCommand"

    try {
        & $Executable @Arguments
    }
    catch {
        $script:failureExitCode = 1
        throw "Could not run '$DisplayCommand': $($_.Exception.Message)"
    }

    $commandExitCode = $LASTEXITCODE
    if ($commandExitCode -ne 0) {
        $script:failureExitCode = $commandExitCode
        throw "Command '$DisplayCommand' exited with code $commandExitCode."
    }
}

$exitCode = 0
try {
    Push-Location -LiteralPath $repoRoot
    $locationDepth++

    Invoke-CheckedCommand `
        -DisplayCommand 'git diff --check HEAD' `
        -Executable 'git' `
        -Arguments @('diff', '--check', 'HEAD')

    Invoke-CheckedCommand `
        -DisplayCommand 'go test -vet=off -v ./...' `
        -Executable 'go' `
        -Arguments @('test', '-vet=off', '-v', './...')

    Invoke-CheckedCommand `
        -DisplayCommand ('go build -o "{0}" ./cmd/ergo' -f $validationBinary) `
        -Executable 'go' `
        -Arguments @('build', '-o', $validationBinary, './cmd/ergo')

    Invoke-CheckedCommand `
        -DisplayCommand ('"{0}" --version' -f $validationBinary) `
        -Executable $validationBinary `
        -Arguments @('--version')

    Push-Location -LiteralPath $extensionRoot
    $locationDepth++

    if (-not (Test-Path -LiteralPath $extensionModules -PathType Container)) {
        Invoke-CheckedCommand `
            -DisplayCommand 'npm ci (editors/vscode)' `
            -Executable 'npm' `
            -Arguments @('ci')
    }

    Invoke-CheckedCommand `
        -DisplayCommand 'npm test (editors/vscode)' `
        -Executable 'npm' `
        -Arguments @('test')

    Write-Host "`nAll Windows contributor checks passed."
}
catch {
    if ($script:failureExitCode -ne 0) {
        $exitCode = $script:failureExitCode
    }
    else {
        $exitCode = 1
    }
    [Console]::Error.WriteLine($_.Exception.Message)
}
finally {
    while ($locationDepth -gt 0) {
        Pop-Location
        $locationDepth--
    }

    if (Test-Path -LiteralPath $validationBinary -PathType Leaf) {
        try {
            Remove-Item -LiteralPath $validationBinary -Force
        }
        catch {
            Write-Warning "Could not remove temporary binary '$validationBinary': $($_.Exception.Message)"
        }
    }
}

exit $exitCode
