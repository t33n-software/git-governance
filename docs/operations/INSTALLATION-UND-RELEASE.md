# Installation und Release

## 1. Entscheidung

`git-governance` wird als vorgebaute, signierte native Binary verteilt. Endgeräte benötigen Git, aber weder Go noch Node.js, Python, PowerShell 7 oder eine zusätzliche Sprachruntime.

Primäre Installationswege:

| Plattform | Primär | Sekundär | Kontrollierter Fallback |
|---|---|---|---|
| Windows | WinGet | Scoop | signiertes ZIP/MSI aus dem Release |
| macOS | Homebrew Tap | signiertes PKG, sobald erforderlich | signiertes tar.gz |
| Linux | deb/rpm/apk oder organisationsinterner Paketkanal | Homebrew/Nix nach Bedarf | signiertes tar.gz |

Ein Paketmanager ist der Best-Practice-Installationsweg, weil er Installationsort, `PATH`, Upgrade und Uninstall besitzt. Das Projekt editiert nicht ungefragt `.bashrc`, `.zshrc`, Fish-Konfiguration oder PowerShell-Profile.

## 2. Abgrenzung zu NVM

NVM verwaltet eine Laufzeit und mehrere Node-Versionen. Dafür muss es Shell-Initialisierung konfigurieren. `git-governance` ist dagegen eine einzelne native Anwendung. Ein eigener Runtime-/Versionsmanager wäre zusätzliche Komplexität ohne fachlichen Nutzen.

Passendere Vergleichsmodelle sind native CLIs, die über WinGet, Homebrew, Linux-Pakete oder signierte Release-Archive verteilt werden.

## 3. Unterstützte Zielmatrix

Verbindliche erste Release-Matrix:

| OS | Architekturen | Artefakt |
|---|---|---|
| Windows 10/11 | `amd64`, `arm64` | `.zip`, optional MSI |
| macOS, aktuell unterstützte Versionen | `amd64`, `arm64` | `.tar.gz`, optional PKG |
| Linux | `amd64`, `arm64` | `.tar.gz`, deb, rpm, optional apk |

Zusätzliche Betriebssysteme sind keine implizite Zusage. Sie benötigen Zielplattformtests und ein dokumentiertes Support-Gate.

Build-Eigenschaften:

- Go 1.26 als Sprachversion
- gepinnte Toolchain Go 1.26.6
- `go 1.26` plus `toolchain go1.26.6` im Modulvertrag
- `CGO_ENABLED=0`, solange kein belegter Adapter cgo benötigt
- keine Build- oder Compilerinstallation auf Endgeräten
- reproduzierbare Versionsmetadaten aus dem unveränderlichen Release-Commit

Cross-Compilation allein reicht nicht. Jede verbindliche OS-/Architekturklasse benötigt mindestens einen nativen Smoke Test; die drei OS-Familien benötigen vollständige Integrationspfade.

## 4. Paketnamen

- Executable: `git-governance` beziehungsweise `git-governance.exe`
- WinGet Package ID: bei Publisher-Festlegung `<Publisher>.GitGovernance`
- Homebrew Formula: `git-governance`
- Debian/RPM/APK: `git-governance`
- Scoop Manifest: `git-governance`

Der Name `git-flow` wird vermieden, weil er mit bestehenden Gitflow-Werkzeugen kollidiert. `mk` ist zu generisch; `git-tools` beschreibt keine stabile Produktfähigkeit.

## 5. `PATH`-Verhalten

### 5.1 Paketmanager

Der jeweilige Paketmanager besitzt Installation und `PATH`-Integration. Das Projekt hängt keine Zeilen an Benutzerprofile an.

### 5.2 Direkte Archive

Direkte Archive sind ein Fallback:

- Unix: Binary in ein bereits im `PATH` befindliches Verzeichnis installieren, bevorzugt `~/.local/bin` für eine User-Installation oder einen administrativ verwalteten Systempfad.
- Windows: Binary in ein dediziertes Benutzerprogrammverzeichnis entpacken. Eine Änderung des User-`PATH` erfolgt nur durch einen signierten Installer oder nach expliziter Benutzeraktion.

