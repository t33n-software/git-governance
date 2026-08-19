# Hosting-Plattform: GitHub — rulesets
[INTENT: REFERENZ]

## Kanonische Quelle

Die GitHub-rulesets für die Organisation `t33n-software` werden einmalig und
zentral in diesem Repository unter
[`rulesets/github/`](../../../../../rulesets/github/README.md) definiert und
verwaltet. Dieses Repository ist die kanonische Quelle der Wahrheit für die
JSON-Definitionen: Es erklärt die Architektur, setzt die Definitionen und
liefert die versionierten, importierbaren Artefakte.

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

## Dokumentation der Konventionen

| Dokument | Inhalt |
|---|---|
| [Organisation als Verwaltungsebene](organisation.md) | Warum eine Organisation existieren MUSS; Organisations-Verwaltung; Anti-Pattern einzelner Repository-rulesets |
| [Branch-Governance](branch-governance.md) | Aufbau und Begründung jedes Shared-Line- und Working-Branch-rulesets; Namens-Triple |
| [Push-Protections](push-protections.md) | Grenze gegen secret-förmige Artefakte; Verfügbarkeit; Template-Architektur |
| [Klassen und Selektoren](klassen-und-selektoren.md) | `quality-gates`-Klassenmodell, gegenseitiger Ausschluss, Selektor-Formen, `~ALL` |
| [Code-Quality und Coverage](code-quality-und-coverage.md) | Organisations-eigene, sprachagnostische Gates; Ausschluss der Hosting-Controls |
| [Merge-Strategie](merge-strategie.md) | Merge-Methoden-Matrix, Update-branch-Grenze, globale Repository-Einstellungen |
| [Import und Verifikation](import-und-verifikation.md) | Voraussetzungen, Import-Reihenfolge, Evaluate→Active, Verifikations-Checkliste |
| [Tag-Governance](tag-governance.md) | Version-Tag-Namespace an die Release-Automation-Identität, Namespace-Floor, Aktivierungsvorbedingung |
| [Custom Properties](../custom-properties/README.md) | Positive-List-Konvention, Drei-Schichten-Vertrag, `org_actors`-Grenze, `pending`-Onboarding, Aktivierungssequenz |

## Verwaltung

- Verwaltungsebene: die **Organisation** (`t33n-software`), niemals die
  einzelne Repository-Ebene.
- Klassenmitgliedschaft dieses Repositorys: Custom Property
  `quality-gates=full`.
- Die Custom-Property-Definition liegt kanonisch in
  [`properties/github/`](../../../../../properties/github/README.md); ihre
  Projektion und Zuweisung folgen der
  [Custom-Properties-Konvention](../custom-properties/README.md).
- Änderungen an den rulesets erfolgen ausschließlich in diesem kanonischen
  Repository und werden danach auf Organisationsebene re-importiert.
