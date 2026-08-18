# ADR-0003: Governed Release Promotion Base Alignment

- Status: angenommen
- Datum: 2026-08-01
- Geltungsbereich: `release/<semver> -> main` bei strikter Main-Basisaktualität
- Entscheider: Release-Governance

## Kontext

`main` und eine eingefrorene `release/<semver>`-Linie können beidseitig
divergieren. Ein bloßer Hinweis, dass der Promotion-PR „out of date“ ist,
beweist weder einen Merge-Konflikt noch, dass alle fehlenden Main-Commits in
den Release gehören.

Das Main-Ruleset verlangt aktuelle erforderliche Statuschecks. GitHub kann für
einen PR eine Aktion wie **Update branch** anzeigen. Diese Aktion aktualisiert
jedoch die Head-Ref des PR. Bei einem Promotion-PR mit
`release/<semver>` als Head wäre dies eine direkte Mutation einer geschützten
Shared Line.

Ein Rebase der Release-Line ist ebenfalls ausgeschlossen: Er schreibt
veröffentlichte Release-Historie um und erfordert einen nicht-fast-forward
Update. Eine Schwächung des Main-Rulesets würde dagegen den kombinierten
Main-/Release-Zustand ungeprüft lassen.

## Entscheidung

Eine erforderliche Promotion-Base-Ausrichtung erfolgt ausschließlich über eine
ticketgebundene `release-prep`-Working-Branch:

```text
release/<semver>
  -> chore/<ticket>-<promotion-alignment>
  -> kontrollierter Merge von origin/main
  -> Quality Gates und Review
  -> Merge-Commit-PR nach release/<semver>
  -> erneute Prüfung des bestehenden release/<semver> -> main PR
```

Die bestehende `release/*`-Ref wird nie durch GitHub **Update branch**, Rebase,
Force Push oder einen direkten Entwickler- beziehungsweise CI-Code-Commit
ausgerichtet.

Die Ausrichtung ist nur zulässig, wenn vor dem Merge nachgewiesen wird:

1. Das Main-Ruleset verlangt tatsächlich einen aktuellen Promotion-Head.
2. Die fehlenden Main-Commits sind bereits inhaltlich auf der Release-Line
   vertreten oder ausdrücklich für diese Release-Version freigegeben.
3. Die Working-Branch wurde aus genau der betroffenen Release-Line erstellt.
4. Der kombinierte Zustand besteht alle für die Release-Line geltenden
   Qualitäts-, Sicherheits- und Review-Gates.

Der lokale Produktadapter ist
`workflow release align-promotion-base`. Er ist keine neue kanonische
Git-Policy: Ein geschützter Workflow oder eine eng begrenzte GitHub-App-/
Hosting-Integration ist gleichwertig, wenn sie dieselben Invarianten
durchsetzt.

## Durchsetzung

Die Entscheidung wird auf mehreren Ebenen durchgesetzt:

- `04-release.json` schützt `release/*` mit PR-Pflicht,
  `non_fast_forward`, Review, Statuschecks und ausschließlich Merge Commits.
- Der Workflow akzeptiert nur die ausgecheckte `chore/*`-Release-Preparation-
  Branch mit gespeicherter Basis `origin/release/<semver>`.
- Der Merge von `origin/main` entsteht auf der Working-Branch mit einer
  ticketgebundenen Nachricht, nicht auf der Shared Line.
- Ein Konflikt bleibt auf der Working-Branch fail-closed. Der kontrollierte
  Resume-Pfad verlangt explizit gestagte Resolution-Pfade, bindet
  `MERGE_HEAD` an die gefetchte Main-Revision und verwirft einen Kandidaten,
  wenn Main vor der Veröffentlichung weitergelaufen ist.
- Vor Push und PR laufen die Repository-Quality-Gates.
- Der PR zurück auf `release/*` und der spätere Promotion-PR nach `main`
  bleiben unabhängige Review-Ereignisse.

GitHub Rulesets können die zugrunde liegende Release-Ref schützen, aber keinen
UI-Button anhand der PR-Head-Familie separat ausblenden. Deshalb muss die
Release-Ref-Mutation serverseitig scheitern; die source-aware Entscheidung
bleibt Aufgabe des kontrollierten Workflows.

## Abgelehnte Alternativen

### GitHub Update branch auf einem Promotion-PR

Abgelehnt, weil die Aktion den Head `release/*` direkt aktualisiert und damit
Ticket, Provenance, getrennten Release-Review und explizite Quality-Gates
umgeht.

### Rebase von `release/*` auf `main`

Abgelehnt, weil die Shared-Line-Historie umgeschrieben wird und ein
nicht-fast-forward Update benötigt wird.

### Deaktivieren der strikten Main-Basisaktualität

Abgelehnt, weil die Resultatmenge aus Main und Release dann nicht zwingend von
den erforderlichen Checks geprüft wird.

### Merge Queue als alleinige Lösung

Nur dann zulässig, wenn sie den resultierenden Merge gegen das aktuelle Main
prüft und zusätzlich die release-spezifische Freigabe- und
Kompatibilitätsentscheidung abbildet. Sie ersetzt keine Release-Preparation-
Provenance.

## Konsequenzen

- Ein „out of date“-Hinweis löst zuerst eine semantische
  Release-Kompatibilitätsprüfung aus, keinen automatischen Git-Vorgang.
- Eine wiederverwendbare, auditierbare Ausrichtung ist möglich, ohne
  Release-Line-Rewrites zuzulassen.
- Release- und Main-PR bleiben getrennte Governance-Grenzen.
- Nach erfolgreicher Promotion folgen weiterhin Tag, Delivery-Nachweis und die
  delta-bedingte Reconciliation nach `develop`.

## Referenzen

- `docs/specification/POLICY-UND-VALIDIERUNG.md`
- `docs/usage/workflows/release.md`
- `docs/specification/CLI-VERTRAG.md`
- `rulesets/github/04-release.quality-gates-full.json`
- `rulesets/github/README.md`
