# CLI-Vertrag für `git-governance`

## 1. Aufruf und Benennung

Die Release-Binary heißt:

```text
git-governance
```

Git erkennt ausführbare Dateien nach dem Muster `git-<name>` als Subcommand. Deshalb sind beide Formen äquivalent:

```text
git governance branch create
git-governance branch create
```

Die primäre Dokumentation verwendet `git governance ...`.

Die alten Namen `mkbranch` und `mkcommit` werden nicht zur Zieloberfläche. Ein einzelnes Root-Kommando ist notwendig, weil Branches, Commits, Validierung, Konfiguration und Workflows dieselben Domain-Objekte und dieselbe Release-Version verwenden. Fachlich getrennte Use Cases bleiben getrennte Subcommands.

## 2. Globale Optionen

```text
--interactive auto|always|never   Standard: auto
--output human|json              Standard: human
--quiet                          nur notwendige Ausgabe
--color auto|always|never        Standard: auto
--accessible                     vereinfachte Screenreader-Oberfläche
--remote <name>                  Standard: origin
--repo <path>                    Standard: aktuelles Verzeichnis
--config <path>                  explizite Konfigurationsdatei
--quality-config <path>          explizite Repository-Quality-Gate-Datei
--pull-request-provider none|github
--dry-run                        Plan anzeigen, nichts mutieren
--yes                            bestätigbare Schritte freigeben
--timeout <duration>             Grenze für externe Prozesse
```

Regeln:

- `auto` startet Formulare nur bei vorhandenem TTY und fehlenden Pflichtwerten.
- `never` liest niemals interaktiv; fehlende Werte sind ein Nutzungsfehler.
- `always` scheitert klar, wenn kein TTY verfügbar ist.
- `always` ist mit `--output=json` unzulässig, weil JSON keine Prompts enthält.
- `--yes` ersetzt keine fehlenden fachlichen Werte.
- `--quiet` verändert weder Interaktion noch Validierung.
- `--color=auto` verwendet Farbe nur bei Terminalausgabe; `always` erzwingt
  ANSI-Farbe und `never` verwendet reine Textausgabe.
- Im JSON-Modus sind Prompts verboten; bei `--interactive=auto` verhält sich JSON deshalb wie `never`.
- Secrets werden weder über Flags noch über diese Konfigurationsdatei verwaltet.
- `--pull-request-provider=github` aktiviert ausschließlich den GitHub-Adapter;
  dieser löst eine GitHub-App-Sitzung oder einen Managed-Credential-Broker erst
  unmittelbar vor dem API-Aufruf auf.
- `--create-pull-request` ist ein expliziter Workflow-Flag, verlangt bei
  Publish-Workflows zusätzlich `--push` und erzeugt ohne Provider keinen
  stillen Fallback.

Interaktive Textfelder zeigen vor der Eingabe ihren vollständigen kanonischen
Vertrag. Bei einer fachlich ungültigen Eingabe bleibt die UI auf diesem Feld:
Sie zeigt den sicheren tatsächlichen Wert, die verletzte Regel, das erwartete
Format, ein gültiges Beispiel und die Korrektur und fragt denselben Wert erneut
ab. Es gibt kein Retry-Limit und keine Rückkehr an den Workflow-Anfang.

Schlägt ein Command erst nach akzeptierten Eingaben fehl, enthält die
Human-/JSON-Fehlerausgabe eine geordnete Eingabeübersicht. Die Übersicht umfasst
die im Command verwendeten Werte; sicherheitsmarkierte Werte werden redigiert.
Bei Git-Fehlern stehen `context` und `diagnostic` getrennt vom Feld
`actual`, damit Operationskontext nicht fälschlich als Benutzereingabe gilt.

`--quality-config` ist keine Spracheinstellung. Es zeigt auf einen
repository-lokalen, explizit vertrauenswürdigen JSON-Vertrag aus ausführbaren
Command-/Argumentarrays. Fehlt die Datei, lautet das Ergebnis
`qualityStatus=unconfigured`; es wird niemals als bestandener Build oder Lint
ausgegeben.

Ist eine gültige Konfiguration vorhanden, ermittelt `validate pre-push` den
Scope jedes Gates gegen die tatsächlichen Branch-Familien im Update-Stream.
Ein Gate ohne eigenen Scope erbt `defaults.includeFamilies`; ein Gate mit
`includeFamilies` beschränkt sich auf diese Familien und `excludeFamilies`
zieht danach Familien ab. Jedes dadurch berechtigte Gate läuft bei einem
Multi-Ref-Push höchstens einmal.

Ein finaler lokaler Quality-Lauf bindet seinen Nachweis an die ausgehenden
Revisionen, die Zielbasisrevision, den Remote, den Konfigurationsdigest, die
Gate-Auswahl, die Toolchain und einen sauberen Arbeitsbaum. Der Nachweis liegt
nur in lokaler Git-Metadatenauflösung unter
`git-governance.final-quality-evidence`, enthält keine Credentials und wird
nicht committed. `validate pre-push` prüft alle strukturellen Regeln weiterhin
immer. Es verwendet den Nachweis nur bei exakter, frischer Übereinstimmung;
bei fehlendem, abgelaufenem oder nicht passendem Nachweis läuft die
repo-definierte Vollsuite als Fallback einmal. Beschädigte oder unvollständige
Nachweise werden fail-closed abgewiesen.

Die empfohlene Default-Menge enthält alle offiziellen Arbeitsfamilien:
`feature`, `fix`, `docs`, `refactor`, `chore`, `test`, `perf` und `hotfix`.
`scratch` ist damit standardmäßig nicht ausgewählt, kann aber gezielt für ein
einzelnes leichtgewichtiges Gate eingeschlossen werden. Das ist keine globale
Sonderregel, sondern dieselbe Scope-Semantik wie für Dokumentations-, Test-,
Performance- oder Stress-Gates.

## 3. Command Tree