Falls der Zielpfad noch nicht im `PATH` liegt, zeigt die Dokumentation den notwendigen manuellen Schritt. Ein allgemeines Installationsskript darf nicht mehrere Shellprofile erraten und verändern.

### 5.3 Neue Terminals

Eine dauerhafte `PATH`-Änderung wird von neuen Prozessen übernommen. Der Installer muss:

- klar melden, ob das aktuelle Terminal den neuen Pfad bereits kennt
- bei Bedarf zum Start eines neuen Terminals auffordern
- mit `git governance --version` und `git governance doctor` verifizieren

## 6. Benutzerkonfiguration

Die Binary ermittelt den Konfigurationsroot mit Go `os.UserConfigDir()` und legt darunter `git-governance/config.json` an:

| Plattform | Standard |
|---|---|
| Linux | `$XDG_CONFIG_HOME/git-governance`, sonst `$HOME/.config/git-governance` |
| macOS | `$HOME/Library/Application Support/git-governance` |
| Windows | `%AppData%\git-governance` |

Der Pfad ist über `--config` explizit überschreibbar. Ein relatives `XDG_CONFIG_HOME` wird abgewiesen, entsprechend dem Go-Vertrag.

Konfiguration wird:

- versioniert
- typisiert validiert
- mit einer absturzsicheren Write-/Recovery-Strategie ersetzt; die genaue
  Atomizitätsgarantie folgt der jeweiligen Plattform
- mit restriktiven Benutzerrechten angelegt
- nie als Secret Store verwendet

### 6.1 GitHub App credentials

The configuration file never stores GitHub tokens, refresh tokens, App private
keys, client secrets, broker credentials, or authorization headers. A local
user supplies only the public GitHub App client ID through
`GIT_GOVERNANCE_GITHUB_APP_CLIENT_ID`, then completes `auth login github` in a
real terminal. The refresh session is protected by DPAPI on Windows, Keychain
on macOS, or Secret Service on Linux; no plaintext fallback is permitted.

Managed CI does not reuse a developer refresh session. It supplies a
workload-identity token and a HTTPS credential-broker endpoint at deployment
time. The broker holds the GitHub App private key outside the repository and
mints only short-lived, repository-bound installation tokens. See
[GitHub App authentication](../usage/authentication.md) for the precise
runtime contract.

### 6.2 Repository-Quality-Gates

Projekt- und programmiersprachenabhängige Build-, Test- und Lint-Kommandos
gehören nicht in die Binary und nicht in die Benutzerkonfiguration. Ein
Repository deklariert sie optional in `git-governance.quality.json` oder über
`--quality-config`.

Die Datei enthält nur einen Namen, ein Executable, ein Argumentarray, eine
repository-relative Working Directory und einen Timeout je Gate. Shell-Strings
und Pfade außerhalb des Repository-Roots sind unzulässig. Ohne Datei bleibt
das Ergebnis explizit `unconfigured`; die CLI behauptet keine bestandene
projektspezifische Quality Suite.

Ist eine gültige Datei vorhanden, sind ihre Gates lokal verpflichtend für
jeden `pre-push` mit mindestens einem offiziellen Arbeitsbranch. Die
Konfiguration definiert dafür einen Default-Scope und optionale Scopes je
Gate: `includeFamilies` wählt Familien aus, `excludeFamilies` entfernt sie
anschließend. Ein Gate ohne Scope erbt den Repository-Default. Die
Standardmenge enthält alle offiziellen Arbeitsfamilien; private `scratch/*`
ist nicht enthalten, kann aber gezielt für ein einzelnes Gate gewählt werden.

Die Suite führt bei einem Multi-Ref-Push jedes berechtigte Gate einmal nach
erfolgreicher Ref-Policy-Prüfung aus. Damit bleibt das Tool projekt- und
sprachenagnostisch: Es kennt keine vorgegebenen Build- oder Lint-Kommandos,
setzt aber einen vorhandenen expliziten Repository-Vertrag zuverlässig durch.

