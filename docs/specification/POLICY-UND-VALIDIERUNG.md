# Policy- und Validierungsspezifikation

## 1. Zweck und Autorität

Dieses Dokument ist der ausführbare, eigenständige Konventionsvertrag für
`git-governance`. Es ist keine zweite Policy Registry. Die anfängliche
Implementierung validiert Ticket-Keys nur syntaktisch; die spätere
Registry-/Bundle-Prüfung wird über denselben `KeyPolicy`-Port ergänzt.

Die Syntax ist vollständig für den hier gebundenen Scope:

- alle 13 in diesem Dokument definierten Branch-Familien
- Ticket-, Slug-, Release- und Support-Namen
- Conventional Commits / Angular-Style mit Ticket-Bezug
- Breaking Changes
- Cross-Field- und Repository-Zustandsregeln

„Vollständig“ bedeutet nicht, dass ein einzelner Regex fachlichen Git-Zustand beweisen kann. Die Implementierung verwendet einen Parser, kleine Regexe pro Value Object und anschließende semantische Prüfungen.

## 2. Validierungspipeline

Jede Eingabe durchläuft in dieser Reihenfolge:

1. Größen- und Kontrollzeichenlimit
2. Normalisierung nur dort, wo sie verlustfrei und explizit ist
3. lexikalische Regex-Prüfung
4. Parsing in Value Objects
5. Cross-Field-Regeln
6. Git-Referenzprüfung mit `git check-ref-format --branch`
7. Repository- und Publication-State-Regeln
8. optionale Policy-Registry-/Bundle-Prüfung

Ungültige Eingaben werden nie stillschweigend in andere Werte umgeschrieben.
Die interaktive UI zeigt für das betroffene Feld den sicheren tatsächlichen
Wert, Regel, erwartetes Format, gültiges Beispiel und Korrektur und fragt
denselben Wert erneut ab. Sie verlässt weder das Feld noch startet sie den
Workflow neu; der Benutzer kann unbegrenzt korrigieren. Die UI darf eine
explizite Korrektur vorschlagen, benötigt aber die Bestätigung des Benutzers.

## 3. Ticket-Key

### 3.1 Entscheidung

Keys dürfen Großbuchstaben und Zahlen enthalten, müssen aber mit einem Großbuchstaben beginnen:

```regex
^[A-Z][A-Z0-9]*$
```

Beispiele:

- gültig: `ABC`, `PLATFORM2`, `A1`
- ungültig: `abc`, `2ABC`, `ABC-OPS`, `ABC_OPS`, leer

Zahlen müssen erlaubt sein, weil ein syntaktischer Namespace nicht unnötig auf reine Buchstaben eingeschränkt werden soll. Ein führender Buchstabe hält die Trennung zwischen Key und Ticketnummer eindeutig. Bindestriche sind im Key verboten, weil der erste Bindestrich den Key von der Ticketnummer trennt.

Zusätzliche technische Grenzen:

- Länge: 1 bis 32 ASCII-Zeichen
- keine Leer- oder Steuerzeichen

Die Längenbegrenzung ist ein Schutzlimit des CLI-Vertrags, keine Aussage über eine Policy Registry. Ohne Registry wird jeder Key akzeptiert, der diese Syntax erfüllt; es gibt keine Allowlist.

### 3.2 Ticketnummer

```regex
^[1-9][0-9]*$
```

Zusätzliche technische Grenze: maximal 18 Ziffern.

Damit sind `0`, negative Werte, Vorzeichen, Dezimalstellen und führende Nullen nicht kanonisch. Die Nummer wird als String behandelt, damit kein Integer-Overflow in die Domain gelangt.

### 3.3 Ticket-ID

```regex
^([A-Z][A-Z0-9]*)-([1-9][0-9]*)$
```

Beispiele:

- gültig: `ABC-123`, `PLATFORM2-7`
- ungültig: `ABC123`, `abc-123`, `ABC-001`, `ABC-0`

## 4. Branch-Slug

Der Slug ist kanonisches ASCII-`kebab-case`:

