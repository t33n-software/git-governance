# ADR-0004: Trusted Release Reconciliation Control

- Status: Accepted
- Datum: 2026-08-01
- Geltungsbereich: Ausführung einer strikten
  `release/<semver> -> develop`-Reconciliation
- Entscheider: Release-Governance

## Kontext

Eine ausgelieferte `release/<semver>`-Ref bleibt nach Promotion, Tag und
Delivery unverändert. Wenn `develop` seit der Promotion neue Commits enthält
und eine aktuelle Pull-Request-Basis erzwingt, wird eine ticketgebundene
Preparation-Branch aus der Release-Ref benötigt.

Der Preparation-Branch enthält absichtlich nicht automatisch die neueste
CLI-Implementierung. Ein Aufruf von `go run ./cmd/git-governance` aus diesem
Worktree würde daher nicht zuverlässig den kontrollierten
Reconciliation-Workflow enthalten. Ein manueller Merge wäre keine zulässige
Alternative.

## Entscheidung

Die geschützte Release-Control-Workflow-Datei auf `main` baut vor jedem
Reconciliation-Branch-Wechsel einen unveränderlichen CLI-Binary aus dem
vertrauenswürdigen Main-Control-Plane-Commit.

```text
trusted main control-plane source
  -> build immutable release-control binary
  -> create release-derived preparation branch
  -> execute binary against that branch
  -> merge current develop only into preparation branch
  -> quality gates and reviewed PR to develop
  -> or fail-closed conflict manifest and controlled recovery
```

Die Workflow-Operation `reconciliation-align` benötigt Release-Linie,
Ticket-Key, Ticket-Nummer und Slug. Sie erhält eine kurzlebige
Broker-Workload-Identität, konfiguriert den Git-Transport nur im ephemeren
Runner mit einem maskierten Installation-Token und entfernt diese Konfiguration
am Ende des Jobs.

Nach erfolgreicher Tag-, Artefakt-, Attestations- und Release-Delivery wird
die Reconciliation im regulären Zielpfad automatisch aus demselben
Delivery-Lifecycle gestartet. Der Controller verifiziert alle Delivery-Fakten
erneut und erstellt nur bei effektivem Delta den Preparation-Branch-PR nach
`develop`. Ein manueller `workflow_dispatch`-Start ist ausschließlich
Incident-, Retry- und Recovery-Fallback.

Der Binary führt erst `workflow release stabilize --kind release-prep` und
anschließend `workflow release align-reconciliation-base` aus. Der Broker
bleibt ausschließlich Token-Aussteller; er erzeugt weder Branches noch Pull
Requests.

Die Release-Automation-Identität und die Reconciliation-Publisher-Identität
sind getrennt. Die Publisher-Identität wird ausschließlich über das geschützte
`release-reconciliation` Environment und einen eigenen Broker, App-Key und
kurzlebigen Installation-Token verwendet. Sie publiziert nur den
provenance-validierten Kandidaten und dessen Pull Request; sie hat keinen
Ruleset-Bypass, keine Release-Line-Dispatch- und keine Shared-Line-Mutation-
Berechtigung. ADR-0005 definiert diese Identitätsgrenze.

Bei einem Merge-Konflikt wird kein unaufgelöster Branch gepusht und kein PR
erstellt. Der Konfliktnachweis bindet Release-SHA, Develop-SHA, Ticket,
Preparation-Branch, Konfliktpfade und Controller-Run. Eine menschlich oder
agentisch aufgelöste Kandidatenbranch bleibt nicht-shared und erhält keine
Release-Automation-Berechtigung.

Der geschützte `reconciliation-resume`-Pfad übernimmt einen Kandidaten nicht
aufgrund seines Namens. Er prüft Branch- und Ticketbindung, den unveränderten
Release-Ursprung sowie einen exakten Zwei-Parent-Merge mit der gepinnten
Develop-Revision. Erst danach darf CI mit kurzlebigem Broker-Token Quality
ausführen, die Kandidatenbranch veröffentlichen und den PR nach `develop`
erstellen.

## Invarianten

- `release/<semver>` wird nie durch den Controller aktualisiert, rebased oder
  direkt gepusht.
- Der Controller akzeptiert nur einen `main`-Workflow-Dispatch im geschützten
  Release-Environment.
- OIDC-Token und Installation-Token werden nicht ausgegeben, persistiert oder
  als Repository-Secret gespeichert.
- Die Reconciliation-Publisher-Identität ist von der Release-Automation-
  Identität getrennt und besitzt nur die minimale Kandidaten- und PR-
  Publikationsberechtigung.
- Der Git-Transport-Header ist nur im lokalen Runner-Konfigurationsbereich
  vorhanden und wird vor Job-Ende entfernt.
- Der Preparation-Branch trägt den Ticketbezug, startet von der Release-Ref und
  ist der einzige Merge-Ort für den aktuellen Develop-Stand.
- Ein Konflikt führt fail-closed zu keiner Shared-Line-Mutation, keinem
  unaufgelösten Remote-Branch und keinem PR.
- Ein Recovery-Kandidat muss exakt aus Release- und gepinntem Develop-Parent
  bestehen; beliebige Branch-Eingaben sind kein Vertrauensnachweis.
- Der resultierende Pull Request zielt auf `develop` und verwendet einen Merge
  Commit.
- Der Controller erstellt PRs idempotent, merged aber niemals direkt nach
  `develop`; Review und Required Checks bleiben bindend.

## Konsequenzen

- Ein nach `develop` gemergter Workflow kann erst nach seiner governeden
  Main-Control-Plane-Promotion privilegiert für eine ausgelieferte Release-Linie
  verwendet werden.
- Dry-Run-Aufrufe bleiben strikt read-only und dürfen weder Provider-Publish
  noch Pull-Request-Erstellung auslösen.
- Die Reconciliation bleibt nachvollziehbar, ohne die veröffentlichte
  Release-Lineage zu verändern.
- Der Normalpfad benötigt keinen manuellen Operatorstart; dessen manueller
  Fallback bleibt für kontrollierte Recovery verfügbar.