```text
git governance
├── branch
│   ├── list
│   ├── create
│   ├── validate
│   ├── merge-scratch
│   └── sync-base
├── commit
│   ├── create
│   └── validate
├── workflow
│   ├── ticket
│   │   ├── start
│   │   └── publish
│   ├── hotfix
│   │   ├── start
│   │   ├── validate-record
│   │   ├── verify-merge
│   │   ├── verify-delivery
│   │   ├── publish
│   │   ├── propagate
│   │   └── propagate-manifest
│   ├── release
│   │   ├── cut
│   │   ├── stabilize
│   │   ├── publish-stabilization
│   │   ├── align-promotion-base
│   │   ├── promote
│   │   ├── backmerge
│   │   ├── align-reconciliation-base
│   │   └── support
│   └── cleanup
├── validate
│   └── pre-push
├── auth
│   ├── login
│   │   └── github
│   ├── status
│   │   └── github
│   └── logout
│       └── github
├── config
│   └── key
│       ├── list
│       ├── add
│       ├── remove
│       └── set-default
├── policy
│   └── describe
├── completion
└── doctor
```

## 4. `auth`

```text
git governance auth login github
git governance auth status github
git governance auth logout github
```

`auth login github` ist ein expliziter interaktiver GitHub-App-Device-Flow.
Er verlangt Human-Output und ein echtes TTY, gibt nur Verifikations-URL und
Einmalcode aus und öffnet ausschließlich in diesem Command einen Browser.
`--interactive never` und JSON-Ausgabe sind dafür ungültig. Der lokale Client
speichert ausschließlich eine geschützte Refresh-Sitzung im nativen
Betriebssystem-Tresor; Access-Tokens, Refresh-Tokens, Private Keys und
Client-Secrets werden nie angezeigt, persistiert oder als Flag akzeptiert.

`auth status github` ist nicht interaktiv und gibt nur Host, Account,
Credential-Quelle sowie den Refresh-Ablaufstatus aus. `auth logout github`
löscht die lokale Tresor-Sitzung. Eine Gerätefluss-Sitzung wird nicht remote
widerrufen, weil ein lokaler Client keinen GitHub-App-Client-Secret besitzen
darf. Der vollständige Ablauf und der Brokervertrag stehen in
[`docs/usage/authentication.md`](../usage/authentication.md).

## 5. `branch list`

Zeigt alle Branch-Familien einschließlich Shared Lines und governance-gebundener Linien:

- `main`
- `develop`
- `release`
- `support`
- `feature`
- `fix`
- `docs`
- `refactor`
- `chore`
- `test`
- `perf`
- `hotfix`
- `scratch`

Jeder Eintrag enthält:

- Rolle
- Naming-Form
- zulässige Startbasis
- typisches PR-Ziel
- Protection-/Rewrite-Regel
- ob die Familie über `branch create` oder einen Workflow erzeugt wird

`branch list` ist die vollständige Informationsoberfläche. `branch create` zeigt nur auswählbare Familien für den konkreten Kontext und erklärt, warum andere Familien nicht direkt erzeugt werden dürfen.

## 6. `branch create`

### 5.1 Zweck

Erzeugt genau einen Branch aus einer explizit bestimmten Basis. Das Kommando validiert und mutiert Git; es enthält keinen Ticket-bis-PR-Gesamtworkflow.

### 5.2 Optionen

```text
--family feature|fix|docs|refactor|chore|test|perf|scratch
--key <KEY>
--ticket <NUMBER>
--slug <kebab-case>
--base <remote-ref>
--switch                        Standard: true
```

Regeln:

- Fehlt `--family` interaktiv, erscheint eine Auswahl mit Erklärung jeder Familie.
- Für reguläre Ticket-Familien ist die Standardbasis `<remote>/develop`.
- Nach `fetch --prune` muss diese Remote-Tracking-Basis existieren. Fehlt etwa
  `origin/develop`, wird die Erstellung als `BRANCH_BASE_INVALID` mit der
  fehlenden Basis abgelehnt, bevor Git einen Branch-Wechsel versucht.
- Vor einer echten Erstellung prüft das Kommando nach `fetch --prune`, ob
  bereits ein lokaler oder ausgewählter Remote-Tracking-Branch für dasselbe
  Ticket existiert. Ein zweiter regulärer offizieller Ticket-Branch wird
  abgewiesen.
- `hotfix` verlangt die real betroffene Basis und wird über `workflow hotfix start` erzeugt.
- `scratch` wird aus einem lokalen offiziellen Ticket-Branch desselben Tickets erzeugt; bei direkter Auswahl wird diese Basis abgefragt.
- `scratch` akzeptiert keine Remote-Tracking-Referenz, Shared Line, andere Scratch-Basis oder ticketfremde Basis.
- `release` und `support` verweisen auf governance-gebundene Workflow-Kommandos.
- `main` und `develop` sind keine auswählbaren Arbeitsbranches.
- Das Kommando führt nie `git add`, Commit, Amend oder Force Push aus.

### 5.3 Nicht-interaktives Beispiel

```text
git governance branch create \
  --interactive never \
  --family feature \
  --key ABC \
  --ticket 123 \
  --slug add-export-button \
  --output json
```

Der generierte Name ist:

```text
feature/ABC-123-add-export-button
```

### 5.4 Mutationsplan

Vor Bestätigung oder bei `--dry-run` wird ein Plan angezeigt:

```text
Remote aktualisieren: git fetch --prune origin
Basis prüfen: refs/remotes/origin/develop
Branch erzeugen: feature/ABC-123-add-export-button
Startpunkt: origin/develop
Arbeitsbranch wechseln: ja
```

### 5.5 `branch merge-scratch`

```text
git governance branch merge-scratch \
  [--branch scratch/<ticket>-<slug>] \
  [--target <official-ticket-branch>] \
  [--type <commit-family> --subject <description> | \
   --message "<complete Conventional Commit>"]
```

Ohne `--branch` ist der aktuelle Branch die Scratch-Quelle. Das Kommando
akzeptiert nur einen lokalen `scratch/*`-Branch und überträgt dessen Inhalt als
genau einen Squash-Commit in einen lokalen offiziellen Ticket-Branch.

Die Zielauflösung verwendet die Ticket-ID, nicht den Branch-Slug:

- `scratch/ABC-123-export-exploration` und
  `feature/ABC-123-add-export-button` gehören zusammen, obwohl ihre
  Beschreibungen unterschiedlich sind.
- Genau ein lokaler `feature`, `fix`, `docs`, `refactor`, `chore`, `test`,
  `perf` oder `hotfix`-Branch für dasselbe Ticket wird automatisch verwendet.
