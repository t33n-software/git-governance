# Organisation als Verwaltungsebene
[INTENT: ANWEISUNG]

## Konvention

Eine GitHub-**Organisation MUSS** als Verwaltungsebene für rulesets
existieren, weil nur dort repository-übergreifende rulesets definiert werden
können. Die rulesets dieser Organisation werden einmalig auf
Organisationsebene verwaltet:

- grafisch: **Organization Settings → Repository → Rulesets**
- programmatisch: `POST /orgs/{org}/rulesets`

Voraussetzung: Der Organisations-Plan MUSS mindestens **GitHub Team** (oder
Enterprise) sein, weil die Organisations-Ruleset-Oberfläche erst ab diesem
Plan existiert.

## Anti-Pattern: einzelne Repository-rulesets

Es ist ein **Anti-Pattern**, die allgemeinen rulesets einzeln pro Repository
zu definieren. Eine pro Repository kopierte Definition dupliziert eine
Governance-Definition über die Flotte, driftet unbemerkt auseinander und
erzeugt redundante Parallelwahrheiten.

Einzelne Repository-rulesets sind **nur** bei expliziten Ausnahmeszenarien
valide. Ein Ausnahmeszenario MUSS benannt und auditierbar sein, zum Beispiel
ein repository-spezifischer Required-Check-Kontext oder eine strengere
repository-lokale Schicht oberhalb der Organisations-Grundlage. Repository-
und Organisations-rulesets aggregieren: Das Aggregat kann einen Branch nur
restriktiver machen, niemals schwächer. Ein Ausnahme-Rule-Set DARF daher
niemals versuchen, ein Organisations-Rule-Set abzuschwächen, zu duplizieren
oder zu überdecken; ein empfundenes Bedürfnis nach einer schwächeren Regel
wird über das Targeting des Organisations-rulesets gelöst (Ausschluss oder
engere Selektion), niemals über ein gegenläufiges Repository-Rule-Set.

## Governance-Eigenschaften der Organisationsebene

- Nur **Organisations-Owner** können Organisations-rulesets erstellen und
  bearbeiten; Repository-Admins können lediglich zusätzliche, strengere
  Repository-rulesets ergänzen.
- Die Organisationsebene ist damit die verbindliche Governance-Basis für
  alle Repositories.
- Neue Repositories werden bei der Selektorform `~ALL` automatisch ab ihrer
  Erstellung gebunden, ohne Re-Import.
