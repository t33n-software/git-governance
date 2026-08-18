# Klassen und Selektoren
[INTENT: SPEZIFIKATION]

## Klassenmodell

Die Flotte wird über die Repository-Custom-Property `quality-gates`
partitioniert:

| Klasse | Property-Wert | Bedeutung |
|---|---|---|
| `full` | Quality Gates für Linux, Windows und macOS | Projekte, die Binaries für alle drei Betriebssysteme ausliefern (CLI-Werkzeuge) |
| `linux-only` | Quality Gate ausschließlich für Linux | Projekte, die reine Linux-/Docker-/CI-/CD-Artefakte liefern und keine betriebssystemspezifischen Binaries ausliefern |

Die architektonische Begründung der Klassen: Projekte mit echten
Betriebssystem-Auslieferungen benötigen die vollständige Matrix, weil jede
Plattform eine eigenständige Auslieferungsgrenze ist. Projekte ohne solche
Artefakte würden von Windows- und macOS-Gates keinen zusätzlichen
Kontrollwert erhalten; ihre verbindliche Grenze ist die Linux-CI, aus der
die Container- und Delivery-Artefakte entstehen.

## Gegenseitiger Ausschluss (harte Korrektheitsregel)

Organisations-Rule-Sets aggregieren untereinander und mit
Repository-Rule-Sets; das Aggregat kann nur restriktiver werden, niemals
schwächer. Ein „allgemeines `~ALL`-Rule-Set plus ein zusätzliches schwächeres
Klassen-Rule-Set" würde deshalb auf den Klassen-Repositories fail-closed
scheitern, weil die allgemeine Variante sie weiterhin bindet.
Klassen-Varianten derselben Governance-Fläche MÜSSEN die Flotte in sich
gegenseitig ausschließende Selektor-Klassen partitionieren
(`quality-gates=full` versus `quality-gates=linux-only`), und kein Repository
darf beiden Klassen angehören.

## Selektor-Formen der Organisationsebene

- `repository_name`: fnmatch-Muster, `~ALL` für alle Repositories, optionale
  Ausschlussliste und Renaming-Schutz-Flag.
- `repository_id`: explizite Repository-IDs (manuell, stabil, aber
  unhandlich bei Flottenwachstum).
- `repository_property`: dynamische Klassen- oder System-Properties;
  die Mitgliedschaft folgt automatisch jeder Property-Änderung am Repository.
  Dies ist der kanonische Mechanismus für klassenbasierte Differenzierung.

## Globale Flottenadressierung ist eine explizite Option

Ein Organisations-Rule-Set gilt nur dann für alle Repositories, wenn der
Selektor das ausdrücklich sagt: grafisch die Targeting-Wahl **All
repositories**, im REST-Schema `repository_name.include: ["~ALL"]`. Ein
fehlender oder leerer Selektor bedeutet niemals implizit „alle"; das
Organisations-Schema verlangt genau einen expliziten Selektor pro Rule-Set.
Die `~ALL`-Auswahl ist dynamisch: Sie bindet jedes gegenwärtige und jedes
künftige Repository ab dessen Erstellung, ohne Re-Import.

## Selektor der Push-Protections

Das Push-Rule-Set verwendet die System-Property `visibility` mit den Werten
`private` und `internal`, weil Push-Rule-Sets nur dort existieren. Vor dem
Import MUSS die Selektierbarkeit dieser System-Property im Ziel-Account
verifiziert werden; andernfalls ist die dokumentierte Fallback-Form die
explizite `repository_name`-Selektion der privaten Repositories.
