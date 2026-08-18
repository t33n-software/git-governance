# Push-Protections: Grenze gegen Secret-förmige Artefakte
[INTENT: SPEZIFIKATION]

Das Rule-Set `00-push-protections.json` blockiert secret- und
schlüsselförmige Artefakte, bevor sie den Commit-Graphen erreichen. Das
Push-Ziel gilt für jeden Push in das Repository und dessen gesamtes
Fork-Netzwerk; es gibt keine Branch-Bindung.

## Verfügbarkeitsgrenze

Push-Rule-Sets existieren nur für **private und interne** Repositories und
erfordern den Team-Plan. Öffentliche Repositories können dieses Rule-Set
nicht tragen; ihre Grenze ist Secret-Scanning mit Push-Protection plus die
lokalen Quality-Gates. Der Selektor bindet deshalb über die
System-Property `visibility` nur private und interne Repositories. Die Datei
bleibt trotzdem Teil der kanonischen Menge: Sie ist die versionierte
Definition der Grenze, unabhängig davon, wo sie aktivierbar ist.

## Geblockte Artefaktklassen

- Schlüssel- und Keystore-Dateiendungen: `*.pem`, `*.key`, `*.p12`, `*.pfx`,
  `*.jks`, `*.keystore`, `*.kdbx`, `*.ppk`, `*.gpg`
- Umgebungs- und Credential-Dateien sowie Infrastruktur-Zustände:
  `**/.env`, `**/.env.*`, `**/.envrc`, `**/credentials`, `**/credentials.*`,
  `**/*.tfstate`, `**/*.tfstate.*`

Diese Klassen sind die Artefakttypen, die in dieser Architektur
Credential-Material tragen. Die Liste ist ein sicherer Default; eine
Erweiterung erfolgt nur über eine reviewte Änderung im kanonischen
Repository.

## Warum die gesamte `.env`-Familie blockiert ist

Secret-Material ist unabhängig vom Datei-Suffix. `.env.development`,
`.env.test` und jede künftige punktierte Variante können produktive
Credentials tragen, und committete Umgebungsdateien sind ein primäres
Harvest-Ziel. Die Wildcard `**/.env.*` ist deshalb fail-closed gegen jede
gegenwärtige und künftige Variante; `**/.envrc` erweitert die Grenze auf
direnv-Dateien. Da die Pfad-Restriktion keine Ausschlussliste kennt, ist das
Absicht — und der Grund, warum die Template-Konvention außerhalb der
blockierten Familie liegt.

## Template-Architektur

Teilbare, nicht-geheime Umgebungs-Defaults MÜSSEN außerhalb der blockierten
Familie liegen: Namen wie `env.example`, `example.env` oder ein
`templates/env.*`-Verzeichnis, ausschließlich mit Platzhalterwerten. Echte
Werte kommen aus dem Secret-Manager oder Credential-Broker; lokale
Überschreibungen bleiben in `.env*.local`, die sowohl gitignored als auch
push-blockiert sind.

## Defense in Depth

- Die Push-Regel verhindert serverseitig, inklusive des Fork-Netzwerks.
- `.gitignore` verhindert clientseitig versehentliches Staging.
- Secret-Scanning mit Push-Protection bleibt die detektive Ebene — auf
  öffentlichen Repositories die einzige, weil dort kein Push-Rule-Set
  existieren kann.
