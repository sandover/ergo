[CmdletBinding()]
param(
    [switch] $Install,
    [switch] $Upgrade,
    [switch] $Uninstall,
    [string] $ExpectedVersion
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$packageId = 'Sandover.Ergo'
$noApplicationsFound = -1978335212 # APPINSTALLER_CLI_ERROR_NO_APPLICATIONS_FOUND (0x8A150014)
$operationCount = @(@($Install, $Upgrade, $Uninstall) | Where-Object { $_.IsPresent }).Count
if ($operationCount -gt 1) {
    throw 'Choose at most one operation: -Install, -Upgrade, or -Uninstall.'
}
if (($Install -or $Upgrade) -and [string]::IsNullOrWhiteSpace($ExpectedVersion)) {
    throw '-Install and -Upgrade require -ExpectedVersion so a no-op cannot look successful.'
}

if (-not (Get-Command winget -ErrorAction SilentlyContinue)) {
    [Console]::Error.WriteLine('WinGet (winget.exe) is not available on PATH.')
    exit 127
}

function Invoke-WinGet {
    param(
        [Parameter(Mandatory)]
        [string[]] $Arguments,

        [switch] $AllowAbsence
    )

    Write-Host "`n> winget $($Arguments -join ' ')"
    $output = @(& winget @Arguments 2>&1 | ForEach-Object { "$_" })
    $commandExitCode = $LASTEXITCODE
    $output | ForEach-Object { Write-Host $_ }

    if ($AllowAbsence -and $commandExitCode -eq $noApplicationsFound) {
        return $false
    }

    if ($commandExitCode -eq 0) {
        return $true
    }

    [Console]::Error.WriteLine("WinGet exited with code $commandExitCode.")
    exit $commandExitCode
}

function Get-InstalledVersion {
    Write-Host "`n> winget list --id $packageId --exact --source winget"
    $output = @(& winget list --id $packageId --exact --source winget 2>&1 | ForEach-Object { "$_" })
    $commandExitCode = $LASTEXITCODE
    $output | ForEach-Object { Write-Host $_ }

    if ($commandExitCode -eq $noApplicationsFound) {
        return $null
    }
    if ($commandExitCode -ne 0) {
        [Console]::Error.WriteLine("WinGet exited with code $commandExitCode.")
        exit $commandExitCode
    }

    $packagePattern = [regex]::Escape($packageId) + '\s+(\S+)'
    $packageRow = $output | Where-Object { $_ -match $packagePattern } | Select-Object -First 1
    if (-not $packageRow) {
        [Console]::Error.WriteLine("WinGet listed $packageId but its installed version could not be read.")
        exit 1
    }
    return ([regex]::Match($packageRow, $packagePattern).Groups[1].Value)
}

$available = Invoke-WinGet -Arguments @('show', '--id', $packageId, '--exact', '--source', 'winget') -AllowAbsence
if (-not $available) {
    Write-Host "`n$packageId is not available in the configured WinGet sources yet. No changes were made."
    exit 2
}

$installedBefore = Get-InstalledVersion

if ($Install) {
    if ($installedBefore) {
        throw "$packageId $installedBefore is already installed; use -Upgrade to verify an update."
    }
    [void](Invoke-WinGet -Arguments @('install', '--id', $packageId, '--exact', '--source', 'winget', '--accept-source-agreements', '--accept-package-agreements'))
}
elseif ($Upgrade) {
    if (-not $installedBefore) {
        throw "$packageId is not installed; use -Install first."
    }
    if ($installedBefore -eq $ExpectedVersion) {
        throw "$packageId $ExpectedVersion is already installed; an upgrade would be a no-op."
    }
    [void](Invoke-WinGet -Arguments @('upgrade', '--id', $packageId, '--exact', '--source', 'winget', '--accept-source-agreements', '--accept-package-agreements'))
}
elseif ($Uninstall) {
    if (-not $installedBefore) {
        throw "$packageId is not installed through WinGet."
    }
    [void](Invoke-WinGet -Arguments @('uninstall', '--id', $packageId, '--exact', '--source', 'winget'))
    $installedAfter = Get-InstalledVersion
    if ($installedAfter) {
        [Console]::Error.WriteLine("$packageId $installedAfter remains installed after uninstall.")
        exit 1
    }
    if (Get-Command ergo -ErrorAction SilentlyContinue) {
        Write-Host "`nAn ergo command remains on PATH; check whether it belongs to another installation."
    }
    else {
        Write-Host "`nUninstall check passed: no ergo command remains on PATH."
    }
    exit 0
}

if ($Install -or $Upgrade) {
    $installedAfter = Get-InstalledVersion
    if ($installedAfter -ne $ExpectedVersion) {
        [Console]::Error.WriteLine("Installed version is '$installedAfter'; expected '$ExpectedVersion'.")
        exit 1
    }
    if (Get-Command ergo -ErrorAction SilentlyContinue) {
        Write-Host "`n> ergo version"
        $versionOutput = @(& ergo version 2>&1 | ForEach-Object { "$_" })
        $versionExitCode = $LASTEXITCODE
        $versionOutput | ForEach-Object { Write-Host $_ }
        if ($versionExitCode -ne 0) {
            [Console]::Error.WriteLine("ergo version exited with code $versionExitCode.")
            exit $versionExitCode
        }
        if (($versionOutput -join "`n") -notmatch [regex]::Escape($ExpectedVersion)) {
            [Console]::Error.WriteLine("ergo version did not report expected version $ExpectedVersion.")
            exit 1
        }
    }
    else {
        Write-Host "`nergo is not visible to this PowerShell process yet. Open a new shell and run 'ergo version' to confirm PATH."
    }
}
else {
    if (-not $installedBefore) {
        Write-Host "`n$packageId is not installed through WinGet."
    }
    if (Get-Command ergo -ErrorAction SilentlyContinue) {
        Write-Host "`n> ergo version"
        & ergo version
        $versionExitCode = $LASTEXITCODE
        if ($versionExitCode -ne 0) {
            [Console]::Error.WriteLine("ergo version exited with code $versionExitCode.")
            exit $versionExitCode
        }
    }
    else {
        Write-Host "`nergo is not currently on PATH."
    }
}
