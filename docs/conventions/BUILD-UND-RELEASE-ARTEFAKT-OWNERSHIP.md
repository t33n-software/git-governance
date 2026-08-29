# Build- und Release-Artefakt-Ownership

## Zweck

Diese Konvention trennt versionierte Quellen, flüchtige Build-Inputs und
veröffentlichungsfähige Release-Artefakte. Sie verhindert, dass lokale
Smoke-Binaries oder generierte Packaging-Dateien den von GoReleaser verwalteten
Artefaktbereich verunreinigen.

## Verzeichnisvertrag

| Pfad | Owner | Versioniert | Zulässiger Inhalt |
| --- | --- | --- | --- |
| `docs/` | Produkt- und Betriebsdokumentation | Ja | Handgeschriebene, reviewbare Dokumentation |
| `build/` | Build-Infrastruktur, falls benötigt | Ja | Rezepte, Skripte oder Konfiguration, keine Outputs |
| `.build/` | Lokale und CI-Build-Schritte | Nein | Smoke-Binaries sowie generierte Packaging-Inputs |
| `dist/` | GoReleaser | Nein | Archive, Pakete, Checksums, SBOMs, Signaturen und Artefaktmetadaten |
| `release/` | Release-Lifecycle-Begriff | Nicht als Output-Verzeichnis | Release-Workflow, immutable Tags, GitHub Release und Reconciliation |

`.build/` und `dist/` werden durch `.gitignore` ausgeschlossen. Der führende
Punkt in `.build/` kennzeichnet einen tool-privaten Workspace; Go-Paketmuster
wie `./...` ignorieren Verzeichnisse mit führendem Punkt zusätzlich.

## Build- und Release-Fluss

```text
Versionierte Quellen und docs/
        ↓
.build/bin/ und .build/generated/
        ↓
GoReleaser
        ↓
dist/
        ↓
GitHub Release, Checksums, SBOMs, Signaturen und Attestationen
```

Die kanonische Quality-Gate (`go tool -modfile tools/go.mod quality-gate`)
und die nativen CI-Smoke-Tests schreiben ihre ausführbare Binary
nach `.build/bin/`. `cmd/generate-docs` erzeugt Shell-Completions und Manpages
unter `.build/generated/`. GoReleaser nimmt diese Dateien als Packaging-Input
auf, erzeugt seine eigenen Outputs aber ausschließlich in `dist/`.

## Verbindliche Regeln

1. Kein vorbereitender Generator, lokaler Smoke-Test oder manueller Build darf
   nach `dist/` schreiben.
2. Nur GoReleaser darf `dist/` für Release-Artefakte erzeugen und verwalten.
3. `release/` oder `.release/` dürfen nicht als Ersatz für den flüchtigen
   Build-Workspace verwendet werden.
4. Handgeschriebene Dokumentation bleibt unter `docs/`; nur reproduzierbare
   Packaging-Inputs gehören unter `.build/generated/`.
5. Änderungen an diesen Pfaden benötigen eine direkte Contract-Test-Aktualisierung
   sowie eine nicht veröffentlichende GoReleaser-Snapshot-Prüfung.

## 12-Factor-Einordnung

`.build/` gehört zur Build-Phase und enthält keine veröffentlichungsfähige
Wahrheit. `dist/` ist der Artefaktübergaberaum zwischen Build und Release.
Der Release kombiniert den immutable Tag mit der Release-Konfiguration; der
Run verwendet ausschließlich veröffentlichte Artefakte. Kein Runtime-Prozess
schreibt in `.build/` oder `dist/`.
