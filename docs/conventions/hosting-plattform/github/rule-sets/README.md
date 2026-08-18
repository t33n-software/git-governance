# Hosting-Plattform: GitHub — Rule-Sets
[INTENT: REFERENZ]

## Kanonische Quelle

Die GitHub-Rule-Sets für die Organisation `t33n-software` werden einmalig und
zentral im Repository
[`git-governance`](https://github.com/t33n-software/git-governance) unter
`rulesets/github/` definiert und verwaltet. Dieses Repository ist die
kanonische Quelle der Wahrheit für die JSON-Definitionen: Es erklärt die
Architektur, setzt die Definitionen und liefert die versionierten,
importierbaren Artefakte.

Eine lokale Kopie, Neudefinition oder Abweichung in einem anderen Repository
ist ein Anti-Pattern und verboten (Redundanz- und Drift-Verbot). Erlaubt sind
ausschließlich benannte, auditierbare Repository-Ausnahmen, die restriktiver
als die Organisations-Grundlage sind, niemals schwächer.

## Verwendete Familie

Dieses Projekt (`git-governance`) verwendet die Familie
**`quality-gates=full`**:

- Die Quality Gates laufen für **Linux**, **Windows** und **macOS**.
- Architektonische Begründung: Dieses Projekt liefert ein CLI aus, das als
  natives Binary für alle drei Betriebssysteme gebaut, attestiert und
  verifiziert wird; die Auslieferung für alle Betriebssysteme erfordert die
  vollständige Quality-Gate-Matrix.

## Gebundene Rule-Sets

| Rule-Set | Klasse |
|---|---|
| `push-protections: secret artifact boundary` | klassenlos (private/interne Sichtbarkeit) |
| `branch-governance: ticket working branches` | klassenlos (`~ALL`) |
| `branch-governance: develop shared line (quality-gates=full)` | full |
| `branch-governance: main shared line (quality-gates=full)` | full |
| `branch-governance: release shared lines (quality-gates=full)` | full |
| `branch-governance: support shared lines (quality-gates=full)` | full |

## Verwaltung

- Verwaltungsebene: die **Organisation** (`t33n-software`), niemals die
  einzelne Repository-Ebene.
- Klassenmitgliedschaft dieses Repositorys: Custom Property
  `quality-gates=full`.
- Änderungen an den Rule-Sets erfolgen ausschließlich im kanonischen
  Repository und werden danach auf Organisationsebene re-importiert
  (Organisation Settings → Repository → Rulesets).