- Fehlt dieser lokale Branch, endet der Command mit
  `SCRATCH_TARGET_BRANCH_MISSING`; ein Remote-Tracking-Ref allein ist kein
  mergebarer lokaler Zielbranch.
- Bei mehreren lokalen Kandidaten ist `--target` Pflicht; der Command rät
  nicht zwischen Branch-Familien.

Interaktiv zeigt zuerst Scratch-Quelle und Zielbranch sowie den daraus
abgeleiteten, nicht editierbaren Ticket-Key und die Ticket-ID. Danach zeigt es
die vollständige kanonische Commit-Familienansicht (`build`, `chore`, `ci`,
`docs`, `feat`, `fix`, `perf`, `refactor`, `revert`, `style`, `test`) und fragt
anschließend nur die Beschreibung ab. Die Beschreibung ist genau der
nichtleere, ungepolsterte Text nach `: ` im erzeugten Header; sie darf keine
Steuerzeichen enthalten und höchstens 200 Unicode-Codepoints lang sein.

Für Automation sind die vorhandenen globalen Optionen maßgeblich:

```text
git governance --interactive never --yes branch merge-scratch \
  --type feat \
  --subject "add export button"
```

Die Ausführung wechselt auf das Ziel, führt `git merge --squash` aus und
erstellt den daraus erzeugten, ticket-konsistenten Conventional Commit. Sie
führt nie `git add .`, Push oder Scratch-Löschung aus. `--message` ist ein
vollständiger Kompatibilitätseingang für bestehende Automation und darf nicht
mit `--type` oder `--subject` kombiniert werden. Bei einem Konflikt bleibt der
normale Git-Konfliktzustand für explizite Auflösung erhalten. Der direkte
Command setzt keinen automatischen Merge fort; innerhalb von `workflow ticket
publish` wird derselbe Konfliktzustand dagegen über eine Retry-Auswahl
fortgesetzt, nachdem der Benutzer ihn aufgelöst und gestaged hat.

## 7. `branch validate`

```text
git governance branch validate [<branch-name>]
```

Ohne Argument wird der aktuelle Branch verwendet. Das Kommando prüft:

- vollständige Branch-Grammatik
- Value-Object-Regeln
- `git check-ref-format --branch`
- Family-spezifische Regeln
- optional Key-Policy/Bundle
- bei vorhandenem Repository den zulässigen Arbeitskontext

Es mutiert nichts und eignet sich für lokale Diagnose und CI.

## 8. `branch sync-base`

### 7.1 Zweck

Stellt fest, ob der aktuelle offizielle Arbeitsbranch Commits seiner tatsächlichen Zielbasis vermisst. Das Kommando ersetzt keine Merge Queue und führt keinen blinden Rebase aus. Ein optionales `--branch` ist eine explizite Erwartung an den aktuellen Branch und muss mit ihm übereinstimmen; das Kommando wechselt niemals still auf einen anderen Branch.

```text
git governance branch sync-base \
  --strategy check|auto|rebase|merge

git governance branch sync-base --resume
```

Für `--strategy merge` erzeugt die interaktive Oberfläche denselben
strukturierten Commit-Ablauf mit festem Branch-Ticket. Nicht-interaktive Aufrufe
verwenden `--merge-type <family>` und `--merge-subject <description>`;
`--merge-message` bleibt ausschließlich als vollständiger
Kompatibilitätseingang erhalten.

`--resume` setzt einen durch dieses Kommando pausierten Rebase oder Merge fort,
nachdem alle Konfliktpfade aufgelöst und explizit gestagt wurden. Der
Wiedereinstieg akzeptiert bewusst keine Strategie, kein Dry-Run und keine
Merge-Commit-Eingaben: Die aktive Git-Operation bestimmt die Fortsetzung. Vor
der Fortsetzung prüft das Kommando Branch-Grammatik, Publication State und die
aktive Operation; ein Merge wird zusätzlich nur fortgesetzt, wenn keine
unaufgelösten Konflikte mehr offen sind und er noch auf die gefetchte
Basisrevision zeigt. Nach der Fortsetzung werden Basis-Frische,
Branch-Validierung und die konfigurierten Quality Checks erneut ausgeführt.

### 7.2 Entscheidungslogik

1. aktuellen Branch und optionales `--branch` gegeneinander prüfen
2. tatsächliche Zielbasis bestimmen
3. sauberen Arbeitsbaum prüfen
4. `git fetch --prune <remote>`
5. Publication State prüfen
6. fehlende Basis-Commits bestimmen
7. Policy anwenden

| Zustand | Ergebnis |
|---|---|
| keine fehlenden Basis-Commits | `BASE_UP_TO_DATE`, keine Mutation |
| unveröffentlicht und Basisdelta vorhanden | Rebase zulässig |
| veröffentlicht und Basisdelta vorhanden | Rebase verboten; optional kontrollierter Merge |
| Publication State unbekannt | History Rewrite blockieren |
| Shared Line oder Scratch | eigener Family-Vertrag |

`auto` bedeutet nicht „immer mutieren“:

- unveröffentlicht: nur bei Delta rebasen
- veröffentlicht: ohne explizite Merge-Freigabe nur Handlungsplan ausgeben

Nach einer Mutation laufen Governance-Checks und konfigurierte Quality Checks erneut. Schlägt ein direkter `branch sync-base`-Rebase oder -Merge konfliktbedingt fehl, bleibt Git im normalen Rebase- beziehungsweise Merge-Zustand und wird nicht verborgen. Nach Auflösung und explizitem Staging setzt `branch sync-base --resume` die pausierte Operation governet fort. Im `workflow ticket publish` wird derselbe Zustand zusätzlich als Retry-Schritt dargestellt: Nach Auflösung und Staging setzt Retry den bestehenden Rebase fort.

## 9. `commit create`

### 8.1 Optionen

```text
--type build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test
--ticket <KEY-NUMBER>            Kompatibilitätsprüfung gegen den Branch
--subject <text>                 Commit-Beschreibung
--body <text>
--breaking
--breaking-description <text>
--footer <token=value>           wiederholbar
--stage <path>                   wiederholbar
--push                           Standard: false
```

