# Hosting-Plattform: GitHub — workflows — Provider-Fehlerdiagnostik
[INTENT: REFERENZ]

## Kanonische Quelle

Diese Datei ist die kanonische Quelle der Wahrheit für die Konvention, wie die
Release- und Hotfix-Lifecycle-Familie die Fehlerdiagnostik des Providers in der
CLI-Oberfläche sichtbar macht. Der Business-Code re-referenziert diese Datei als
Autorität und trägt an der betroffenen Stelle nur den knappen Verweis, dass —
und warum — keine Redaction erfolgt; die vollständige Begründung lebt ausschließlich
hier.

## Was vom Provider zurückkommt

Die Lifecycle-Lanes rufen die REST-API des Providers (GitHub). Eine
fehlgeschlagene Operation liefert das Fehler-Envelope mit diesen Properties:

- `message`: der vom Provider erzeugte Klartext-Diagnosetext der Operation.
- `errors[]`: optionale Validierungseinträge mit `resource`, `field`, `code`
  und einer Detail-`message`.
- `documentation_url`: ein öffentlicher Verweis auf die Provider-Dokumentation.
- `status`: der Status-Code.

Keines dieser Properties enthält Credentials, Tokens, Private Keys,
Authorization-Header oder Secrets. Die Erfolgs-Response trägt ausschließlich
öffentliche Metadaten (Refs, SHAs, URLs, User- und App-Stammdaten inklusive der
öffentlichen `client_id`). Die Request-Seite — der Authorization-Header und der
Request-Payload — wird von der Diagnose-Oberfläche niemals gelesen oder
ausgegeben.

## Warum keine Redaction erfolgt

Die Diagnose-Oberfläche gibt den Provider-Diagnosetext unverändert wieder. Es
erfolgt bewusst **keine** Redaction, weil der Kanal per Konstruktion secret-frei
ist — nicht weil ein Netz ihn nachträglich säubert:

1. Die Request an den Provider ist vollständig typisiert und besitzt kein
   Freitext-Feld, in das ein Secret gelangen könnte; sie trägt ausschließlich
   Refs, SHAs, benannte Konstanten und den Request-Record.
2. Das Credential lebt ausschließlich im Authorization-Header; es wird niemals
   in den Body geschrieben und vom Provider niemals zurückgespiegelt.
3. Der `message`-Text des Providers ist ein öffentlicher Diagnosetext über die
   API-Operation und enthält keine Credentials.

Die 100-%-Sicherheit wird also nicht durch Aufzählung möglicher Inhalte
erreicht, sondern durch den Konstruktionsbeweis: secret-freie Request,
header-isoliertes Credential, provider-seitiger Diagnosetext. Ein
Redaction-Mechanismus über diesem Kanal wäre eine nicht tragfähige Schicht:
Pattern-Matching ist notwendigerweise unvollständig, und über einem Kanal, der
per Konstruktion keine Secrets führt, trägt es keine Kontrollkraft. Die
Konvention lehnt eine solche Schicht ab und begründet die Sicherheit über die
Konstruktion.

## Quellen-Re-Referenzierung

Die verbindliche Quelle für das Fehler-Envelope und seine Properties ist die
offizielle REST-API-Dokumentation des Providers (GitHub) für die Deployments- und
Fehler-Oberfläche: sie definiert die Status-Codes (201, 202, 409, 422) und das
Fehler-Envelope mit `message`, `errors[]` (mit `resource`, `field`, `code`,
`message`) und `documentation_url`. Diese Konvention re-referenziert diesen
Vertrag, statt ihn erneut zu definieren.

## Vertrag für den Business-Code

- Die Diagnose-Oberfläche liest ein längengebundenes Präfix des Response-Body,
  extrahiert ausschließlich `message` und `errors[].message` (bei Nicht-JSON den
  rohen Text) und schreibt das Ergebnis in das nicht-sensitive
  `Diagnostic`-Feld.
- Der Kommentar an der betroffenen Stelle hält nur fest, dass keine Redaction
  erfolgt, und re-referenziert diese Datei als kanonische Quelle; er trägt
  bewusst nicht die vollständige Begründung.
- Niemals werden Request- oder Response-Header, Tokens, der Request-Payload oder
  irgendein Secret gelesen oder ausgegeben.
