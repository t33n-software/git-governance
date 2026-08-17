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

## Trigger-Grenze und Admission-Act

Die Auslösung des Reconciliation-Controllers ist ein eigener Governance-Act
und strikt von der privilegierten Ausführungskette getrennt. Der Trigger ist
eine Admission — die bewusste Anmeldung eines Sachverhalts zur privilegierten
Prüfung — und nicht das Privileg selbst. Die privilegierte Wirkung entsteht
ausschließlich hinter der serverseitigen Kette aus Environment-Freigabe,
OIDC/WIF-Workload-Identität, Publisher-Broker und dediziertem
Publisher-Token (ADR-0005).

Vier Festlegungen gelten verbindlich:

1. **Trigger-Identitäts-Äquivalenz.** Ob ein Mensch den Dispatch über die
   GitHub-Oberfläche, ein Mensch oder Agent über `gh` oder ein explizites,
   separates Kommando auslöst, ist trust-äquivalent, solange der Act separat,
   bewusst und mit aufgezeichneter Actor-Identität erfolgt. Die Kontrolle
   trägt die serverseitige Freigabe- und Validierungskette, nicht die
   auslösende Identität. Ein Dispatch erfordert weder Publisher- noch
   Shared-Line-Berechtigung, sondern nur eine dispatch-berechtigte
   Operator-Identität.

2. **Kein automatischer Trigger als Side-Effect.** Die lokale CLI löst den
   Reconciliation-Controller niemals als automatische Folgewirkung eines
   Kandidaten-Pushs oder Resume aus. Vorbereitung (nicht-shared, lokal) und
   Admission (bewusster Act) kollabieren nicht zu einem automatisierten Akt;
   die lokale CLI erhält kein stehendes Dispatch-Credential für die
   Reconciliation-Lane.

3. **Automatisierungs-Ort ist serverseitig.** Die reguläre
   Trigger-Automatisierung gehört ausschließlich in den serverseitigen
   Delivery-Lifecycle, der nach bestätigter Delivery event-getrieben und
   idempotent startet. Lokales Tooling ist niemals der
   Automatisierungs-Ort der Reconciliation-Lane.

4. **Manueller Dispatch als geschützter Recovery-Einstieg.** Der manuelle
   Dispatch bleibt der vorgesehene Incident-, Retry- und Recovery-Pfad. Er
   ist eine bewusste Admission-Entscheidung nach einem fail-closed Zustand
   und durchläuft dieselben Eingabe-, Delivery-, Idempotenz- und
   Auditprüfungen wie der automatische Pfad.

Diese Grenze begründet sich nicht aus einer fehlenden Fähigkeit der lokalen
CLI — ein Dispatch benötigt keine Publisher-Rechte —, sondern aus der
Trennung von Deliberation und Privileg, der Least-Privilege-Hygiene lokaler
Tooling-Identitäten und dem serverseitigen Automatisierungs-Ort.

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
- Der Reconciliation-Trigger ist eine bewusste Admission; Mensch, Agent und
  UI-Dispatch sind trust-äquivalent, solange der Act separat und auditiert
  bleibt.
- Die lokale CLI triggert den Controller niemals als Side-Effect einer
  lokalen Mutation und hält kein stehendes Dispatch-Credential für die
  Reconciliation-Lane.
- Die reguläre Trigger-Automatisierung liegt serverseitig im
  Delivery-Lifecycle; der manuelle Dispatch bleibt der geschützte
  Recovery-Einstieg.

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
- Die Trigger-Grenze ist als Admission-Act niedergelegt; die Fehllesart, die
  lokale CLI könne den Dispatch nicht ausführen, weil ihr Publisher-Rechte
  fehlten, ist ausgeschlossen — ein Dispatch benötigt nur eine
  dispatch-berechtigte Operator-Identität.
