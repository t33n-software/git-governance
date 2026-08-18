# Import, Aktivierung und Verifikation
[INTENT: ANFORDERUNG]

## Voraussetzungen

1. Die Organisation trägt mindestens den **Team-Plan**; die
   Organisations-Ruleset-Oberfläche existiert sonst nicht, und
   Push-Rule-Sets für private Repositories bleiben davon ebenfalls abhängig.
2. Die Custom Property `quality-gates` ist auf Organisationsebene mit den
   Werten `full` und `linux-only` angelegt, und jedes governed Repository
   trägt genau einen der beiden Werte.
3. `.github/CODEOWNERS` ist gemergt, bevor Shared-Line-Rule-Sets mit
   `require_code_owner_review` aktiv werden; ohne den Vertrag blockiert jede
   Shared-Line-PR fail-closed.
4. Ein Required-Check-Kontext wird erst dann auf einer Live-Linie erzwungen,
   wenn der emittierende Workflow nachweislich auf einem echten PR gegen
   exakt diese Ziel-Linie reportet — inklusive des Pfads ohne Änderung, der
   erfolgreich statt ausstehend melden MUSS.

## Import-Reihenfolge

1. `00-push-protections.json` (bindet nur private/interne Repositories)
2. `01-ticket-working-branches.json`
3. `02-develop.*` (beide Klassen)
4. `03-main.*` (beide Klassen)
5. `04-release.*` (beide Klassen; `do_not_enforce_on_create: true` prüfen)
6. `05-support.*` (beide Klassen; `do_not_enforce_on_create: true` prüfen)

Grafisch über **Organization Settings → Repository → Rulesets → New ruleset
→ Import a ruleset**, oder programmatisch über `POST /orgs/{org}/rulesets`
mit einer Organisations-Owner-Identität. Token werden niemals über
Projektdateien, Kommando-Historie oder Quellcode weitergegeben.

## Aktivierung

- Neue oder geänderte Organisations-Rule-Sets starten im Status
  **Evaluate**; die Rule-Insights-Auswertung zeigt, was passiert wäre.
- Erst nach sauberer Evaluation wird auf **Active** umgeschaltet.
- Ein Import allein mutiert keinen Bestand: Bereits aktive Rule-Sets werden
  über die UI oder die REST-API aktualisiert; nach jeder gemergten Änderung
  an den kanonischen Quellen wird re-importiert.
- Bypass-Actors werden nur für einen auditierten Release- oder
  Notfall-Prozess konfiguriert, niemals als bequemer Erstellungsweg.

## Verifikations-Checkliste

- Ein gemergter `feature/*`-PR löscht seinen Remote-Head-Branch.
- Der Löschschutz verhindert die Löschung von `main`, `develop`,
  `release/*` und `support/*`.
- Ein `develop`-PR erlaubt nur merge, rebase und squash; ein PR auf `main`,
  `release/*` oder `support/*` erlaubt nur Merge-Commits.
- Ein Shared-Line-PR ohne Freigabe des gebundenen CODEOWNERS-Owners ist
  blockiert; offene Review-Threads blockieren ebenfalls.
- Die Required Checks reporten auf jeder Shared-Line-PR und blockieren bei
  Fehlschlag; ein PR ohne Änderung meldet Erfolg statt „pending".
- CodeQL-Ergebnisse liegen vor und blockieren.
- Auf einem privaten Repository lehnt das Push-Rule-Set einen Push mit einem
  secret- oder schlüsselförmigen Artefakt (zum Beispiel `.pem` oder `.env`)
  ab — inklusive Fork-Netzwerk.
- Kein Repository trägt eine Kopie eines allgemeinen Rule-Sets außerhalb
  eines benannten Ausnahmeszenarios; jedes allgemeine Rule-Set existiert
  genau einmal auf Organisationsebene.
- Die Sichtbarkeits-Selektion des Push-Rule-Sets und die
  Klassen-Selektoren adressieren exakt die vorgesehenen Repositories.
