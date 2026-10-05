<#
.SYNOPSIS
    Verification-first install and update script for git-governance.

.DESCRIPTION
    Install and update are one idempotent mechanism: re-running this script
    installs the pinned version, or the latest release when no pin is given.
    The update is the install re-run through the same channel; there is no
    separate update path and no automatic self-update.

    The script implements the verification-first standard of
    docs/operations/installation-and-release.md (section 9.1) and
    docs/architecture/ADR-0009-CHANNEL-OWNED-DELIVERY-AND-UPDATE-MODEL.md:

      1. Resolve the version from an explicit pin or a validated latest
         resolution; an unvalidated version is never installed.
      2. Download the release artifact, its SHA-256 checksum manifest, and its
         signature bundle, and verify fail-closed before any mutation. The
         release checksum manifest is the single verification truth; this
         script introduces no second checksum scheme.
      3. The signature bundle is verified with cosign when cosign is installed.
         When cosign is not installed the script reports this explicitly instead
         of assuming an additional consumer dependency; the checksum
         verification stays mandatory in every case.
      4. Write the binary to a temporary path, run an executable smoke test,
         replace the target, and preserve the previous version as
         git-governance.exe.previous for a controlled rollback.
      5. Report whether the current terminal already knows the install
         directory and recommend a new terminal when needed. PowerShell
         profiles are never edited by this script.

    The script runs on Windows PowerShell 5.1 and PowerShell 7+ and uses only
    native cmdlets (Invoke-RestMethod, Invoke-WebRequest, Expand-Archive,
    Get-FileHash); it declares no additional consumer dependency. Write-Host is
    used deliberately as this script's console UX surface; the script returns
    no data objects.

    Environment overrides:

      GIT_GOVERNANCE_VERSION      pin (e.g. 1.2.3 or v1.2.3); latest when unset
      GIT_GOVERNANCE_INSTALL_DIR  target directory
                                  (default %LOCALAPPDATA%\Programs\git-governance)

    Manual zero-dependency path: download the artifact, the checksum manifest,
    and the signature bundle from
    https://github.com/t33n-software/git-governance/releases and follow section
    5.2 of docs/operations/installation-and-release.md.

.EXAMPLE
    irm https://raw.githubusercontent.com/t33n-software/git-governance/main/install.ps1 | iex

.EXAMPLE
    $env:GIT_GOVERNANCE_VERSION = '1.2.3'; .\install.ps1
