# Code-Quality- und Coverage-Gates in Organisationseigentum
[INTENT: ANWEISUNG]

## Konvention

Die Organisation MUSS ihre eigenen Code-Quality- und Code-Coverage-Gates
definieren und durchsetzen, und diese Gates MÜSSEN an der Vertragsoberfläche
programmiersprachenagnostisch sein. Das Quality- und Coverage-Gate DARF
NICHT an die Hosting-Plattform übergeben werden. Die Hosting-Controls
`code_quality` und `code_coverage` DÜRFEN weder in den versionierten
Rule-Set-Quellen erscheinen noch Organisations-Default werden.

## Die vier Schichten der Konvention

1. **Vertragsoberfläche (sprachagnostisch).** Die Organisations-rulesets
   fordern nur stabile, identische Check-Kontexte — in der komponierten Ära
   die Composite-Form `Quality gates / linux-amd64` und
   `Dependency review / Dependency admission review` (Klasse `linux-only`;
   die Klasse `full` trägt bis zur Caller-Adoption ihrer Repositories die
   Inline-Ära-Form `Quality gates (<os>)` plus `Dependency admission
   review`). Jedes Repository — Go, Python, Node.js oder ein künftiges
   Ökosystem — emittiert dieselben Kontexte; der organisationsweite Vertrag
   nennt niemals ein sprachspezifisches Werkzeug.
2. **Gate-Inhalt (produzenten-eigen).** Das eigene Build-Gate jedes
   Repositorys berechnet Quality und Coverage mit seiner nativen Toolchain —
   zum Beispiel golangci-lint plus exaktem 100-%-Statement-Coverage-Check für
   Go, ruff plus coverage.py für Python, eslint plus c8 für Node.js. Evidenz
   wird am autoritativen Produzenten erzeugt und erzwungen: der
   Sprach-Toolchain, die sie im Build berechnet.
3. **Hoheitsgrenze.** Die Definition dessen, was als Quality- oder
   Coverage-Verstoß gilt, ist organisations-eigen, versioniert und reviewbar
   im Repository. Ein Hosting-Gate würde Merges den generischen
   Preview-Heuristiken der Plattform unterwerfen: Es könnte fail-closed auf
   Befunden blockieren, die die Organisation nicht als Defekt einstuft; es
   würde die Organisation auf die Quality-Konvention der Plattform deckeln
   statt auf die strengeren projektspezifischen Kriterien; und sein
   Regelzustand läge außerhalb der versionierten JSON-Quellen als
   nicht-auditierbare, UI-only Drift-Fläche.
4. **Ausschluss der Hosting-Controls.** Beide Controls fehlen im
   importierbaren Rule-Set-Schema und im REST-Rule-Vokabular (gegen die
   Organisations- und Repository-Schemas verifiziert); beide sind hinter
   GitHub Code Quality in Public Preview feature-gated; Coverage ist
   zusätzlich redundant, weil das exakte 100-%-Statement-Coverage-Gate im
   Required Check bereits eine strengere Grenze vom selben
   CI-Evidenz-Produzenten erzwingt.

## Entscheidungsprotokoll (normalisiert)

| Option | Anteil | Einordnung |
|---|---:|---|
| Organisations-eigene Gates | 50 % | Einzige Form mit Produzenten-Autorität, Sprach-Uniformität, Hoheit über die Fehlerdefinition und Fail-Closed-Sicherheit |
| Hosting-Plattform-Controls | 26 % | Scheitert am Portabilitäts-Gate (nicht versionierbar) und am Hoheits-Gate |
| Duale Durchsetzung (beide) | 24 % | Scheitert am Redundanz-Gate: gleiche Evidenz, kein unabhängiger Produzent |

## Sicherheit des projekt-eigenen Modells

Das projekt-eigene Modell bleibt sicher, weil das Organisations-Rule-Set den
Kontext als Merge-Bedingung erzwingt und jede Änderung am emittierenden
Workflow dieselbe Review- und Code-Owner-Grenze passiert. Existenz und
Grün-Status des Gates sind damit organisations-erzwungen, während der Inhalt
projekt-autoritativ bleibt.

## Ausnahme und Revisit

Ein einzelnes Repository DARF GitHub Code Quality über die UI als benannte,
auditierbare Ausnahme aktivieren — nach Verifikation von Verfügbarkeit, Plan
und tatsächlichen PR-Ergebnissen — niemals als Flotten-Default und niemals in
den versionierten Quellen. Die Neubewertung dieses Ausschlusses erfolgt erst,
wenn alle Revisit-Trigger feuern: Die Controls betreten das importierbare
Schema, das Feature verlässt die Preview, und — für Coverage — der
Evidenz-Produzent ist unabhängig vom bestehenden Quality-Gate. Die erwartete
Landezone wären dann ausschließlich die Shared-Line-Familien in beiden
Klassen, aktiviert über `evaluate` vor `active`.
