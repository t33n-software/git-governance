# Hosting-Plattform: GitHub — Custom Properties
[INTENT: SPEZIFIKATION]

## Kanonische Quelle

Die Organisations-Custom-Properties für `t33n-software` werden einmalig und
zentral in diesem Repository unter
[`properties/github/`](../../../../../properties/github/README.md) definiert
und verwaltet. Rule-Sets **referenzieren** Properties über den
`repository_property`-Selektor; sie definieren sie niemals. Die Definition ist
ein eigenes, versioniertes Artefakt und wird über denselben signierten und
attestierten Release-Kanal wie die Rule-Set-Familie verteilt.

Eine Definition oder Wertezuweisung außerhalb dieses Vertrags — insbesondere
die manuelle Pflege in den Organization Settings — ist ein Anti-Pattern und
verboten (Redundanz- und Drift-Verbot).

## Drei-Schichten-Vertrag
[INTENT: ARCHITEKTUR]

1. **Kanonische Definition (öffentlich, dieses Repository).** Jede von einem
   Rule-Set-Selektor konsumierte Property existiert genau einmal als
   versioniertes Artefakt `properties/github/<name>.json` im REST-Schema der
   Property-Definition (`value_type`, `allowed_values`, `required`,
   `default_value`, `description`, `values_editable_by`). Contract-Tests
   beweisen, dass die `allowed_values` exakt der Klassenpartition der
   Rule-Set-Selektoren und Dateinamen entsprechen.
2. **Organisations-Bindung (privat, Plattform-Instanz).** Die Instanz pinnt
   die Definition und bindet jede Repository-zu-Wert-Zuweisung unter
   `policy-overlays/hosting-platforms/github/properties/`
   (`definitions.yaml`, `assignments.yaml`, `enforcement.yaml`). Die Instanz
   authored niemals eine Definition; sie pinnt und bindet — exakt wie der
   Rule-Set-Bundle-Pin.
3. **Projektion (reviewte IaC-Lane).** Definition und Zuweisungen werden über
   die OpenTofu-Standardlane projiziert: ein wertefreies, generisches Modul
   des Developer-Platform-Infrastructure-Core, ausgeführt mit einer
   Organisations-Owner-Identität aus der Instanz-Lane. Die grafische
   Oberfläche ist ausschließlich Lese- und Verifikationsfläche; eine manuelle
   Mutation ist Drift gegen die gepinnten Bindings und wird auf sie
   zurückgeführt.

## Positive-List-Konvention
[INTENT: ANWEISUNG]

Eine Custom Property DARF nur existieren, wenn alle vier Vorbedingungen
erfüllt sind:

1. ein benannter GitHub-nativer Konsument (Rule-Set-Selektor oder
   Repository-Policy-Rule-Set),
2. eine kanonische Definition in `properties/github/`,
3. eine Instanz-Bindung,
4. eine drift-gebundene Verifikation.

Reine Betriebs-Metadaten ohne Rule-Set-Konsumenten — beispielsweise
`databases`, `backup-required` oder `runtime-environment` — DÜRFEN keine
GitHub-Custom-Properties werden. Ihre kanonische Heimat ist der zuständige
Bereich der Plattform-Instanz oder der Policy-Registry; CI/CD konsumiert sie
von dort. Metadaten nur in den Organization Settings zu definieren erzeugt
eine unversionierte Drift-Fläche und ist verboten.

## Werte-Editierbarkeit (harte Grenze)
[INTENT: CONSTRAINT]

Jede governete Property setzt `values_editable_by: org_actors`. Bei
`org_and_repo_actors` könnte ein Repository-Administrator sein eigenes
Repository in eine schwächere Rule-Set-Klasse verschieben (beispielsweise
`quality-gates` von `full` auf `linux-only`) und damit seine eigenen
Merge-Gates schwächen. Klassenmitgliedschaft ist eine
Governance-Entscheidung, niemals eine Repository-lokale.

## Onboarding-Wert `pending`
[INTENT: SPEZIFIKATION]

Die Property `quality-gates` trägt den dritten erlaubten Wert `pending` als
`default_value`. Ein Repository, dessen Workflows die erforderlichen
Check-Kontexte seiner Klasse noch nicht nachweislich emittieren, wird
`pending` zugewiesen, statt property-los zu bleiben: Der
Klassifizierungszustand ist explizit, auditierbar und durchsetzbar statt
implizit abwesend. Kein Shared-Line-Rule-Set targetet `pending`; solche
Repositories bleiben nur an die klassenlosen Rule-Sets `00` und `01` gebunden,
bis ihre Workflows aligned sind.

## Aktivierungssequenz
[INTENT: ANWEISUNG]

Das Property-Schema und die initialen Zuweisungen MÜSSEN projiziert werden,
bevor die klassen-partitionierten Shared-Line-Rule-Sets `02`–`05` auf aktiv
geschaltet werden. Ein Klassen-Rule-Set, dessen Selektor eine fehlende oder
unzugewiesene Property referenziert, bindet null Repositories — ein stilles
Fail-open der gesamten Shared-Line-Fläche.

## Klassifizierungs-Durchsetzung (deferred, entitlement-gebunden)
[INTENT: SPEZIFIKATION]

Ein Organisations-Rule-Set mit Target `repository` (Repository-Policy-Rule-Set)
KANN die `quality-gates`-Klassifizierung erzwingen, sodass kein governetes
Repository unklassifiziert bleibt. Diese Fähigkeit ist an das GitHub-Enterprise-Cloud-Entitlement
gebunden und bis dahin deferred. Bis dahin ist die äquivalente Kontrolle der
Drift- und Klassifizierungs-Report des Instanz-Verifiers und des
Policy-Validators. Bei Aktivierung MUSS das Rule-Set `pending` als
konformen Onboarding-Wert behandeln, damit die dokumentierte
Workflow-Alignment-Vorbedingung nicht gebrochen wird.
