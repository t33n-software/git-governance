# ADR-0004: Governed Release Reconciliation Base Alignment

- Status: angenommen
- Datum: 2026-08-01
- Geltungsbereich: `release/<semver> -> develop` bei strikter
  Develop-Basisaktualität
- Entscheider: Release-Governance

## Kontext

Nach einer erfolgreichen `release/<semver> -> main`-Promotion, einem
unveränderlichen Tag und bestätigter Delivery muss der Release gegen `develop`
reconciliert werden. Neue reguläre Pull Requests können in der Zwischenzeit
nach `develop` gemergt worden sein.

Ein striktes Develop-Ruleset kann für einen Backmerge einen aktuellen
PR-Head verlangen. Ein GitHub-„Update branch“, ein Rebase oder ein direkter
Merge von `develop` in `release/<semver>` würde jedoch die bereits ausgelieferte
Shared Release Line nachträglich verändern.

Die Release-Ref bleibt bis zum dokumentierten Reconciliation-Abschluss erhalten,
ist nach Promotion und Delivery aber keine Arbeitslinie mehr.

## Entscheidung

Bei einem effektiven Release-only-Delta und einer strikten
Develop-Basisaktualität erfolgt die Reconciliation ausschließlich über eine
ticketgebundene Preparation-Branch:

```text
release/<semver>
  -> chore/<ticket>-<reconciliation-alignment>
  -> kontrollierter Merge von origin/develop
  -> vollständige Quality-, Security- und Review-Gates
  -> Merge-Commit-PR nach develop
```

Der Workflow `workflow release align-reconciliation-base` akzeptiert nur eine
aus der angegebenen Release-Linie erzeugte, aktuell ausgecheckte
`chore/*`-Preparation-Branch. Er prüft den Delivery-Nachweis und den
effektiven Delta, merged `origin/develop` ausschließlich in die Working-Branch,
führt die Repository-Quality-Gates aus und publiziert optional den
Merge-Commit-PR nach `develop`.

Wenn Develop keinen aktuellen PR-Head verlangt, bleibt der direkte, kontrollierte
PR `release/<semver> -> develop` zulässig. Wenn kein effektiver Delta besteht,
wird kein Preparation-Branch-PR erstellt; das Ergebnis lautet `not-required`.

## Invarianten

- `release/<semver>` bleibt nach Promotion, Tag und Delivery unverändert.
- Neue Develop-Arbeit wird niemals nachträglich in `release/<semver>` gemergt.
- Ein Preparation-Branch startet von exakt der Release-Ref und besitzt einen
  Ticketbezug.
- Die aktuelle Develop-Ref wird mit einem nachvollziehbaren Merge-Commit in die
  Preparation-Branch gemergt, nicht rebased.
- Ein Konflikt bleibt fail-closed, bis nur die konkreten Pfade aufgelöst und
  gestagt wurden; die Fortsetzung erfolgt über den governeden Resume-Vertrag.
- Ein serverseitig veröffentlichter Resolution-Kandidat weist exakt die
  Release-Ref als ersten und die geprüfte Develop-Ref als zweiten Merge-Parent
  nach.
- Der resultierende PR zielt auf `develop` und verwendet einen Merge Commit.
- Eine neue funktionale Änderung nach Delivery erfordert einen neuen
  Patch-Release oder Hotfix auf der tatsächlich betroffenen veröffentlichten
  Linie; sie wird nicht rückwirkend in den ausgelieferten Release geschrieben.

## Abgelehnte Alternativen

### GitHub Update branch auf `release/<semver> -> develop`

Abgelehnt, weil die Aktion die geschützte Release-Ref direkt verändert und den
ticketgebundenen Merge, die Quality-Gates und den separaten Review-Event
umgeht.

### `develop -> release/<semver>`-Pull-Request

Abgelehnt, weil neue Integrationsarbeit Teil eines bereits ausgelieferten
Release-Stands würde. Der Backmerge dient ausschließlich der Rückführung
fehlender Release-Differenzen nach `develop`.

### Rebase der Release-Ref oder der Preparation-Branch auf Develop

Abgelehnt, weil ein Rebase die relevante Merge-Provenance verdeckt. Die
Preparation-Branch muss den aktuellen Develop-Stand durch einen sichtbaren,
ticketgebundenen Merge aufnehmen.

### Abschwächung der strikten Develop-Basisaktualität

Abgelehnt, weil sie auch reguläre Integrations-PRs schwächen würde. Der
source-aware Preparation-Workflow erhält die Zielschutzregel und löst nur den
Release-Reconciliation-Sonderfall.

## Konsequenzen

- Die veröffentlichte Release-Lineage bleibt unverändert und auditierbar.
- Develop kann während der Release-Delivery weiter reguläre Arbeit integrieren.
- Der konkrete kombinierte Stand wird vor dem Backmerge vollständig geprüft.
- Der Backmerge bleibt ein sichtbares, reviewbares Release-Ereignis.
- Der Release-Branch wird erst nach erfolgreich gemergtem Backmerge oder
  auditierbarem `not-required` bereinigt.
