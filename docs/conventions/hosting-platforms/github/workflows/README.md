# Hosting-Plattform: GitHub — workflows
[INTENT: REFERENZ]

## Kanonische Quelle

Die Release- und Hotfix-Lifecycle-Workflow-Familie für die Organisation
`t33n-software` wird einmalig und zentral in diesem Repository definiert und
verwaltet. Dieses Repository ist die kanonische Quelle der Wahrheit für die
Familie: Es erklärt die Architektur, trägt die Payloads und liefert die
versionierten, referenzierbaren Artefakte.

Eine lokale Kopie, Neudefinition oder Abweichung in einem anderen Repository
ist ein Anti-Pattern und verboten (Redundanz- und Drift-Verbot). Ein Tenant
adoptiert die Familie ausschließlich über die dünnen, hash-verifizierten
Caller.

## Ablage der Artefakte

- Die **Payloads** liegen an der plattform-erzwungenen Ausführungsstelle
  [`.github/workflows/`](../../../../../.github/workflows/) und tragen das
  Präfix `reusable-`: `reusable-<capability>.yml`. GitHub liest
  wiederverwendbare Workflows ausschließlich aus diesem Ort; die Ablage ist
  keine Wahl, sondern Plattform-Zwang.
- Die **kanonischen Caller-Master** und der **Familien-Arbeitsvertrag** liegen
  im Familien-Baum
  [`workflows/github/`](../../../../../workflows/github/CONTRACT.md) — die
  Familie ist die zweckbenannte Wurzel, die Plattform (`github/`) liegt eine
  Ebene darunter, gespiegelt zu `rulesets/github/` und `properties/github/`.

Begründung der Achsen-Form: Dieses Repository ist das Domain-Home des
Git-Lifecycle; seine Wurzel bildet die primären Artefakt-Ebenen ab
(`rulesets/`, `properties/`, `workflows/`, die CLI). Die Hosting-Plattform ist
die Skalierungsachse eine Ebene darunter, niemals ein Gruppierungs-Elternteil
darüber.

## Namenskonvention

- **Payloads:** `reusable-<capability>.yml`, ausschließlich
  `on: workflow_call`, niemals selbst-triggernd. Das Präfix kodiert die Rolle
  und verhindert die Namenskollision mit dem Caller im selben Verzeichnis.
- **Caller:** die Caller-Datei behält ihre kanonische Identität
  (`release-control.yml`, `execute-protected-line-request.yml`,
  `tag-promoted-release.yml`, `publish-release-artifacts.yml`,
  `release-reconciliation.yml`, `hotfix-delivery.yml`,
  `hotfix-propagation.yml`) — Workflow-Identität ist Vertrag, weil die CLI und
  die Flotten-Pins an sie binden.
- **Check-Kontext:** ein Reusable-Aufruf emittiert den Kontext als Composite
  `<Caller-Job-Name> / <Callee-Job-Name>`; der Caller-Job trägt die
  Lane-Identität, der Callee-Job trägt die Gate-Identität.
- **Composite Actions:** `.github/actions/<verb>-<object>/action.yml`.

## Convention documents

- [Provider-Fehlerdiagnostik](provider-fehlerdiagnostik.md) — how the
  lifecycle lanes surface the provider's error diagnostic without redaction.
- [Environment-gate trigger context](environment-gate-trigger-context.md) —
  an environment-gated job never runs on a `pull_request` run; event-driven
  lanes detect on the pull request and execute on the main-bound dispatch.
- [Canonical copies, never symlinks](canonical-copies-not-symlinks.md) — why
  every execution location carries a byte-identical regular-file copy of the
  canonical caller and why a symbolic link cannot work.

## Verwaltung

- Verwaltungsebene: die **Organisation** (`t33n-software`), niemals die
  einzelne Repository-Ebene; die Familie wird einmalig in diesem Home
  gepflegt und von Tenants per SHA-Pin referenziert.
- Die Delivery-Variante (`cloud` oder `github-only`) ist Daten — gebunden über
  die typisierte Eingabe, niemals über eine geforkte Payload.
- Die sieben Workflow-Dateien dieses Repositorys unter `.github/workflows/`
  sind byte-identisch zu den kanonischen Master-Callern (Dogfooding); der
  Contract-Test-Satz beweist die Identität bei jeder Änderung.
- Jede Abweichung eines Tenants vom kanonischen Caller ist ein Hash-Mismatch
  und schlägt fail-closed fehl.
