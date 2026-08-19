# Merge-Strategie und Repository-Einstellungen
[INTENT: SPEZIFIKATION]

## Merge-Methoden je Ziel-Linie

Die Merge-Strategie ist eine bewusste, zielabhängige Governance-Entscheidung,
kein generischer Plattform-Default. rulesets wirken auf die Ziel-Branch,
nicht auf die Quell-Familie; die Auswahl trifft die merge-berechtigte Person
zum Merge-Zeitpunkt nach dieser Matrix:

| Szenario | Ziel | Methode | Grund |
|---|---|---|---|
| Reguläres Ticket mit bedeutsamer semantischer Commit-Serie | `develop` | Rebase-Merge | Erhält die reviewte Serie ohne Merge-Commit |
| Reguläres Ticket mit sichtbarer Integrations-Topologie | `develop` | Merge-Commit | Explizites Integrationsereignis |
| Reguläres Ticket mit internem Commit-Rauschen | `develop` | selektiver Squash | Ein sauberer Integrations-Commit |
| Release-Promotion | `main` | Merge-Commit | Release-Lineage und sichtbare Freigabe |
| Hotfix auf die betroffene Shared Line | `main`, `release/*`, `support/*` | Merge-Commit | Explizite Incident-Lineage |
| Stabilisierungs- oder Wartungs-PR | `release/*`, `support/*` | Merge-Commit | Kontrollierte Shared-Line-Historie |
| Release-Backmerge | `develop` | Merge-Commit erlaubt | Überträgt nur die effektive Release-Differenz nach bestätigter Delivery |

Lokaler Rebase vor dem ersten Push eines unveröffentlichten Arbeitsbranchs
ist eine andere Operation als der GitHub-Rebase-Merge und nur dort erlaubt;
nach dem ersten Push bleibt die offizielle Branch append-only.

## Update-branch-Grenze

GitHubs **Update branch**-Aktion mutiert die PR-Head-Ref; sie ist keine
Metadaten-Auffrischung. Bei einem PR mit einer Shared Line als Head wäre das
eine direkte Mutation einer geschützten Linie — die rulesets lehnen das
zugrunde liegende Update ab. Eine erforderliche Basis-Ausrichtung einer
veralteten Promotion oder Reconciliation erfolgt ausschließlich über eine
ticketgebundene Preparation-Working-Branch mit vollständigen Gates und
eigenem Merge-Commit-PR; die eingefrorene Shared Line wird nie durch die
Plattform-Oberfläche aktualisiert.

## Repository-Einstellungen (global, pro Repository)

| Einstellung | Erforderlicher Zustand | Grund |
|---|---|---|
| Allow merge commits | aktiviert | Für `main`, `release/*`, `support/*` und Integrationspfade erforderlich |
| Allow rebase merging | aktiviert | Für den erlaubten Rebase-Merge-Pfad nach `develop` |
| Allow squash merging | aktiviert | Für den erlaubten selektiven Squash-Pfad nach `develop` |
| Automatically delete head branches | aktiviert | Räumt gemergte, löschbare Head-Branches automatisch auf; Shared Lines sind durch den Löschschutz davon unberührt |
| Allow auto-merge | deaktiviert | Auto-merge würde die bewusste Merge-Methoden-Entscheidung und die Freigabe-Reihenfolge unterlaufen |
| Always suggest updating pull request branches | deaktiviert | Der sichtbare Update-Vorschlag ist keine Autorisierung, eine geschützte PR-Head zu mutieren |
| Release immutability | aktiviert, bevor die erste Produktions-Release veröffentlicht | Veröffentlichte Releases und Tags bleiben unveränderliche Evidenz |

Die effektive Merge-Methode ist die Schnittmenge aus global aktivierter
Fähigkeit und den `allowed_merge_methods` des Ziel-rulesets.
