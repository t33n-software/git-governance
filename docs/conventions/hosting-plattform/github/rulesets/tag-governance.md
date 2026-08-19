# Tag-Governance: Version-Tag-Namespace und Namespace-Floor
[INTENT: SPEZIFIKATION]

Dieses Dokument beschreibt eigenständig, was die beiden Tag-Rule-Sets der
Familie durchsetzen und warum. Die importierbaren JSON-Definitionen liegen
unter [`rulesets/github/`](../../../../../rulesets/github/README.md).

## Übersicht

| Rule-Set | Ziel-Refs | Klasse |
|---|---|---|
| `tag-governance: release version tags` | `refs/tags/v*` | klassenlos, `~ALL` |
| `tag-governance: tag namespace floor` | `refs/tags/*` ohne `refs/tags/v*` | klassenlos, `~ALL` |

## Warum zwei Artefakte

Ein Rule-Set trägt genau ein Target, genau eine Ref-Selektor-Menge und genau
eine Bypass-Liste. Der `v*`-Namespace und der übrige Namespace benötigen
verschiedene Regeln und verschiedene Bypass-Actors; zwei klassenlose
Artefakte derselben Familie sind die korrekte Form — niemals ein
zusammengemischtes Rule-Set.

## `07-release-version-tags`: der Evidence-Namespace

Version-Tags (`v<semver>`) sind die unveränderlichen Evidence-Anker der
Delivery-Kette: Der immutable Release-Tag referenziert den freigegebenen
Promotion-Merge-Commit, und die Release-Attestation bindet Tag, Commit und
Assets. Deshalb gilt:

- **creation**, **update**, **deletion**: nur über Bypass-Actors.
- Bypass-Actors: die Release-Automation-GitHub-App (`Integration`,
  `bypass_mode: always`) und die Organisations-Owner-Rolle
  (`OrganizationAdmin`, `always`) als benannter, auditierter
  Break-Glass-Pfad. Die Hotfix-Delivery-Lane verwendet für Tag-Operationen
  dieselbe Release-Automation-Identität; ein Integrations-Bypass deckt beide
  Lanes ab.
- `bypass_mode` ist niemals `exempt`: Jede Umgehung MUSS einen
  Audit-Eintrag erzeugen.
- Die Bypass-Liste ist konstitutiv, keine Ausnahme: Ohne sie würde das
  Rule-Set die governete Release-Automation selbst blockieren. Die konkrete
  App-ID im Artefakt ist die Referenz-Bindung der besitzenden Organisation
  an die logische Release-Automation-Identität; Adopter substituieren ihre
  eigene App-ID (wie `source` und die Check-Kontexte), und die
  Steady-State-Projektion bindet die konkrete ID aus den
  Instanz-Bindings.

**Aktivierungsvorbedingung (blockierend).** Ein erstellungsbeschränktes
Tag-Rule-Set schlägt für jede Identität fail-closed fehl, die kein
Bypass-Actor ist. Die governeten Tag-Workflows MÜSSEN sich deshalb vor der
Aktivierung als die Release-Automation-App authentisieren
(Broker-gemintetes Installation-Token); ein Tag-Push mit dem
Repository-`GITHUB_TOKEN` wird nach der Aktivierung abgelehnt. Bis dahin wird
das Artefakt mit `enforcement: disabled` importiert und erst nach einer
verifizierten Tag-Erstellung über die App-Identität aktiviert.

**Verhältnis zur Release-Immutability.** Defense in Depth: Die
Release-Immutability sperrt Tags **publizierter** Releases; das Tag-Rule-Set
sperrt den gesamten `v*`-Namespace unabhängig davon, ob ein Release-Objekt
existiert, und bindet die Erstellung an die Automation-Identität.

## `08-tag-namespace-floor`: der Floor für den Rest

Tags außerhalb `v*` haben in dieser Architektur keine kanonische Rolle:
Staging ist eine Artefakt-Umgebung, Releases sind `v*`. Ein ungoverneter
Look-alike-Tag ist ein Confusion- und Supply-Chain-Vektor. Deshalb gilt:

- **creation** und **update**: nur über die Organisations-Owner-Rolle
  (Break-Glass).
- Bewusst **kein** `deletion`: Bestehende Nicht-`v*`-Tags bleiben
  aufräumbar; der Namespace kann nur schrumpfen, nie wachsen oder sich
  bewegen.

## Bewusst nicht enthalten

- **Kein `tag_name_pattern`:** Die Tag-Grammatik wird von der governeten
  Release-Automation zur Erstellungszeit erzwungen; da nur diese Identität
  `v*`-Tags erstellen kann, wäre eine Muster-Regel redundant. Die
  Muster-Regeltypen sind außerdem an das Enterprise-Cloud-Entitlement
  gebunden.
- **Keine Required Status Checks:** Tag-Rule-Sets tragen keine
  `required_status_checks`; `do_not_enforce_on_create` gilt nicht, und
  `bypass_mode: pull_request` existiert nur für Branch-Rule-Sets.
- **Keine Klassen:** Tag-Governance ist klassenunabhängig; beide Artefakte
  bleiben klassenlos auf `~ALL`. Tag-Rule-Sets existieren — anders als
  Push-Rule-Sets — auch auf öffentlichen Repositories.

## Namens-Triple

Titel, Selektor und Dateiname bilden das maschinell prüfbare Triple:
`tag-governance: <aggregat>` ↔ `~ALL` ↔ `07|08-<name>.json`. Die Tag-Familie
trägt bewusst keine `quality-gates`-Klasse.