### 8.2 Defaults und Ableitungen

- Das Ticket wird auf einem Ticket-Branch aus dem Branch-Namen abgeleitet.
- Ein explizites `--ticket` muss exakt zum Branch passen und ändert den
  abgeleiteten Scope nicht.
- Der Commit-Typ wird aus der Branch-Familie vorgeschlagen, aber nicht blind erzwungen.
- `feature` schlägt `feat`, `fix` und `hotfix` schlagen `fix` vor.
- `docs`, `refactor`, `chore`, `test` und `perf` schlagen den gleichnamigen Typ vor.
- Interaktiv zeigt die feste Branch-, Key- und Ticket-ID-Kontextzeile, dann die
  kanonische Commit-Familienansicht und zuletzt die Beschreibung. Key und
  Ticket sind in diesem Ablauf nicht auswählbar.
- Das Kommando prüft, ob Änderungen gestaged sind.
- Ohne `--stage` wird niemals automatisch `git add .` ausgeführt.
- `--stage` akzeptiert explizite Pfade und zeigt sie vor der Mutation.
- `--push` ist optional und läuft durch dieselbe Pre-Push-Validierung wie Lefthook.

### 8.3 Breaking Changes

Bei `--breaking` erzeugt die UI standardmäßig:

```text
feat(ABC-123)!: replace the export contract

BREAKING CHANGE: clients must consume the new resource envelope.
```

Der Benutzer erhält eine Erklärung:

- Breaking bedeutet inkompatible öffentliche Vertragsänderung.
- Der Marker darf nicht für interne Refactors missbraucht werden.
- Die Beschreibung muss konkrete Migrationsauswirkung nennen.

### 8.4 Amend und Force Push

`commit create` bietet kein Amend-Flag. Vor dem ersten Push wäre ein lokales Amend gemäß Referenz-Governance zwar grundsätzlich zulässig, ist aber kein notwendiger Produkt-Use-Case. Nach dem ersten Push ist Amend als Routine verboten. Force Push wird von keinem Kommando angeboten.

## 10. `commit validate`

```text
git governance commit validate --message-file <path>
git governance commit validate --message <text>
```

Prüfungen:

- Header-Grammatik
- Commit-Typ
- Ticket-ID
- Beschreibung
- Body-/Footer-Struktur
- Breaking-Change-Semantik
- Ticketkonsistenz zum aktuellen Branch
- Shared-Line-Regeln
- optionale Key-Policy

Für `commit-msg` wird immer `--message-file` verwendet. Die Datei wird begrenzt gelesen; NUL und unzulässige Kontrollzeichen werden abgewiesen.

## 11. `workflow ticket start`

### 10.1 Zweck

Startet reguläre Ticket-Arbeit und endet auf dem offiziellen oder optionalen Scratch-Branch.

```text
git governance workflow ticket start \
  --family feature \
  --key ABC \
  --ticket 123 \
  --slug add-export-button \
  --scratch
```

### 10.2 Ablauf

1. Repository und Git-Version prüfen.
2. Arbeitsbaum und laufende Git-Operationen prüfen.
3. Ticket-Eingaben validieren.
4. `git fetch --prune origin`.
5. offiziellen Branch direkt von `origin/develop` erzeugen.
6. optional Scratch-Frage mit Erklärung anzeigen.
7. bei Zustimmung `scratch/<ticket>-<scratch-slug>` vom offiziellen Branch erzeugen.
8. auf dem gewählten Branch enden.

`--scratch` erstellt ausdrücklich eine private Exploration. Ohne Flag fragt der
interaktive Modus nach; nicht-interaktiv wird ohne Flag kein Scratch-Branch
angelegt.

### 10.3 Scratch-Erklärung in der UI

Die UI muss vor der Auswahl sinngemäß anzeigen:

```text
Scratch-Branches sind private, kurzlebige Explorationslinien.
Verwende Scratch nur, wenn Lösungsweg oder Experiment unsicher sind.
Erstelle keinen Pull Request aus Scratch und teile ihn nicht als offiziellen
Arbeitsbranch. Übernimm stabile Ergebnisse später kontrolliert per Squash
oder Cherry-Pick in den offiziellen Ticket-Branch.
```

## 12. `workflow ticket publish`

Dieses Kommando wird nach Entwicklung und lokalen Tests aufgerufen. Es ist kein automatisch fortlaufender Teil von `ticket start`.

```text
git governance workflow ticket publish \
  --push --draft
```

Ablauf:

1. aktuellen Branch auflösen
2. bei `scratch/*` den lokalen offiziellen Zielbranch über die Ticket-ID
   bestimmen, Scratch, Ziel und Squash-Commit anzeigen und bestätigen
3. bei bestätigtem Scratch-Pfad denselben `branch merge-scratch`-Use-Case
   ausführen und auf dem offiziellen Branch fortsetzen
4. offiziellen Ticket-Branch und sauberen Zustand prüfen
5. Branch- und Commit-Serie validieren
6. Basisfrische prüfen
7. bei unveröffentlichtem Branch und Basisdelta rebasen
8. nach einem Rebase Branch-/Policy-Prüfung und Commit-Serie erneut ausführen
9. projektdefinierte Vollsuite auf dem finalen Publish-Kandidaten ausführen
   und den revisionsgebundenen lokalen Nachweis erzeugen
10. in der interaktiven Ansicht anzeigen, ob ein Rebase erfolgt ist oder warum
    er nicht erfolgt ist
11. bei einem pausierten Scratch-Squash oder Rebase Konflikte lösen und
    stagen; die passende Retry-Auswahl setzt exakt diese Git-Operation fort,
    statt den Workflow von vorn zu starten
12. vor dem ersten Push interaktiv bestätigen oder `--push` nicht-interaktiv
    explizit setzen
13. Pre-Push-Policy gegen die tatsächliche Aktualisierung prüfen und den
    Nachweis nur bei exakter Bindung wiederverwenden
14. nach einem Push bei konfiguriertem Provider interaktiv die PR-Erstellung
    bestätigen; nicht-interaktiv `--create-pull-request` explizit setzen;
    ohne Provider nur den providerneutralen PR-Intent ausgeben