```regex
^[a-z0-9]+(?:-[a-z0-9]+)*$
```

Beispiele:

- gültig: `add-export-button`
- gültig: `oauth2-token-refresh`
- gültig: `docs-v2`
- ungültig: `Add-Export`
- ungültig: `add--export`
- ungültig: `-add-export`
- ungültig: `add-export-`
- ungültig: `feature/frontend`

Zusätzliche technische Grenze: 1 bis 100 Zeichen.

Der Regex des alten Projekts, `^[a-z0-9-]+$`, ist nicht ausreichend, weil er führende, folgende und doppelte Bindestriche akzeptiert.

## 5. Branch-Familien

### 5.1 Vollständige Taxonomie

| Familie | Namensform | Erstellung |
|---|---|---|
| `main` | exakt `main` | nicht über normalen Wizard |
| `develop` | exakt `develop` | nicht über normalen Wizard |
| `release` | `release/<semver>` | nur Release-Workflow |
| `support` | `support/<major.minor>` | nur Support-/Release-Workflow |
| `feature` | `feature/<ticket>-<slug>` | regulärer Ticket-Workflow |
| `fix` | `fix/<ticket>-<slug>` | regulärer Ticket-Workflow |
| `docs` | `docs/<ticket>-<slug>` | regulärer Ticket-Workflow |
| `refactor` | `refactor/<ticket>-<slug>` | regulärer Ticket-Workflow |
| `chore` | `chore/<ticket>-<slug>` | regulärer Ticket-Workflow |
| `test` | `test/<ticket>-<slug>` | regulärer Ticket-Workflow |
| `perf` | `perf/<ticket>-<slug>` | regulärer Ticket-Workflow |
| `hotfix` | `hotfix/<ticket>-<slug>` | Hotfix-Workflow |
| `scratch` | `scratch/<ticket>-<slug>` | private Exploration |

`feature` ist Branch-Familie; `feat` ist Commit-Typ. Aliase wie `feat/`, Schreibfehler wie `featch/`, Entwicklernamen und zusätzliche Pfadsegmente werden nicht akzeptiert.

### 5.2 Offizielle Ticket-, Hotfix- und Scratch-Branches

```regex
^(feature|fix|docs|refactor|chore|test|perf|hotfix|scratch)/([A-Z][A-Z0-9]*)-([1-9][0-9]*)-([a-z0-9]+(?:-[a-z0-9]+)*)$
```

Capture-Gruppen:

1. Branch-Familie
2. Ticket-Key
3. Ticketnummer
4. Slug

Beispiele:

- `feature/ABC-123-add-export-button`
- `fix/PLATFORM2-7-handle-null-customer-id`
- `hotfix/ABC-999-payment-timeout`
- `scratch/ABC-123-export-button-exploration`

Nicht erlaubt:

- `feat/ABC-123-add-export-button`
- `feature/frontend/ABC-123-add-export-button`
- `feature/ABC-123/add-export-button`
- `feature/dennis/ABC-123-add-export-button`
- `feature/ABC-123-add--export-button`

### 5.3 Release-Branches

Ein Release verwendet Semantic Versioning 2.0.0 ohne führendes `v`.

SemVer-Komponente:

```regex
^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$
```

Branch:

```text
release/<semver>
```

Beispiele:

- gültig: `release/2.8.0`
- gültig: `release/2.8.0-rc.1`
- gültig: `release/2.8.0-rc.1+build.7`
- ungültig: `release/v2.8.0`
- ungültig: `release/02.8.0`
- ungültig: `release/2.8`

Die Implementierung prüft zuerst das Präfix und validiert danach die Version mit einem eigenen `SemanticVersion`-Parser. Ein zusammengesetzter Monster-Regex wird nicht als einzige Fehlerquelle verwendet.

