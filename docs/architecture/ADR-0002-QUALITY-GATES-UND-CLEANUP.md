# ADR-0002: Quality-Gate-Default und Cleanup-Verantwortung

- Status: angenommen
- Datum: 2026-07-10
- Geltungsbereich: lokale Push-Prüfung, projektspezifische Quality Gates und Branch-Cleanup

## Entscheidung

Eine vorhandene, gültige `git-governance.quality.json` aktiviert die lokale
Quality-Suite verbindlich für jeden Push, der mindestens einen offiziellen
Arbeitsbranch aktualisiert:

```text
feature  fix  docs  refactor  chore  test  perf  hotfix
```

`scratch/*` ist standardmäßig nicht Teil dieser Suite. Ein Repository kann
Scratch explizit in einen einzelnen, passenden Gate-Scope aufnehmen.

Die Konfigurationsdatei selbst ist das Opt-in. Ohne Datei wird keine
projekt- oder sprachspezifische Build-, Test- oder Lint-Annahme getroffen; der
Status lautet dann `unconfigured`, nie `passed`.

Für eine reguläre Veröffentlichung läuft die vollständige lokale Suite erst,
nachdem der Branch gegen seine aktuelle Zielbasis synchronisiert wurde. Ein
bestandener Lauf erzeugt einen kurzlebigen, revisionsgebundenen Nachweis in
der lokalen Git-Metadatenauflösung unter
`git-governance.final-quality-evidence`. Der Nachweis enthält keine Secrets
und wird nicht committed. Er bindet mindestens ausgehende Ref und Commit,
Remote und Zielbasisrevision, Quality-Konfigurationsdigest, Gate-Auswahl,
Toolchain, sauberen Arbeitsbaum und Erstellungszeit.

Der Pre-Push-Pfad prüft weiterhin jede tatsächliche Ref-Aktualisierung
strukturell. Er darf den Nachweis nur wiederverwenden, wenn alle Bindungen
noch exakt passen und der Nachweis frisch ist. Fehlt er, ist er abgelaufen
oder passt nicht, läuft die vollständige Suite einmal als Fallback. Ein
beschädigter oder unvollständiger Nachweis blockiert fail-closed. Weder
`--no-verify` noch Hook-Deaktivierung oder ein ungebundener Skip-Schalter sind
eine zulässige Deduplizierung.

Dieses Repository verwendet zusätzlich den plattformneutralen Coverage-Gate
über das gepinnte Werkzeug (`go tool -modfile tools/go.mod check-coverage`).
Er führt `go test -count=1 -cover ./...` aus und bricht ab, wenn ein
Go-Package keine `_test.go`-Datei enthält oder ein Package mit ausführbaren
Statements nicht exakt `100.0 %` erreicht. Derselbe Gate läuft lokal, im
Pre-Push-Pfad und in CI.

Die CLI löscht standardmäßig nur lokale `scratch/*`-Branches und entfernt dabei
deren lokale Workflow-Basis-Metadaten. Sie löscht nie Remote-Branches und
löscht keine Shared Lines. Die Remote-Löschung von Ticket- und Hotfix-Branches
liegt bei GitHub, GitLab oder einer gleichwertigen Hosting-Automation. Ein
Release-Branch wird erst nach Promotion nach `main`, bestätigter
Tag-/Artefakt-Delivery und abgeschlossener Reconciliation nach `develop` durch
CI oder Hosting-Automation entfernt. Ein auditierbares `not-required` ist ein
gültiger Reconciliation-Abschluss, wenn kein effektiver Delta bleibt.

## Entscheidungsbewertung

| Rang | Modell | Absoluter Fit | Normalisierter Anteil |
|---:|---|---:|---:|
| 1 | Repository-Konfiguration aktiviert Pflicht-Gates auf allen offiziellen Arbeitsbranches; Scratch ist explizites Opt-in | 94,0/100 | 53 % |
| 2 | Repository-Konfiguration aktiviert Gates nur in expliziten Workflow-Kommandos | 82,0/100 | 46 % |
| 3 | Die CLI enthält fest verdrahtete Build-/Lint-Befehle für jedes Repository | disqualifiziert | 1 % |

Das gewählte Modell gewinnt, weil es lokale Umgehung durch direktes
`git push` verhindert, ohne eine Sprache, ein Buildsystem oder einen
Paketmanager zu erraten. Es hält die fachliche Governance im Produktkern und
die projektspezifischen Befehle in einem explizit überprüfbaren
Repository-Vertrag.

Die normalisierten Anteile gelten nur für diese drei Optionen. Sie sind weder
Marktanteile noch allgemeine Qualitätswerte.

## Quality-Gate-Vertrag

```text
gültige Quality-Datei
→ Pre-Push prüft alle tatsächlichen Git-Ref-Updates
→ Shared-Line-, Rewrite- und Basisregeln bestehen
→ passender finaler Nachweis wird nur für exakt dieselben Updates verwendet
→ sonst laufen berechtigte Gates einmal pro Push
→ Push darf fortgesetzt werden
```

Ein Gate darf über `includeFamilies` und `excludeFamilies` enger gefasst
werden, etwa ein Dokumentationscheck nur auf `docs/*` oder ein Lasttest auf
`feature/*` und `perf/*`. Diese explizite Scoping-Regel ändert nicht den
Default: Auf jeder offiziellen Arbeitsbranch-Aktualisierung wird die dafür
anwendbare konfigurierte Suite ausgeführt.

Unvertrauenswürdige Repository-Konfiguration wird nicht ausgeführt. Der
Runner akzeptiert nur ein Executable mit Argumentarray, eine
repository-relative Working Directory und einen positiven Timeout. Shell-Strings
und Pfade außerhalb des Repository-Roots sind ausgeschlossen.

## Cleanup-Vertrag

| Branch-Klasse | Lokale CLI-Löschung | Remote-Löschung | Verantwortliche Instanz |
|---|---|---|---|
| `scratch/*` | ja | nicht vorgesehen | Entwickler / CLI |
| reguläre Ticket-Branches | nein | nach PR-Merge | Hosting-Plattform |
| `hotfix/*` | nein | nach Zielmerge und Weiterleitung | Hosting-Plattform / CI |
| `release/*` | nein | erst nach Main-Promotion, bestätigter Delivery und abgeschlossener Reconciliation | CI / Hosting-Automation |
| `main`, `develop`, aktive `support/*` | nein | nein | Branch Protection |

Die CLI darf keinen Merge-, Pull-Request- oder Weiterleitungsabschluss
behaupten, solange kein autoritativer Hosting-Adapter diesen Nachweis liefert.
Eine allgemeine lokale oder Remote-Löschfunktion würde diese Vertrauensgrenze
überschreiten.

## Tag-Lebenszyklus

```text
release/<semver> → Pull Request nach main → geschützter Merge
→ CI prüft den Main-Merge-Commit
→ CI erzeugt den annotierten unveränderlichen Tag v<semver>
→ CI startet Artefakt-Build, Signierung und Veröffentlichung
→ Lifecycle-Adapter prüft Promotion, Tag, Delivery und effektiven Delta
→ bei Delta: release/<semver> → Pull Request nach develop
→ ohne Delta: auditierbares not-required
→ CI oder Hosting-Automation bereinigt die abgeschlossene Release-Linie
```

Die lokale CLI erzeugt weder Tags noch direkte `main`-, `release/*`- oder
`support/*`-Pushes. Sie erstellt ausschließlich providerneutrale Intents,
welche ein geschützter, autorisierter CI-Workflow ausführt.