Für einen Scratch-Start benötigt der nicht-interaktive Modus eine
Commit-Familie, eine Beschreibung und die bestehende Mutationsfreigabe:

```text
git governance --interactive never --yes workflow ticket publish \
  --type feat \
  --subject "add export button" \
  --push
```

`--target <official-ticket-branch>` ist nur auf `scratch/*` zulässig und löst
manuelle Mehrdeutigkeit auf. Auf einem offiziellen Branch bleiben `--target`
und die Scratch-Transfer-Eingaben `--type`, `--subject` und `--message`
ungültig. `--message` bleibt als vollständiger Kompatibilitätseingang
erhalten und darf nicht mit den strukturierten Eingaben kombiniert werden.

Ohne Provider-Adapter wird kein Hosting-API-Aufruf erfunden. Die JSON-Ausgabe
ist eine stabile Übergabeoberfläche für GitHub-, GitLab-, Bitbucket- oder
andere Adapter. Eine Benutzerbestätigung kann deshalb nur dann einen echten PR
erzeugen, wenn ein solcher Adapter zur Laufzeit konfiguriert ist.

Für einen GitHub-PR setzt die Automation `--pull-request-provider github`,
`--push` und `--create-pull-request`. Sie verwendet eine bereits bestehende
GitHub-App-Session oder einen CI-Credential-Broker; sie startet nie einen
Browser und akzeptiert keinen statischen GitHub-Token. Der Adapter leitet Host,
Owner und Repository aus dem ausgewählten Git-Remote ab, prüft die exakte
Repository-Autorisierung und gibt einen bereits offenen gleichartigen PR
idempotent zurück.

Nach manueller Konfliktauflösung und Staging ist die Fortsetzung auch ohne TTY
verfügbar:

```text
git governance --interactive never --yes workflow ticket publish \
  --branch feature/ABC-123-add-export \
  --resume --push
```

Auf `scratch/*` bleiben die ursprünglichen `--type`/`--subject`- oder
`--message`-Eingaben erforderlich; bei Mehrdeutigkeit bleibt `--target` Pflicht.

## 13. `workflow hotfix start`

Pflichtoptionen:

```text
--key <KEY>
--ticket <NUMBER>
--slug <slug>
--affected-line main|release/<semver>|support/<major.minor>
```

Ablauf:

1. betroffene Linie validieren
2. `fetch --prune`
3. Remote-Linie und sauberen Arbeitsbaum prüfen
4. `hotfix/<ticket>-<slug>` direkt von der betroffenen Remote-Linie erzeugen
5. Ziel-PR auf dieselbe Linie festlegen

Ein Hotfix startet nie automatisch von `develop`.

### 13.1 `workflow hotfix publish`

```text
git governance workflow hotfix publish \
  --affected-line main|release/<semver>|support/<major.minor> \
  --push
```

Der Befehl verlangt die tatsächlich betroffene Linie erneut, validiert den
Hotfix gegen dieselbe Basis und erzeugt den PR-Intent auf genau diese Linie.
Ein Hotfix wird niemals stillschweigend nach `develop` umgeleitet.
`--create-pull-request` ist nur zusammen mit `--push` zulässig. Nach
manueller Rebase-Konfliktauflösung setzt `--resume` dieselbe Hotfix-Publikation
ohne interaktive Eingaben fort.

### 13.2 `workflow hotfix validate-record`

```text
git governance workflow hotfix validate-record \
  --branch hotfix/<KEY-NUMBER>-<slug> \
  [--record .git-governance/hotfix-release-records/<KEY-NUMBER>.json]
```

Der read-only Befehl lädt nur den Ticket-gebundenen JSON-Record aus dem
kontrollierten Repository-Verzeichnis. Er verlangt Schema-Version 1, einen
Main-Hotfix, einen stabilen Patch-Nachfolger des vorherigen Tags, die exakte
Hotfix-PR-Bindung, einen geordneten vollständigen SHA-Manifest und deklarierte
zusätzliche Propagationsziele.

### 13.3 `workflow hotfix verify-merge` und `verify-delivery`

```text
git governance --pull-request-provider github workflow hotfix verify-merge \
  --branch hotfix/<KEY-NUMBER>-<slug>

git governance --pull-request-provider github workflow hotfix verify-delivery \
  --branch hotfix/<KEY-NUMBER>-<slug>
```

`verify-merge` prüft den gemergten Same-Repository-Main-PR, den exakten
GraphQL-Merge-Commit, das geordnete Commit-Manifest und die Abwesenheit des
neuen immutable Tags. `verify-delivery` prüft zusätzlich, dass der Tag exakt
auf den Merge zeigt, ein nicht-draft GitHub Release mit Payload, Checksums,
SBOM und Sigstore-Bundle existiert und der Artifact-Workflow erfolgreich war.
Beide Befehle sind read-only und erhalten ihre kurzlebige Identität nur im
geschützten Controller.

### 13.4 `workflow hotfix propagate`

```text
git governance workflow hotfix propagate \
  --target-line main|develop|release/<semver>|support/<major.minor> \
  --commit <sha> \
  --push
```

Der Befehl erzeugt einen kontrollierten `fix/*`-Branch aus der Ziel-Linie,
führt `git cherry-pick -x <sha>` aus und bereitet den PR gegen diese Ziel-Linie
vor. Damit bleibt die Herkunft eines Forward- oder Backports nachweisbar.
Bei einem pausierten Cherry-Pick löst der Benutzer die Konflikte und setzt
anschließend mit `--source`, `--target-line`, dem erzeugten `--branch` und
`--resume` fort. `--commit` ist beim Fortsetzen nicht erneut erforderlich.

### 13.5 `workflow hotfix propagate-manifest`

```text
git governance workflow hotfix propagate-manifest \
  --source hotfix/<KEY-NUMBER>-<slug> \
  --target-line develop|release/<semver>|support/<major.minor> \
  [--publish]
```