Im Governed-Publish-Pfad läuft die Vollsuite auf dem finalen Kandidaten nach
einer möglichen Synchronisation. Ihr kurzer lokaler Git-Metadatennachweis
bindet Revision, Zielbasis, Remote, Konfigurationsdigest, Gate-Auswahl,
Toolchain und sauberen Arbeitsbaum. Pre-Push prüft die Policy trotzdem immer
und akzeptiert den Nachweis nur bei exakter, frischer Übereinstimmung. Ohne
passenden Nachweis ist die Vollsuite der Raw-Push-Fallback; beschädigte
Nachweise blockieren fail-closed.

Quelle: [Go `os.UserConfigDir`](https://pkg.go.dev/os#UserConfigDir)

## 7. Lefthook-Installation

Lefthook bleibt ein eigenes organisationsweit standardisiertes Produkt. `git-governance` vendort oder kopiert keine eigenen Git-Hook-Skripte.

Repository-Setup:

1. Lefthook über den freigegebenen Paketkanal installieren.
2. `lefthook install` im Repository ausführen.
3. `lefthook validate` ausführen.
4. `git governance doctor` ausführen.

Lefthook selbst ist als eigenständige Binary verfügbar und unterstützt unter anderem Homebrew, WinGet und Scoop. Das passt zum gleichen Distribution-Prinzip.

Quellen:

- [Lefthook Installation](https://lefthook.dev/install/)
- [`lefthook install`](https://lefthook.dev/usage/commands/install/)

## 8. Release-Pipeline

Die Release-Pipeline trennt strikt:

### 8.1 Build

- exakten Git-Tag und Commit prüfen
- exakte Go-Toolchain provisionieren und mit `GOTOOLCHAIN=local` erzwingen
- Dependencies mit Checksums auflösen
- Modulgraph mit `go mod tidy -diff` ohne Mutation prüfen
- Build- und Testbefehle mit `-mod=readonly` ausführen
- `go test ./...`
- `go run ./cmd/check-coverage` mit uncached Coverage, verpflichtender
  `_test.go`-Datei je Go-Package und `100.0 %` für ausführbare Statements
- `go test -race ./...` auf nativen Testplattformen
- `go vet ./...`
- statische Analyse und Vulnerability Scan
- Dependency-Review bei jedem Pull Request sowie periodische CI-Neubewertung
- Cross-Platform-Binaries bauen

#### 8.1.1 Lokale GoReleaser-Validierung

Die Release-Konfiguration wird in CI mit derselben fest gebundenen
GoReleaser-Version wie der Release ausgeführt geprüft. Lokale Validierung ist
nur mit einer bereits freigegebenen und verifizierten GoReleaser-Binary
zulässig. Eine ad-hoc-Beschaffung per `go install ...@version` ist nicht Teil
des Release- oder Validierungsprozesses.

Der Check validiert ausschließlich die Konfiguration. Er veröffentlicht keine
Artefakte und benötigt keine Release-Credentials.

#### 8.1.2 Beschaffungsgrenze

Dieses Repository erzwingt bereits Toolchain-, Read-only- und VCS-Fallback-
Kontrollen in CI und Release. Die Umstellung auf einen internen Approved Proxy
und die dazugehörige Artifact-Registry-Admission ist eine externe
Plattformvoraussetzung und wird erst mit deren Bereitstellung aktiviert. Bis
dahin bleibt die bestehende Go-Proxy-Konfiguration unverändert.

#### 8.1.3 Dependency- und Runner-Governance

- Pull Requests durchlaufen eine auf einen unveränderlichen Commit gepinnte
  Dependency Review; neue Vulnerability-Findings jeder Schwere und in jedem
  Dependency-Scope blockieren den Check.
- CI führt dieselben Supply-Chain-Gates täglich zusätzlich zu Pull Requests,
  Pushes und manuellen Ausführungen aus.
- GitHub-hosted Jobs verwenden konkrete, versionsgebundene Runner-Labels statt
  `*-latest`. Diese Labels reduzieren Major-Version-Drift, sind jedoch kein
  Ersatz für eine später bereitzustellende unveränderliche Build-Enklave.

### 8.2 Package

- Windows-, macOS- und Linux-Artefakte erzeugen
- Shell Completions und Manpages erzeugen
- Lizenz und Notices beilegen
- SHA-256-Checksumme pro Artefakt erzeugen
- SBOM pro Artefakt erzeugen

Aktueller Artefaktvertrag:

- Archive enthalten `README.md`, `CONTRIBUTING.md`, `LICENSE` und `NOTICE`.
- Der vor dem Release ausgeführte Generator erstellt Bash-, Zsh-, Fish- und
  PowerShell-Completions sowie Cobra-Manpages unter `.build/generated/`; diese
  Dateien werden in jedes Archiv aufgenommen. `dist/` bleibt ausschließlich
  den von GoReleaser erzeugten Release-Artefakten vorbehalten.
- GoReleaser erzeugt Linux-Pakete in `deb`, `rpm` und `apk`.
- Windows-, Homebrew-, Scoop- und WinGet-Publikation benötigen weiterhin
  konkrete Publisher-, Bucket- oder Tap-Identitäten; diese werden nicht
  erfunden und erst nach ihrer Konfiguration veröffentlicht.
- Repository-lokale Homebrew-, Scoop- und WinGet-Templates liegen unter
  `packaging/` und werden ausschließlich mit Versions- und Checksumdaten eines
  unveränderlichen GitHub Releases ausgefüllt.

### 8.3 Verify

- native Smoke Tests je Zielklasse
- echte temporäre Git-Repositories verwenden
- `version`, `doctor`, Branch-Validierung und Commit-Validierung prüfen
- Install/Upgrade/Uninstall je Paketformat prüfen
- keine ungeplanten Profil- oder Repository-Mutationen zulassen

### 8.4 Sign und Publish

- Checksum-Manifest signieren, bevorzugt mit Sigstore/Cosign
- Windows-Code-Signing für Installer/Binary
- macOS Signing und Notarization für entsprechende Pakete
- Provenance/Attestation veröffentlichen
- unveränderlichen Release mit Artefakten veröffentlichen
- Package-Manager-Manifeste erst aus dem veröffentlichten Artefakt erzeugen

Alle Drittanbieter-Actions in CI und Release werden auf vollständige,
unveränderliche Commit-IDs gepinnt. GoReleaser, Syft und Cosign werden
zusätzlich auf konkrete Versionen gebunden. Govulncheck, Lefthook und
Staticcheck stammen aus dem versionierten `tools/go.mod`-Modul. Ein Release
wird nur von einem SemVer-Tag oder einem manuellen Run mit explizitem
bestehenden SemVer-Tag gebaut.

Der normale automatisierte Pfad lautet:

```text
git-governance workflow release cut --version <semver>
-> maschinenlesbarer Intent für execute-protected-line-request.yml
-> autorisierter release-request Controller bindet Ticket, Quell-SHA und Zielref
-> separat autorisierte Execution erzeugt release/<semver> aus origin/develop
-> automatischer Read-only-Finalizer verifiziert origin/release/<semver>
-> kontrollierte Stabilisierung und PR nach main
release/<semver> -> geschützter Merge nach main
-> CI prüft den Merge-Commit
-> CI erstellt den annotierten Tag v<semver> genau auf diesem Commit
-> CI startet den Artefaktworkflow für diesen Tag
-> GoReleaser baut, signiert, attestiert und veröffentlicht
-> Lifecycle-Adapter prüft Promotion, Tag, veröffentlichte Delivery und Delta
-> bei Delta: reviewbarer Backmerge-PR nach develop
-> ohne Delta: auditierbares not-required, kein leerer PR
```

`execute-protected-line-request.yml` wird ausschließlich vom gebundenen
`release-request` Controller mit einer `request_id` dispatcht. Der Executor
prüft Version, Quelllinie, Release-Tag bei Support-Linien, die Nichtexistenz
der Zielbranch und den dauerhaften Request-Record. Sein automatischer
Read-only-Finalizer verifiziert die erzeugte Remote-Linie. Eine GitHub-Ruleset-
oder Branch-Protection-Regel muss den Workflow als erlaubten Erzeuger von
`release/*` und `support/*` festlegen; die lokale CLI erhält dafür keine
Push-Berechtigung.

Wenn das `release/*`- oder `support/*`-Ruleset Required Status Checks
erzwingt, muss dessen Status-Check-Regel
`do_not_enforce_on_create: true` setzen. Andernfalls verlangt GitHub Checks
für eine Zielbranch, bevor diese überhaupt existiert, und blockiert den
kontrollierten Release- oder Support-Cut. Die Ausnahme gilt ausschließlich bei
der ersten Ref-Erzeugung; alle Schutzregeln gelten danach unverändert.

Ein mit `GITHUB_TOKEN` erzeugter Tag löst keinen weiteren Push-Workflow aus.
Der Tag-Workflow startet deshalb den vorhandenen `workflow_dispatch`-Pfad des
Artefaktworkflows explizit und übergibt den bestehenden Tag.

GoReleaser kann Builds, Archive, Checksummen, Signaturen und Paketmanager-Manifeste orchestrieren. Es bleibt Build-/Release-Tool und ist keine Runtime-Abhängigkeit.

Quellen:

- [GoReleaser](https://goreleaser.com/)
- [GoReleaser Checksums](https://goreleaser.com/customization/package/checksum/)
- [GoReleaser Signing](https://goreleaser.com/customization/sign/sign/)
- [GoReleaser SBOM](https://goreleaser.com/customization/sbom/)

## 9. Versions- und Update-Modell

- SemVer 2.0.0
- Release-Tags: `v<semver>`
- Binary meldet Version, Commit und Build-Provenance
- veröffentlichte Artefakte werden nie ersetzt
- Update erfolgt über denselben Kanal wie die Installation
- kein automatisches Self-Update in Version 1

Ein Self-Updater würde Signaturprüfung, Proxy, Rollback, Kanalwahl und Package-Manager-Ownership duplizieren. Er wird nur bei einem später nachgewiesenen Offline- oder Flottenbedarf erneut bewertet.

## 10. Atomare Installation und Rollback

Paketmanager müssen Upgrade und Rollback entsprechend ihrer Plattform übernehmen. Direkte Installer:

1. Artefakt und Signatur vor Mutation prüfen.
2. neue Binary in einen temporären Pfad schreiben.
3. ausführbaren Smoke Test ausführen.
4. Ziel atomar ersetzen, soweit die Plattform dies erlaubt.
5. vorherige Version für kontrollierten Rollback erhalten.

Konfigurationsmigrationen sind vorwärtskompatibel und dürfen eine ältere Binary nicht stillschweigend zerstören. Breaking Schema-Änderungen benötigen Expand/Contract oder explizite Migration mit Backup.

## 11. Uninstall

Uninstall entfernt:

- das vom Paketmanager besessene Programm
- vom Paket installierte Completions und Manpages

Uninstall entfernt standardmäßig nicht:

- Benutzerkonfiguration
- Repository-Konfiguration
- Lefthook
- Git-Daten

Ein separater `--purge`-Pfad darf Benutzerkonfiguration nur nach expliziter Bestätigung löschen.

## 12. Verbotene Installationsmuster

- ungefragtes Editieren mehrerer Shellprofile
- `ExecutionPolicy Bypass` als regulärer Installationsvertrag
- `curl | sh` ohne Artefaktprüfung
- Download der neuesten ungepinnten Binary bei jedem Start
- Installation einer Go-Toolchain auf Endgeräten
- parallele Installer mit eigener fachlicher Logik pro Betriebssystem
- Vermischung von Lefthook-, CLI- und Repository-Policy-Installation
- Änderung veröffentlichter Artefakte unter demselben Tag
- mutable GitHub-Action-Tags oder ungepinnte `@latest`-Toolinstallationen in
  CI- und Release-Workflows
- Veröffentlichung in einen Homebrew Tap, Scoop Bucket, WinGet oder
  Plattform-Signing-Kanal ohne nachweislich maintainer-kontrollierte
  Zielidentität und Zugangsdaten

## 13. Abnahmekriterien

- frische Installation auf jeder Zielplattform
- Aufruf sowohl als `git governance` als auch als `git-governance`
- neues Terminal findet die Binary
- Upgrade erhält Benutzerkonfiguration
- Uninstall entfernt keine Benutzerdaten ohne `--purge`
- Checksumme und Signatur sind verifizierbar
- Offline-Aufruf der lokalen Validierung funktioniert
- keine Sprachruntime ist auf dem Endpoint erforderlich
- kein Shellprofil wird durch die Standardinstallation direkt editiert