Quelle: [Semantic Versioning 2.0.0](https://semver.org/)

### 5.4 Support-Branches

Komponente:

```regex
^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$
```

Branch:

```text
support/<major.minor>
```

Beispiele:

- gültig: `support/2.7`
- gültig: `support/0.9`
- ungültig: `support/2.7.1`
- ungültig: `support/v2.7`

### 5.5 Gemeinsame Linien

```regex
^(main|develop)$
```

Die syntaktische Gültigkeit bedeutet nicht, dass der Benutzer dort arbeiten darf. Direkte Commits und Pushes werden durch Repository-State-Regeln blockiert.

## 6. Branch-semantische Regeln

Regexe können folgende Regeln nicht beweisen und werden deshalb durch Domain- und Git-Prüfungen ergänzt:

- reguläre Ticket-Branches starten von `origin/develop`
- Hotfixes starten von der real betroffenen `origin/main`-, `origin/release/*`- oder `origin/support/*`-Linie
- Release-Cuts starten von `origin/develop`
- `scratch/*` ist privat und kein PR-Ziel
- ein direkt erstellter `scratch/*`-Branch startet von einem lokalen offiziellen Ticket-Branch mit derselben Ticket-ID
- ein Scratch-Transfer nutzt ausschließlich einen vorhandenen lokalen offiziellen
  Ticket-Branch mit derselben Ticket-ID; Scratch- und Ziel-Slug müssen nicht
  identisch sein
- genau ein lokaler offizieller Kandidat wird automatisch gewählt; kein
  Kandidat führt zu `SCRATCH_TARGET_BRANCH_MISSING`, mehrere Kandidaten
  erfordern einen expliziten Zielbranch
- der Transfer nutzt `git merge --squash` und erzeugt einen einzelnen
  ticket-konsistenten Conventional Commit auf dem offiziellen Zielbranch
- `main`, `develop`, `release/*` und `support/*` sind Shared Lines
- pro Ticket existiert im normalen Workflow genau ein offizieller Arbeitsbranch
- die Ticket-Exklusivität wird nach `fetch --prune` gegen lokale und
  ausgewählte Remote-Tracking-Branches geprüft
- Release-Stabilisierung und Hotfix-Propagation sind explizite
  Active-Line-Workflows und keine regulären Ticket-Zweitbranches
- vor dem ersten Push wird die tatsächliche Zielbasis geprüft
- Rebase ist nur vor dem ersten Push und nur bei fehlenden Basis-Commits zulässig
- nach dem ersten Push ist ein offizieller Branch append-only
- Force Push ist für offizielle Branches verboten

Publication State wird nach erfolgreichem Fetch über die Remote-Referenz bestimmt. Kann der Zustand nicht belastbar ermittelt werden, blockiert eine history-rewriting Operation sicher, statt einen unveröffentlichten Branch anzunehmen.

### 6.1 Release-Stabilisierung

Nach einem Release-Cut sind nur drei stabilisierende Branch-Kategorien
zulässig:

| Kategorie | Branch-Familie | Erlaubter Zweck |
|---|---|---|
| `blocker` | `fix/*` | release-blockierender Fehler |
| `docs` | `docs/*` | letzte technische oder operative Dokumentation |
| `release-prep` | `chore/*` | Version, Freigabevorbereitung oder technische Release-Arbeit |

Diese Branches starten aus `origin/release/<semver>` und zielen per PR auf
dieselbe Release-Linie. Feature-, allgemeine Refactor- und themenfremde
Arbeit ist im Stabilisierungspfad nicht zulässig.

### 6.1.1 Promotion-Base-Ausrichtung

Ein Promotion-PR `release/<semver> -> main` kann bei strikten
Main-Statuschecks strukturell veraltet sein. Das ist keine generelle
Rebase-Erlaubnis und erlaubt niemals eine direkte Aktualisierung der
`release/*`-Ref über GitHub **Update branch**, einen Rebase oder einen
Force Push.

Eine Ausrichtung ist nur zulässig, wenn:

1. `main` tatsächlich eine aktuelle Promotion-Basis verlangt;
2. die fehlenden Main-Commits release-kompatibel sind;
3. eine `release-prep`-`chore/*`-Branch mit gespeicherter
   `origin/release/<semver>`-Basis aktiv ist.

`workflow release align-promotion-base` prüft diese Herkunft, merged
`origin/main` ausschließlich in die Working-Branch, führt die
Repository-Quality-Suite aus und veröffentlicht einen PR zurück auf dieselbe
Release-Linie. Der erzeugte Merge-Commit ist ticketgebunden und append-only.
Erst nach dessen Merge werden der bestehende Promotion-PR und seine
Main-Gates erneut bewertet.

Konflikte bleiben auf der Preparation-Branch fail-closed. Eine Resolution darf
nur die exakten Konfliktpfade stagen. Der CLI-Resume-Pfad prüft vor der
Fortsetzung, dass der aktive Merge weiterhin den nach Fetch aktuellen
`origin/main`-Commit integriert, und prüft diese Basis erneut vor Quality,
Push und PR. Bei einer weitergelaufenen Main-Linie ist der Kandidat verworfen;
ein Rohaufruf von `git merge --continue`, ein Rebase oder eine
Release-Ref-Mutation sind keine zulässigen Recovery-Mechanismen.

GitHub Rulesets schützen die zugrunde liegende Release-Ref, können aber die
UI-Aktion anhand ihrer Head-Branch-Familie nicht eigenständig ausblenden.
Deshalb müssen direkte Ref-Updates serverseitig abgewiesen und die
source-aware Ausrichtung durch den Workflow kontrolliert werden.

### 6.2 Support-Provenance

`support/<major.minor>` darf nur aus `origin/main` entstehen, wenn die
Revision einen passenden Release-Tag `v<major.minor.patch>` trägt. Dadurch
beginnt eine Support-Linie nicht aus einem unfreigegebenen Integrationsstand.

### 6.3 Hotfix-Weiterleitung

Ein Hotfix-PR zielt auf dieselbe betroffene Linie. Wenn dieselbe Änderung in
einer weiteren aktiven Linie benötigt wird, erzeugt das Tool dort einen
kontrollierten `fix/*`-Branch und führt `git cherry-pick -x <sha>` aus. Die
Herkunft bleibt dadurch in der Commit-Historie sichtbar.

Ein Main-Hotfix erhält vor dem Merge einen versionierten Record unter
`.git-governance/hotfix-release-records/<KEY-NUMBER>.json`. Der Record bindet
Ticket, Incident, betroffene Linie, vorherigen Tag, Patch-Zielversion,
Same-Repository-PR, vollständiges SHA-Manifest, Commit-Budget-Ausnahme und
alle zusätzlichen Ziel-Linien. Ein bis drei semantische Manifest-Commits sind
normal; vier benötigen eine begründete Ausnahme. Fünf oder mehr benötigen
zusätzlich eine explizite, im Record referenzierte Release-Freigabe; ohne sie
wird der Main-Merge fail-closed abgewiesen.

Die Ein-Commit-Oberfläche `workflow hotfix propagate` bleibt für einen
reviewten Commit bestehen. `workflow hotfix propagate-manifest` darf nur
deklarierte Ziele verwenden und erzeugt einen lokalen, nicht-shared
`fix/*`-Kandidaten. Er speichert den Resume-Cursor ausschließlich in lokaler
Git-Metadatenauflösung, nicht im Record und nicht im Arbeitsbaum. Der Kandidat
wird ausschließlich durch die getrennte Hotfix-Propagation-Publisher-Grenze
veröffentlicht. Der geschützte Controller revalidiert Delivery, Record,
Manifest, Ziel-Linie und Quality, konfiguriert nur temporäre maskierte
Git-Transport-Credentials und ruft `propagate-manifest --publish` auf. Es gibt
keine lokale `--push`- oder PR-Umgehung, keinen pauschalen `main -> develop`-
Merge und keine direkte Shared-Line-Mutation.

Ein serverseitiger Main-Hotfix-Delivery-Controller revalidiert Record,
PR-Identität, Merge, Manifest und Tag-Idempotenz, bevor er den immutable
Patch-Tag erzeugt. Anschließend ist der Release erst nach nicht-draft Release,
Payload, Checksums, SBOM, Sigstore-Bundle und erfolgreichem Artifact-Workflow
vollständig. Ein pauschaler `main -> develop`-Merge ist weder Propagation noch
Reconciliation und bleibt verboten.

### 6.4 Lokale Workflow-Basis-Metadaten

Hotfix-, Release-Stabilisierungs- und Propagation-Workflows speichern ihre
tatsächliche Remote-Basis in der lokalen Git-Konfiguration. Der Schlüssel ist
eine JSON-Map unter `git-governance.workflow-bases`; er ist keine globale
Policy und wird nicht committed. `sync-base`, Ticket-Publish und `pre-push`
lesen diese Basis, wenn keine explizite `--base` übergeben wurde. Dadurch wird
ein Hotfix- oder Stabilisierung-Branch nicht fälschlich gegen
`origin/develop` geprüft.

### 6.5 Repository-Quality-Gates

Eine vorhandene, gültige `git-governance.quality.json` ist ein expliziter
Repository-Vertrag. Auf allen offiziellen Arbeitsbranches sind ihre Gates
vor jedem Push verpflichtend. Der Pre-Push-Validator prüft alle tatsächlichen
Ref-Updates immer strukturell. Ein Push mehrerer offizieller Refs führt die
Suite höchstens einmal aus.

Die Konfiguration verwendet einen Default-Scope und einen Scope je Gate.
`includeFamilies` wählt Branch-Familien aus; `excludeFamilies` wird danach
angewendet und entfernt Familien. Ein Gate ohne eigenen Scope erbt den
Default. Dadurch kann eine Basissuite auf allen offiziellen Arbeitsfamilien
laufen, ein Dokumentations-Linkcheck nur auf `docs/*`, und ein aufwendiger
Stress-Test nur auf `feature/*` und `perf/*`.

`scratch/*` ist private Exploration und nicht Teil des Default-Scopes. Ein
konkretes Gate kann Scratch aber über `includeFamilies` bewusst einschließen.
Eine fehlende Datei lautet stets `unconfigured`, nie `passed`.

Die finale lokale Suite eines Publish-Workflows läuft erst nach der letzten
zulässigen Synchronisationsmutation. Ihr Ergebnis wird als kurzlebiger,
revisionsgebundener Datensatz unter
`git-governance.final-quality-evidence` in der lokalen Git-Konfiguration
gespeichert, nicht im versionierten Arbeitsbaum. Der Datensatz enthält keine
Credentials und bindet Ref und Commit, Zielbasisrevision, Remote,
Konfigurationsdigest, Gate-Auswahl, Toolchain, sauberen Arbeitsbaum und
Erstellungszeit.

Ein passender Nachweis verhindert nur die Wiederholung derselben lokalen
Suite. Er ersetzt nie die strukturelle Pre-Push-Policy, Remote-CI, Required
Checks, Review oder Shared-Line-Schutz. Fehlt der Nachweis, ist er abgelaufen
oder passt nicht exakt, läuft die Suite einmal als Fallback. Beschädigte oder
unvollständige Nachweise werden fail-closed abgewiesen; `--no-verify`,
Hook-Deaktivierung und ungebundene Skip-Schalter bleiben unzulässig.

### 6.6 Cleanup-Grenze

Remote-Löschung ist keine CLI-Aufgabe. Hosting-Plattform und CI steuern die
Löschung gemergter Ticket- und Hotfix-Branches sowie den zeitlich späteren
Release-Cleanup nach Promotion, bestätigter Delivery und Reconciliation. Die
CLI löscht ausschließlich:

- lokale `scratch/*`-Branches,
- niemals offizielle Ticket-, Hotfix-, Release- oder Support-Branches,
- niemals `main` oder `develop`,
- niemals einen Remote-Branch.

Beim lokalen Löschen entfernt die CLI die zugehörige lokale
`git-governance.workflow-bases`-Metadatenzeile. Merge-, PR- und
Forward-/Backport-Nachweise gehören zu Hosting-/CI-Gates, solange kein
konfigurierter Hosting-Adapter diese Daten autoritativ liefern kann.

### 6.7 Release-Reconciliation

Nach jeder Release-Delivery wird `release/<semver>` gegen `develop`
reconciliert. Diese Prüfung ist verpflichtend, aber ein Backmerge-PR ist es
nur bei effektivem Delta:

- Der Lifecycle-Provider muss den gemergten Promotion-PR, den exakt zugehörigen
  unveränderlichen Tag und die erfolgreiche Release-Delivery nachweisen.
- Der Provider vergleicht anschließend `release/<semver>` mit `develop`.
- Bei effektivem Delta erzeugt `workflow release backmerge` den PR nach
  `develop`.
- Ohne effektiven Delta meldet das Kommando `not-required`; der auditierbare
  Nachweis belegt, dass kein Merge nötig ist.

Ein offener Promotion-PR, ein bloßer Tagname oder eine fehlende
Artefakt-Delivery sind keine zulässigen Backmerge-Voraussetzungen.

Erzwingt das Develop-Ziel einen aktuellen PR-Head, darf die veröffentlichte
`release/<semver>`-Ref nicht aktualisiert werden. In diesem Fall erstellt eine
ticketgebundene, release-abgeleitete `chore/*`-Preparation-Branch den geprüften
Kombinationsstand: Sie merged `origin/develop`, durchläuft Quality-Gates und
erzeugt einen Merge-Commit-PR nach `develop`. Der Workflow verlangt die
gespeicherte Release-Basis, einen effektiven Delta und vollständige
Delivery-Evidenz. Ein Rebase, Force Push, Plattform-Update der Release-Ref oder
ein `develop -> release/<semver>`-PR verletzt den Vertrag.

## 7. Commit-Typen

Zugelassene kanonische Typen:

| Typ | Bedeutung |
|---|---|
| `feat` | neue Funktion |
| `fix` | Fehlerkorrektur |
| `docs` | ausschließlich Dokumentation |
| `refactor` | Umbau ohne Feature oder Bugfix |
| `chore` | Maintenance oder Tooling |
| `test` | Tests |
| `perf` | Performanceverbesserung |
| `build` | Buildsystem oder externe Dependencies |
| `ci` | CI-Konfiguration und -Skripte |
| `style` | Formatierung ohne semantische Änderung |
| `revert` | bewusste Rücknahme, mit Referenz im Body/Footer |

Branch- und Commit-Typ sind getrennte Taxonomien. Ein `feature/*`-Branch verwendet typischerweise `feat`, kann für fachlich getrennte Test- oder Dokumentationscommits aber auch `test` oder `docs` verwenden.

## 8. Commit-Header

Der Ticket-Bezug ist der verpflichtende Scope:

```regex
^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)\(([A-Z][A-Z0-9]*)-([1-9][0-9]*)\)(!)?: ([^\r\n]+)$
```

Capture-Gruppen:

1. Commit-Typ
2. Ticket-Key
3. Ticketnummer
4. optionales Breaking-`!`
5. Beschreibung

Zusätzliche Regeln für die Beschreibung:

- nach `: ` nicht leer
- keine führenden oder folgenden Leerzeichen
- keine Steuerzeichen
- genau eine Header-Zeile
- technische Obergrenze: 200 Unicode-Codepoints
- keine automatische Änderung der Groß-/Kleinschreibung

Beispiele:

```text
feat(ABC-123): add export button
fix(ABC-123): address review feedback on export validation
docs(ABC-123): document export workflow
feat(ABC-123)!: replace the export contract
```

## 9. Vollständige Commit-Nachricht

Die Nachricht wird geparst, nicht mit einem einzigen Multiline-Regex validiert:

```text
<header>

[optionaler freier Body]

[optionale Footer]
```

Regeln:

- Body und Footer beginnen jeweils nach einer Leerzeile.
- Footer folgen der Git-Trailer-ähnlichen Conventional-Commits-Syntax.
- `BREAKING CHANGE: <text>` und `BREAKING-CHANGE: <text>` sind synonym.
- Ein Breaking Change ist vorhanden, wenn der Header `!` oder ein Breaking-Footer enthält.
- Die Create-UI erzeugt für Breaking Changes standardmäßig sowohl `!` als auch einen erklärenden Footer.
- Der Validator akzeptiert gemäß Conventional Commits auch eine der beiden Formen allein.
- `revert` benötigt im Body oder Footer mindestens eine Commit-Referenz.

Beispiel:

```text
feat(ABC-123)!: replace the export contract

The endpoint now returns a versioned export resource.

BREAKING CHANGE: clients must consume the new resource envelope.
Refs: 0123456789abcdef
```

Quelle: [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/)

## 10. Commit-zu-Branch-Konsistenz

Auf `feature/*`, `fix/*`, `docs/*`, `refactor/*`, `chore/*`, `test/*`, `perf/*`, `hotfix/*` und `scratch/*` muss das Ticket im Commit-Header exakt dem Ticket im Branch entsprechen.

```text
Branch: feature/ABC-123-add-export-button
Gültig: feat(ABC-123): add export button
Ungültig: feat(ABC-124): add export button
```

Auf Shared Lines sind direkte Entwicklercommits unabhängig von ihrer Syntax verboten.

Lokale Synchronisations-Merges in einen bereits veröffentlichten Ticket-Branch müssen eine konforme Nachricht verwenden, zum Beispiel:

```text
chore(ABC-123): merge origin/develop
```

Hosting-seitig erzeugte Merge-Commits auf Shared Lines sind keine lokalen Ticket-Commits. CI klassifiziert sie über Parent-Anzahl und PR-Metadaten, statt einen normalen Ticket-Header vorzutäuschen.

Alle unterstützten lokalen Commit-Erstellungspfade besitzen einen bekannten
Branch-Kontext: `commit create` verwendet den aktuellen offiziellen
Ticket-Branch, Scratch-Transfers verwenden den aufgelösten offiziellen
Zielbranch und Synchronisations-Merges verwenden den aktuellen offiziellen
Branch. Key und Ticket-ID werden daher aus diesem Branch-Kontext abgeleitet und
sind keine frei wählbaren Commit-Eingaben. Ein nicht kanonischer Branch oder
Detached HEAD kann keinen governeten Commit-Erstellungspfad ausführen.

Die interaktive Erstellung besteht aus zwei Ebenen: zuerst wird eine der
kanonischen Commit-Familien aus Abschnitt 7 ausgewählt, danach wird nur die
Beschreibung für den Header eingegeben. Die UI zeigt den abgeleiteten Key und
die Ticket-ID als feste Information. Vollständige Nachrichten bleiben für
`commit validate` und den expliziten Kompatibilitätseingang verfügbar, weil
Body und Footer dort Teil des zu prüfenden Git-Texts sind.

## 11. Initial Commit

Die alte Fähigkeit, bei leerem Repository automatisch `Initial commit` zu erzeugen, wird nicht übernommen:

- sie gehört zum Repository-Bootstrap, nicht zur Branch-Erstellung
- sie besitzt keinen Ticket-Bezug
- sie verändert Zustand überraschend
- sie vermischt zwei Use Cases

Ein leeres Repository führt deshalb zu `REPOSITORY_HAS_NO_COMMITS` mit einer separaten Bootstrap-Anweisung.

## 12. Key-Präferenzen und Registry-Grenze

Der Benutzer darf syntaktisch gültige Keys in seiner lokalen Präferenzdatei speichern. Das beschleunigt die Auswahl, verleiht einem Key aber keine organisatorische Gültigkeit.

Heute:

```text
SyntaxOnlyKeyPolicy
→ Regex und Größenlimit
→ kein Netzwerk
→ keine Allowlist
```

Später:

```text
BundleKeyPolicy
→ syntaktische Prüfung
→ signiertes/versioniertes lokales JSON-Bundle
→ Status, Repo-Zulässigkeit und Staleness
→ CI bleibt bindend
```

Die beiden Adapter implementieren denselben Port; Branch-, Commit- und Workflow-Use-Cases ändern sich nicht.

## 13. Fehlerklassen

Mindestens folgende stabile Codes sind erforderlich:

- `TICKET_KEY_INVALID`
- `TICKET_NUMBER_INVALID`
- `TICKET_ID_INVALID`
- `BRANCH_FAMILY_INVALID`
- `BRANCH_SLUG_INVALID`
- `BRANCH_NAME_INVALID`
- `BRANCH_REF_INVALID`
- `BRANCH_BASE_INVALID`
- `BRANCH_ALREADY_EXISTS`
- `TICKET_BRANCH_ALREADY_EXISTS`
- `BRANCH_PUBLICATION_UNKNOWN`
- `SCRATCH_SOURCE_BRANCH_MISSING`
- `SCRATCH_TARGET_BRANCH_MISSING`
- `SCRATCH_TARGET_BRANCH_AMBIGUOUS`
- `SCRATCH_MERGE_EMPTY`
- `SHARED_LINE_MUTATION_FORBIDDEN`
- `REBASE_NOT_REQUIRED`
- `REBASE_AFTER_PUBLISH_FORBIDDEN`
- `FORCE_PUSH_FORBIDDEN`
- `COMMIT_TYPE_INVALID`
- `COMMIT_HEADER_INVALID`
- `COMMIT_DESCRIPTION_INVALID`
- `COMMIT_TICKET_MISMATCH`
- `BREAKING_CHANGE_INVALID`
- `WORKTREE_NOT_CLEAN`
- `REPOSITORY_HAS_NO_COMMITS`
- `POLICY_BUNDLE_MISSING`
- `POLICY_BUNDLE_STALE`

Jeder Fehler nennt beobachteten Wert, erwartetes Format, gültiges Beispiel und konkrete Behebung.

## 13.1 Pre-Push-Update-Protokoll

Der Pre-Push-Validator verarbeitet jede Git-Zeile nach diesem Vertrag:

```text
<local-ref> <local-object-id> <remote-ref> <remote-object-id>
```

Objekt-IDs müssen vollständige SHA-1- oder SHA-256-Werte sein. Für jede
`refs/heads/*`-Aktualisierung gilt:

- das tatsächliche Remote-Ziel wird geparst und validiert;
- `HEAD:main` ist deshalb genauso geschützt wie `main:main`;
- Mehrfach-Pushes werden Zeile für Zeile geprüft;
- Löschungen von Shared Lines werden blockiert;
- nicht-fast-forward Updates auf offiziellen Arbeitsbranches werden blockiert;
- bei einem ersten Push wird die Basisfrische gegen die konkrete lokale
  Objekt-ID geprüft, nicht gegen einen zufällig ausgecheckten Branch;
- nicht-Branch-Refs wie Tags werden als außerhalb der Branch-Governance
  klassifiziert und explizit als übersprungen berichtet.

## 14. Testkatalog

Die Implementierung muss mindestens folgende Äquivalenzklassen prüfen:

- jede der 13 Branch-Familien
- jede der 11 Commit-Typen
- minimale und maximale Key-Länge
- Keys mit Ziffern und ungültigem führendem Zeichen
- minimale und maximale Ticketnummer
- Slugs mit Ziffern
- führende, folgende und doppelte Bindestriche
- zusätzliche Slash-Segmente
- SemVer Core, Pre-Release und Build Metadata
- SemVer mit führenden Nullen und leeren Identifiern
- Support-Version mit zu vielen oder zu wenigen Segmenten
- Commit ohne Scope, mit falschem Ticket und mit falschem Typ
- Breaking Change mit `!`, Footer oder beiden
- Body und mehrere Footer
- CRLF- und LF-Nachrichten
- NUL- und Steuerzeichen
- unveröffentlichter Branch ohne Basisdelta
- unveröffentlichter Branch mit Basisdelta
- veröffentlichter Branch mit Basisdelta
- unbekannter Publication State
- direkte Mutation jeder Shared-Line-Klasse

Parser erhalten zusätzlich Fuzztests. Regex-Snapshots allein gelten nicht als ausreichender Nachweis.