#>
[CmdletBinding()]
param(
    [string]$Version = $env:GIT_GOVERNANCE_VERSION,
    [string]$InstallDir = $env:GIT_GOVERNANCE_INSTALL_DIR
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

if ($PSVersionTable.PSVersion.Major -lt 6) {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
}

$RepoOwner = 't33n-software'
$RepoName = 'git-governance'
$RepoSlug = "${RepoOwner}/${RepoName}"
$ReleasesUrl = "https://github.com/${RepoSlug}/releases"
$DownloadUrl = "${ReleasesUrl}/download"
$ApiUrl = "https://api.github.com/repos/${RepoSlug}/releases/latest"
$ArtifactBase = 'git-governance'
$BinaryName = 'git-governance.exe'

function Write-Step {
    param([string]$Message)
    Write-Host $Message
}

function Write-WarnLine {
    param([string]$Message)
    Write-Host "warning: $Message"
}

function Stop-Install {
    param([string]$Message)
    throw $Message
}

function Assert-SemVer {
    param([string]$Candidate)
    if ($Candidate -notmatch '^[0-9]+\.[0-9]+\.[0-9]+$') {
        Stop-Install "invalid version '${Candidate}'; expected SemVer such as 1.2.3"
    }
}

# --- temporary workspace ------------------------------------------------------
$WorkDir = Join-Path ([System.IO.Path]::GetTempPath()) ('git-governance-install-' + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $WorkDir -Force | Out-Null

try {
    # --- architecture detection ------------------------------------------------
    switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { $Arch = 'amd64' }
        'ARM64' { $Arch = 'arm64' }
        default {
            Stop-Install "unsupported architecture '$($env:PROCESSOR_ARCHITECTURE)'; use the manual download path: $ReleasesUrl"
        }
    }

    # --- version resolution ------------------------------------------------------
    $Pinned = ''
    if ($Version) { $Pinned = $Version.Trim() }
    if ($Pinned) {
        if ($Pinned.StartsWith('v')) { $Pinned = $Pinned.Substring(1) }
        Assert-SemVer $Pinned
        $ResolvedVersion = $Pinned
        Write-Step "Installing pinned release: v$ResolvedVersion"
    }
    else {
        Write-Step 'Resolving the latest release ...'
        $Release = Invoke-RestMethod -Uri $ApiUrl
        $Tag = [string]$Release.tag_name
        if (-not $Tag) {
            Stop-Install "could not resolve the latest release tag; pin a version with GIT_GOVERNANCE_VERSION or use the manual path: $ReleasesUrl"
        }
        $ResolvedVersion = $Tag.Trim()
        if ($ResolvedVersion.StartsWith('v')) { $ResolvedVersion = $ResolvedVersion.Substring(1) }
        Assert-SemVer $ResolvedVersion
        Write-Step "Resolved latest release: $Tag"
    }

    # --- artifacts (single verification truth: the release checksum manifest) ----
    $ArchiveName = "${ArtifactBase}_${ResolvedVersion}_windows_${Arch}.zip"
    $ChecksumsName = "${ArtifactBase}_${ResolvedVersion}_checksums.txt"
    $SignatureName = "${ChecksumsName}.sigstore.json"

    Write-Step "Downloading $ArchiveName, $ChecksumsName, and the signature bundle ..."
    foreach ($asset in @($ArchiveName, $ChecksumsName, $SignatureName)) {
        $destination = Join-Path $WorkDir $asset
        Invoke-WebRequest -Uri "${DownloadUrl}/v${ResolvedVersion}/${asset}" -OutFile $destination -UseBasicParsing
    }

    # --- checksum verification (mandatory, native, fail-closed) ------------------
    $escapedArchive = [regex]::Escape($ArchiveName)
    $manifestLine = Select-String -LiteralPath (Join-Path $WorkDir $ChecksumsName) -Pattern ("^[0-9a-fA-F]{64}\s+" + $escapedArchive + "\s*$") | Select-Object -First 1
    if (-not $manifestLine) {
        Stop-Install "checksum manifest does not contain an entry for $ArchiveName; refusing to continue"
    }
    $ExpectedHash = ($manifestLine.Line -split '\s+')[0].ToLowerInvariant()
    if ($ExpectedHash -notmatch '^[0-9a-f]{64}$') {
        Stop-Install "checksum manifest entry for $ArchiveName is malformed; refusing to continue"
    }
    $ActualHash = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $WorkDir $ArchiveName)).Hash.ToLowerInvariant()
    if ($ActualHash -ne $ExpectedHash) {
        Stop-Install "checksum mismatch for ${ArchiveName}: expected $ExpectedHash, got $ActualHash; refusing to install"
    }
    Write-Step "Checksum verified: $ArchiveName"

    # --- signature verification (cosign when present, explicit when absent) ------
    $Cosign = Get-Command cosign -ErrorAction SilentlyContinue
    if ($Cosign) {
        Write-Step 'Verifying the signature bundle with cosign ...'
        & cosign verify-blob --bundle (Join-Path $WorkDir $SignatureName) --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' --certificate-identity-regexp ("^https://github.com/" + $RepoSlug + "/") (Join-Path $WorkDir $ChecksumsName)
        if ($LASTEXITCODE -ne 0) {
            Stop-Install "signature verification failed for $ChecksumsName; refusing to install"
        }
        Write-Step "Signature verified: $ChecksumsName"
    }
    else {
        Write-WarnLine "cosign is not installed; the signature bundle was downloaded but not verified locally. The SHA-256 checksum manifest (the release verification truth) was verified natively. To verify the signature, install cosign and run:"
        Write-WarnLine "cosign verify-blob --bundle $SignatureName --certificate-oidc-issuer https://token.actions.githubusercontent.com --certificate-identity-regexp '^https://github.com/$RepoSlug/' $ChecksumsName"
    }

    # --- extraction and smoke test ------------------------------------------------
    Write-Step "Extracting $ArchiveName ..."
    Expand-Archive -LiteralPath (Join-Path $WorkDir $ArchiveName) -DestinationPath $WorkDir -Force
    $StagedBinary = Join-Path $WorkDir $BinaryName
    if (-not (Test-Path -LiteralPath $StagedBinary -PathType Leaf)) {
        Stop-Install "the archive does not contain $BinaryName; refusing to install"
    }

    Write-Step 'Running the executable smoke test ...'
    $Smoke = (& $StagedBinary --version) -join ' '
    if (-not $Smoke) {
        Stop-Install 'the downloaded binary returned an empty --version; refusing to install'
    }
    Write-Step "Smoke test passed: $Smoke"

    # --- atomic installation with rollback copy -----------------------------------
    if (-not $InstallDir) {
        $InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\git-governance'
    }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $Target = Join-Path $InstallDir $BinaryName

    if (Test-Path -LiteralPath $Target -PathType Leaf) {
        Copy-Item -LiteralPath $Target -Destination "${Target}.previous" -Force
        Write-Step "Previous version preserved: ${Target}.previous"
    }

    Move-Item -LiteralPath $StagedBinary -Destination $Target -Force
    Write-Step "Installed: $Target"

    $InstalledVersion = (& $Target --version) -join ' '
    if (-not $InstalledVersion) {
        Stop-Install "the installed binary failed --version verification; restore the previous version from ${Target}.previous if present"
    }
    Write-Step "Verification: $InstalledVersion"

    try {
        & $Target doctor *> $null
        if ($LASTEXITCODE -eq 0) {
            Write-Step 'Diagnostics: doctor completed successfully'
        }
        else {
            Write-WarnLine "doctor reported issues; its diagnostics include repository checks that depend on the current working directory. Run 'git-governance doctor' inside your project to review them."
        }
    }
    catch {
        Write-WarnLine 'doctor could not be executed; review it manually with: git-governance doctor'
    }

    # --- PATH and session reporting (no profile edits, ever) ----------------------
    $PathEntries = $env:Path -split ';'
    if ($PathEntries -contains $InstallDir) {
        Write-Step "PATH: $InstallDir is already known to this terminal"
    }
    else {
        Write-Step "PATH: $InstallDir is NOT on the PATH of the current terminal."
        Write-Host "  Permanently (explicit user action; this script never edits profiles):"
        Write-Host "    [Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path', 'User') + ';$InstallDir', 'User')"
        Write-Host "  For this session: ``$env:Path = ``$env:Path + ';$InstallDir'"
        Write-Host "  Then open a new terminal so every new process finds git-governance."
    }

    Write-Host ''
    Write-Host "Installation complete: $ArtifactBase $ResolvedVersion (windows/$Arch)"
    Write-Host "  Binary:   $Target"
    Write-Host '  Update:   re-run this script (the update is the install re-run)'
    Write-Host "  Manual:   $ReleasesUrl"
}
finally {
    if (Test-Path -LiteralPath $WorkDir) {
        Remove-Item -LiteralPath $WorkDir -Recurse -Force
    }
}