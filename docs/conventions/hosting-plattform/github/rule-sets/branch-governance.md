# Branch-Governance: Aufbau der Shared-Line- und Working-Branch-Rule-Sets
[INTENT: SPEZIFIKATION]

Dieses Dokument beschreibt eigenständig, was jedes Rule-Set der Familie
durchsetzt und warum. Die importierbaren JSON-Definitionen liegen unter
[`rulesets/github/`](../../../../../rulesets/github/README.md).

## Übersicht

| Rule-Set | Ziel-Refs | Klasse |
|---|---|---|
| `push-protections: secret artifact boundary` | jeder Push (keine Branch-Bindung) | klassenlos, nur private/interne Repositories |
| `branch-governance: ticket working branches` | `feature/*`, `fix/*`, `docs/*`, `refactor/*`, `chore/*`, `test/*`, `perf/*`, `hotfix/*` | klassenlos, `~ALL` |
| `branch-governance: develop shared line (quality-gates=<klasse>)` | `develop` | full / linux-only |
| `branch-governance: main shared line (quality-gates=<klasse>)` | `main` | full / linux-only |
| `branch-governance: release shared lines (quality-gates=<klasse>)` | `release/*` | full / linux-only |
| `branch-governance: support shared lines (quality-gates=<klasse>)` | `support/*` | full / linux-only |

## Working-Branch-Schutz

Offizielle Arbeitsbranches bleiben direkt becommitbar, werden aber nach dem
ersten Push append-only: `non_fast_forward` blockiert das Umschreiben
veröffentlichter Historie. Kein Merge-Zeit-Gate und kein Löschverbot wird
dort gesetzt, damit der normale Entwicklungsfluss und die automatische
Löschung gemergter Head-Branches funktionieren.

## Shared-Line-Schutz

Jede Shared Line erhält denselben Schutzkern:

- **deletion**: Die Linie ist niemals löschbar; sie trägt veröffentlichte
  Historie, Promotion- und Evidenz-Lineage.
- **non_fast_forward**: Kein Rewrite der veröffentlichten Linie.
- **pull_request**: Mutation nur über reviewte Pull Requests mit mindestens
  einer Freigabe, verworfenen veralteten Reviews, erzwungener Auflösung aller
  Review-Threads und **Code-Owner-Review** (`require_code_owner_review` gegen
  `.github/CODEOWNERS`). Ohne die versionierte Ownership-Datei blockiert die
  Linie fail-closed — der Vertrag MUSS vor der Aktivierung gemergt sein.
- **required_status_checks** (strict): Die verbindlichen Kontexte lauten
  `Quality gates (<os>)` je nach Klasse plus `Dependency admission review`.
  Der strenge Modus bindet den Merge an einen aktuellen PR-Stand.
- **code_scanning**: CodeQL mit Schwellen `all` für Alerts und Security-Alerts;
  auf öffentlichen Repositories ohne Zusatzkosten verfügbar.

Merge-Methoden je Linie:

| Linie | Erlaubte Methoden | Grund |
|---|---|---|
| `develop` | merge, rebase, squash | Kontextwahl für reguläre Tickets; die semantische Commit-Serie darf erhalten bleiben oder bereinigt werden |
| `main`, `release/*`, `support/*` | nur merge | Release-, Hotfix- und Wartungs-Lineage bleibt als explizites Merge-Ereignis sichtbar |

## Erstellungsausnahme `do_not_enforce_on_create`

Nur `release/*`- und `support/*`-Rule-Sets tragen
`do_not_enforce_on_create: true`: Eine neu erzeugte geschützte Linie kann vor
ihrer Existenz keine branch-bezogenen Check-Ergebnisse besitzen; ohne die
Ausnahme wäre die governete Erzeugung per Definition unmöglich. `develop` und
`main` tragen explizit `false`: Sie sind stehende Linien ohne kontrollierten
wiederkehrenden Erzeugungspfad; eine Neuerstellung ist eine Anomalie und MUSS
das volle Gate treffen. Die Ausnahme gilt nur für das einmalige
Erstellungsereignis und lockert kein anderes Rule.

## Namens-Triple

Titel, Selektor und Dateiname bilden ein maschinell prüfbares Triple:
`<kontext>: <aggregat> [(quality-gates=<klasse>)]` ↔
`repository_property`-Wert ↔ `<nn>-<linie>[.quality-gates-<klasse>].json`.
Klassenlose Rule-Sets tragen bewusst keine Klasse.

## Bewusst nicht angelegt

- **`scratch/*`**: private Exploration mit eigener Rewrite-Grenze; niemals
  PR-Quelle — ein Rule-Set wäre wirkungslos oder schädlich.
- **`staging`**: eine Umgebung aus Release-Artefakten, kein Branch.
- **Separates `hotfix/*`-Rule-Set**: redundant, weil `hotfix/*` denselben
  Working-Branch-Schutz erhält und sein PR-Ziel das strengere
  Shared-Line-Rule-Set trägt.