Der Befehl akzeptiert ausschließlich eine im geprüften Record deklarierte
Ziel-Linie. Er erzeugt einen workflow-managed `fix/*`-Kandidaten, speichert
seinen lokalen Resume-Cursor in Git-Metadaten, appliziert die deklarierte
SHA-Serie in der angegebenen Reihenfolge und führt die Quality-Suite aus. Bei
Konflikt verlangt `--resume` den identischen Source-, Target- und
Kandidatenbranch. `--push` und `--create-pull-request` bleiben absichtlich
nicht verfügbar. `--publish` ist nur innerhalb des geschützten
Hotfix-Propagation-Publisher-Controllers zulässig: Er verlangt die dedizierte
Broker-Workload-Identität, erzeugt den Kandidaten aus der erklärten Ziel-Linie,
prüft ihn erneut, pusht nur den nicht-shared `fix/*`-Branch und erstellt dessen
PR. Ohne diese serverseitige Boundary endet `--publish` fail-closed; lokale
Kandidaten bleiben nicht veröffentlichend.

## 14. Release-Kommandos

### 13.1 `workflow release cut`

```text
git governance workflow release cut \
  --version 2.8.0
```

Das Kommando:

- verlangt eine explizite Governance-Bestätigung
- prüft die lokale Release-Anfrage und erzeugt einen maschinenlesbaren Intent
  für den geschützten Release-Request-Controller
- erstellt, wechselt oder pusht keinen lokalen `release/*`-Branch
- lehnt einen normalen `--dispatch` außerhalb eines Dry-Runs ab; ein
  ungebundener lokaler CLI-Aufruf darf den Protected-Line-Executor nicht
  starten
- bleibt ohne `--dispatch` absichtlich bei einem Intent; dieser kann keinen
  nachfolgenden Release-Zustand beweisen
- erklärt die danach erlaubte begrenzte Stabilisierung

### 13.1.1 Controller-interne Release-Request- und Finalizer-Kommandos

Die folgenden Unterkommandos sind keine lokalen Normaloperationen. Sie
verlangen den expliziten, kurzlebigen GitHub-Actions-Controller-Modus und ein
job-scoped `GITHUB_TOKEN`:

```text
workflow release request
workflow release execute-request
workflow release finalize-request
```

`request` bindet Ticket, Operation, Version, Quellref, exakte Quell-SHA,
Zielref, Requester, Parent-Run, Ablaufzeit und Idempotenz an einen dauerhaften
Provider-Record. `execute-request` akzeptiert ausschließlich dessen
`request_id` und den korrelierten Executor-Run. `finalize-request` prüft
read-only den Executor und die tatsächliche Remote-Ref und schreibt nur den
Auditstatus. `--recovery` ist ausschließlich für einen bestehenden
`verification_pending`-Record zulässig; er startet niemals eine neue Mutation.

Der reguläre Ablauf wird durch `release-control.yml` mit
`operation=release-request` und die getrennten Environments
`release-request` und `release-execution` ausgelöst. Nur `verified` ist ein
vollständiger Protected-Line-Cut-Nachweis.

Die CLI modelliert keine generische `release`-Sammellane. Der geschützte
Workflow bindet Request, Execution, Credential-Verifikation, reguläre
Delivery, Reconciliation, Hotfix-Delivery und Hotfix-Propagation jeweils an
eine eigene funktionale Lane. Nur die Workflow-Lane mit dem konkreten
Credential-Issuer-Bedarf erhält deren lane-spezifische OIDC/WIF- und
Invocation-Variablen; die lokalen Controller-Kommandos akzeptieren keine
solchen Credentials als Eingaben.

### 13.2 `workflow release stabilize`

```text
git governance workflow release stabilize \
  --release release/<semver> \
  --kind blocker|docs|release-prep \
  --key <KEY> --ticket <NUMBER> --slug <kebab-case>
```

Nur die drei genannten Kategorien sind auf einer eingefrorenen Release-Linie
zulässig. Neue Features, allgemeine Refactors und themenfremde Tickets besitzen
keine auswählbare Stabilisierungskategorie.

### 13.3 `workflow release publish-stabilization`

Dieser Befehl validiert einen Stabilisierung-Branch gegen
`origin/release/<semver>` und erzeugt seinen PR-Intent auf dieselbe
Release-Linie. `--create-pull-request` verlangt `--push`; nach manueller
Rebase-Konfliktauflösung setzt `--resume` die vorhandene Stabilisierung fort.

### 13.4 `workflow release align-promotion-base`

```text
git governance --pull-request-provider github workflow release align-promotion-base \
  --release release/<semver> \
  [--branch chore/<KEY-NUMBER>-<slug>] \
  [--resume] \
  [--push --create-pull-request]
```

Der Befehl ist ausschließlich für eine `chore/*`-Release-Preparation-Branch
zulässig, die durch `workflow release stabilize --kind release-prep` aus der
angegebenen `release/<semver>`-Linie erzeugt wurde. Er prüft die gespeicherte
Release-Basis, verlangt die ausgecheckte Branch und führt ausschließlich dort
einen ticket-scoped Merge von `origin/main` aus. Nach der Quality-Suite kann
er die Working-Branch pushen und ihren PR zurück auf die Release-Linie
erstellen. Damit erfüllt ein striktes Main-Ruleset die Aktualitätsprüfung,
ohne `Update branch`, Rebase oder Direktmutation einer Shared Line.

Bei einem Konflikt bleibt die laufende Merge-Operation auf derselben
nicht-shared Preparation-Branch. `--resume` verlangt eine aktive
konfliktfreie Merge-Operation mit explizit gestagten Resolution-Pfaden,
vergleicht `MERGE_HEAD` mit dem nach Fetch aktuellen `origin/main`, setzt nur
diesen Merge fort und prüft vor Quality, Push und PR erneut die Main-Basis.
Hat sich Main weiterentwickelt, endet der Kandidat fail-closed. `--resume` ist
mit `--dry-run` nicht zulässig.

### 13.5 `workflow release promote`

Dieser Befehl erzeugt den providerneutralen PR-Intent:

```text
release/<semver> -> main
```

Tagging und Artefakterstellung folgen erst nach dem geschützten Merge in der
Release-Pipeline. Der CI-Workflow erzeugt `v<semver>` direkt auf dem
Merge-Commit und startet anschließend den Artefaktworkflow für genau diesen
unveränderlichen Tag. Ein echter Provider-PR ist nur mit
`--pull-request-provider github --create-pull-request` und der expliziten
Mutationsfreigabe möglich.

### 13.6 `workflow release backmerge`

Erzeugt keine stillen Direktcommits und keinen leeren Ritual-PR. Außerhalb
eines Dry-Runs verlangt es einen konfigurierten Release-Lifecycle-Provider,
der vor der PR-Erzeugung nachweist:

1. `release/<semver> -> main` wurde gemergt;
2. `v<semver>` zeigt exakt auf diesen Promotion-Merge;
3. die erforderliche GitHub Release- und Artefakt-Delivery ist erfolgreich;
4. ein effektiver Release-Delta fehlt noch in `develop`.

```text
release/<semver> -> develop
```

Ist der Delta vorhanden, liefert `status=required` und erzeugt bei
`--create-pull-request` den PR. Ist kein effektiver Delta vorhanden, liefert
das Kommando `status=not-required`, den Delivery-Nachweis und keinen PR.
Ein echter Provider-PR folgt derselben expliziten GitHub-Adapter-Konfiguration
wie die Promotion.

### 13.6.1 `workflow release align-reconciliation-base`

Dieser Workflow behandelt ausschließlich einen Backmerge, dessen Ziel-Policy
einen aktuellen Pull-Request-Head verlangt. Er akzeptiert nur eine aktuell
ausgecheckte, ticketgebundene `chore/*`-Preparation-Branch mit gespeicherter
Workflow-Basis `origin/release/<semver>`.

Der Workflow prüft vollständige Release-Delivery und effektiven Delta,
verifiziert die aktuelle `origin/develop`-Basis, merged diese ausschließlich
in die Preparation-Branch, führt Quality-Gates aus und publiziert optional
einen Merge-Commit-PR nach `develop`.

Bei einem Konflikt bleibt der normale Merge-Zustand in der nicht-shared,
ticketgebundenen Preparation-Branch erhalten. Nach expliziter Resolution und
Staging der genauen Konfliktpfade setzt `--resume` ausschließlich diesen Merge
fort; der Befehl übernimmt keine automatische `ours`/`theirs`-Entscheidung.

```text
git governance workflow release align-reconciliation-base \
  --release release/<semver> \
  [--branch chore/<KEY-NUMBER>-<slug>] \
  [--resume] \
  [--prepared] \
  [--push --create-pull-request]
```

`--resume` und `--prepared` schließen sich aus:

- `--resume` verlangt einen aktiven, konfliktfreien und vollständig gestagten
  Merge in derselben lokalen Preparation-Branch. Nach dem Merge muss
  `origin/develop` weiterhin enthalten sein; sonst wird die Kandidatenbranch
  fail-closed abgewiesen.
- `--prepared` ist der serverseitige Recovery-Eingang für eine bereits
  konfliktbereinigte, gepushte Preparation-Branch. Fehlende lokale
  Workflow-Metadaten sind nur dann zulässig, wenn die CLI unabhängig beweist,
  dass HEAD exakt den unveränderten Release-Ref als ersten und den aktuellen
  Develop-Ref als zweiten Merge-Parent besitzt.

Ein beliebiger Branch-Name genügt nie als Recovery-Nachweis. Der geschützte
`reconciliation-resume`-Workflow validiert Ticket, Slug, Branch-Grammatik,
Merge-Provenance, Delivery und Quality erneut, bevor er einen PR nach
`develop` erstellt.

Die serverseitige Veröffentlichung verwendet eine dedizierte
Reconciliation-Publisher-Identität aus dem `release-reconciliation`
Environment. Der lokale Resolution-Workspace erhält diese Identität nicht. Die
Publisher-Identität ist auf den validierten `chore/*` Kandidaten und dessen PR
beschränkt; sie besitzt keinen Ruleset-Bypass und keine direkte Shared-Line-
Mutation-Berechtigung.
`release/<semver>` bleibt unverändert. Im Dry-Run führt kein CLI-Workflow
einen Fetch, Merge, Push, Provider-Preflight oder Provider-Publish aus.

### 13.7 `workflow release support`

`support/<major.minor>` darf nur angefordert werden, wenn die aktuell
gefetchte `origin/main`-Revision einen passenden
`v<major.minor.patch>`-Release-Tag trägt. Die CLI erzeugt einen Intent. Die
normale Ausführung erfolgt anschließend nur über den geschützten
Release-Request-Controller mit `kind=support`; der Request bindet die
freigegebene Main-Revision, bevor der separate Execution-Workflow die
Remote-Support-Linie erzeugen darf.

### 13.8 `workflow cleanup`

`workflow cleanup` löscht niemals Remote-Branches. Remote-Löschung und
Lifecycle-Nachweise gehören zu GitHub, GitLab oder CI:

- Ticket- und Hotfix-Remote-Branches werden nach dem passenden PR-Merge durch
  die Hosting-Plattform entfernt.
- Ein Release-Remote-Branch bleibt bis Main-Promotion, Tag/Artefakt-Workflow
  und abgeschlossener Reconciliation nach `develop` erhalten. Ein
  `status=not-required` ist dabei ein gültiger Abschluss ohne leeren
  Backmerge-PR; danach löscht ihn Hosting-Automation oder CI.
- `main`, `develop`, `release/*` und `support/*` sind nie lokale
  CLI-Cleanup-Ziele.

Die CLI erlaubt ausschließlich lokale `scratch/*`-Bereinigung und entfernt
deren lokale Workflow-Basis-Metadaten. Offizielle Ticket-, Hotfix-, Release-
und Support-Branches sind keine lokalen CLI-Cleanup-Ziele. Das Kommando
behauptet nicht, einen Hosting-Merge oder Forward-/Backport-Abschluss beweisen
zu können.

## 15. `validate pre-push`

Dieses Kommando ist die Lefthook- und manuelle Pre-Push-Oberfläche.

```text
git governance validate pre-push \
  --remote origin
```

Es liest die von Git gelieferte Ref-Liste begrenzt von stdin und prüft:

- jede vierfeldrige Git-Ref-Aktualisierung statt nur den aktuell ausgecheckten Branch
- Ziel-Branch-Grammatik und Key-Policy
- Shared-Line-Push-Verbot
- Commit-Ticket-Konsistenz
- Löschungen, nicht-fast-forward-/Rewrite-Versuche und Mehrfach-Updates
- Bundle-Präsenz und -Frische, sobald der Bundle-Adapter aktiv ist
- Basislinien-Frische vor dem ersten Push
- finalen lokalen Quality-Nachweis nur bei passender ausgehender Revision,
  Basis, Konfiguration, Toolchain, Gate-Auswahl und frischem sauberen
  Arbeitszustand

Der Validator führt nie selbst Rebase oder Merge aus. Fehlt ein passender
finaler Nachweis, läuft die konfigurierte Vollsuite als lokaler Raw-Push-
Fallback. Er blockiert mit einer konkreten, policy-konformen Handlungsanweisung.

## 16. Konfigurationskommandos

```text
git governance config key add PLATFORM2
git governance config key list
git governance config key set-default PLATFORM2
git governance config key remove PLATFORM2
```

Regeln:

- nur syntaktisch gültige Keys werden gespeichert
- Speicherung ist dedupliziert und plattformgerecht wiederherstellbar
- ein gespeicherter Key gilt nicht automatisch als Registry-zugelassen
- Ticketnummern werden nicht als globaler Default gespeichert
- Commits leiten das Ticket aus dem aktuellen Branch ab

## 17. `policy describe`

Gibt die aktive ausführbare Policy versioniert aus:

```text
git governance policy describe --output json
```

Enthalten sind:

- Policy-Schema-Version
- Branch-Familien
- Commit-Typen
- Regex-/Grammatik-IDs
- aktive Key-Policy (`syntax-only` oder `bundle`)
- technische Limits
- Fehlercodes

Dokumentations- und Conformance-Tests verwenden diese Ausgabe, damit keine zweite Regex-Wahrheit in Hooks oder Beispielen entsteht.

## 18. `doctor`

Read-only-Diagnose:

- unterstütztes Betriebssystem und Architektur
- Git vorhanden und Mindestversion erfüllt
- Repository erkannt
- Remote vorhanden, ohne dessen URL in der Human-Ausgabe offenzulegen
- Git-Transportauthentifizierung durch einen nicht-interaktiven
  `push --dry-run --porcelain` des aktuellen Branches
- Benutzerkonfiguration lesbar
- Lefthook vorhanden
- Lefthook-Konfiguration vorhanden
- Policy-Bundle-Status, wenn aktiviert
- keine laufende Merge-/Rebase-/Cherry-Pick-Operation

Eine fehlende Git-Transportauthentifizierung ist ein klassifizierter Fehler,
nicht nur ein warnender Check. Der Dry-Run kontaktiert den Remote, ändert aber
keine Remote-Referenz, überspringt Git-Hooks und darf keinen Credential-Prompt
öffnen. GitHub-App-API-
Sitzungen bleiben davon getrennt und werden durch `auth status github` sowie
den Publish-Preflight geprüft.

`doctor` installiert, repariert oder mutiert nichts ohne ein separates explizites Kommando.

## 19. Human- und JSON-Ausgabe

### 19.1 Human

Nach einem erfolgreichen `git fetch --prune <remote>` beginnt die interaktive
Human-Abschlussmeldung mit:

```text
🟢 Remote references fetched and stale references pruned from <remote> before this operation.
```

Die Anzeige wird nur nach einem tatsächlich erfolgreich abgeschlossenen Fetch
ausgegeben, nicht bei `--dry-run`, `--interactive=never`, JSON-Ausgabe oder
`--quiet`. Fetch aktualisiert konfigurierte Remote-Tracking-Referenzen; ein
lokaler Branch wird dadurch nicht gepullt oder gewechselt.

Fehlerdarstellung:

```text
Fehler [COMMIT_TICKET_MISMATCH]

Tatsächlicher Wert:
  ABC-124

Was ist falsch?
  Der Commit verwendet ABC-124, der aktuelle Branch gehört zu ABC-123.

Wie muss es sein?
  Alle Commits eines offiziellen Ticket-Branches verwenden dessen Ticket-ID.

Gültiges Beispiel:
  feat(ABC-123): add export button

Behebung:
  Verwende ABC-123 oder wechsle auf den zum Commit gehörenden Branch.
```

### 19.2 JSON

```json
{
  "schemaVersion": 1,
  "ok": false,
  "error": {
    "code": "COMMIT_TICKET_MISMATCH",
    "category": "governance",
    "field": "ticket",
    "actual": "ABC-124",
    "expected": "ABC-123",
    "rule": "commit ticket must equal branch ticket",
    "example": "feat(ABC-123): add export button",
    "remediation": "use ABC-123 or switch branches"
  }
}
```

JSON-Feldnamen und Exitcodes sind öffentliche Verträge und werden kompatibel versioniert.

## 20. Interne Komposition

Delivery-Adapter sammeln Eingaben und erzeugen Commands. Workflows rufen Application Services direkt auf:

```text
Cobra/Huh
→ StartTicketWorkflow
  → FetchRemote
  → CreateBranch
  → optional CreateScratchBranch
→ Reporter
```

Nicht zulässig:

```text
workflow command
→ startet git-governance branch create als Kindprozess
→ parst dessen Textausgabe
```

Nur externe Consumer und Automation verwenden die CLI-Oberfläche.

## 21. Übernahme aus dem bisherigen Tool

| Bestehende Fähigkeit | Zielentscheidung |
|---|---|
| interaktive Branch-Auswahl | übernehmen, aber vollständige kanonische Taxonomie |
| interaktive Commit-Typ-Auswahl | übernehmen und vervollständigen |
| Ticket-Key-Historie | als OS-konforme Benutzerpräferenz übernehmen |
| Ticketnummer-Eingabe | übernehmen, aber nicht global wiederverwenden |
| Branch-Slug-Eingabe | übernehmen mit strengerem kebab-case |
| Bestätigung vor Mutation | übernehmen plus `--dry-run` |
| Wechsel auf neuen Branch | übernehmen |
| optionaler Push nach Commit | nur explizit und durch Pre-Push-Validierung |
| Checkout und Pull von `develop` | durch Fetch und direkte Basisreferenz ersetzen |
| Dev-/User-Suffixe im Branchnamen | verwerfen |
| automatischer Initial Commit | verwerfen |
| eigene Hook-Kopierskripte | durch Lefthook-Standard ersetzen |
| parallele PowerShell-/Shell-Logik | vollständig verwerfen |
