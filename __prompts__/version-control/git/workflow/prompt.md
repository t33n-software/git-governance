---
description: Governed git-governance workflow entrypoint for this repository - activates the adapter contract, loads the portable core and binds the Go source entrypoint for all branch, commit, release and hotfix work
alwaysApply: true
---

# Git-Governance Source-Repository Adapter

## [0] ADAPTER-AUTORITÄT UND AKTIVERUNGSVERTRAG
[INTENT: ANWEISUNG]

Diese Datei ist die projektspezifische Adapterhülle für das
`git-governance`-Quellrepository. Sie ist kein zweiter Workflow-Kern und
enthält keine duplizierte Branch-, Commit-, Release-, Hotfix- oder
CLI-Policy.

Der portable, vollständige Workflow-Kern liegt relativ zu dieser Datei unter:

```text
core/prompt.md
```

### 0.1 Aktivierungsvertrag

1. Die Präsenz dieser Datei im Kontext des Agents — als injizierte Regel, als
   angehängte Datei oder als gelesener Inhalt — aktiviert diesen Vertrag
   sofort und verbindlich für die laufende Sitzung. Der Adapter ist niemals
   passive Referenzliteratur.
2. Jede Aufgabe in diesem Repository, die in eine Git-Wirkung mündet oder
   münden kann — einschließlich jeder Datei-Erstellung, -Bearbeitung oder
   -Löschung — wird ausschließlich über die Zustandskette in [2] in den
   Core-Workflow geführt. Es gibt keinen zweiten Einstiegspfad.
3. Der erste sichtbare Schritt des Agents nach Aktivierung ist die
   Adapter-Initialisierung. Der Agent beginnt weder mit der fachlichen
   Analyse der Benutzeraufgabe noch mit Datei-Suche, Datei-Edits,
   Commit-Vorbereitung oder einem manuellen Workflow-Nachbau.
4. Ein Auslassen der Adapter-Initialisierung ist kein alternativer Pfad,
   sondern ein Vertragsbruch. Erkennt der Agent nachträglich, dass er ohne
   abgeschlossene Initialisierung gearbeitet hat, stoppt er, meldet die
   Prozessverletzung dem Benutzer offen und führt die Initialisierung nach,
   bevor irgendeine weitere Mutation erfolgt.
5. Höher priorisierte System-, Sicherheits-, Benutzer- und Repository-Regeln
   bleiben verbindlich. Dieser Adapter ersetzt keine solche Regel und erfindet
   keine Tool-Fähigkeit.

### 0.2 Core-Lade-Mandat

Vor jeder Analyse, Entscheidung, Tool-Verwendung oder Mutation MUSS der Agent:

1. den relativen Pfad von dieser Adapterdatei auflösen;
2. `core/prompt.md` vollständig lesen;
3. den vollständigen Inhalt als `CORE_WORKFLOW_CONTRACT` übernehmen;
4. diesen Adapter nur als Laufzeit- und Binary-Bindung anwenden.

Wenn der Core nicht existiert, nicht vollständig lesbar ist oder sein Inhalt
nicht als Workflowvertrag übernommen werden kann, ist der Zustand `BLOCKED`.
Der Agent darf keinen verkürzten Ersatz, keine externe Dokumentation und keine
ältere Prompt-Kopie verwenden.

## [1] ZUSTANDSMODELL, NACHWEISE UND ÜBERGÄNGE
[INTENT: ANWEISUNG]

### 1.1 Zustandsautomat

```text
ADAPTER_ACTIVATED
-> CORE_PATH_RESOLVED
-> CORE_FULLY_LOADED
-> CORE_CONTRACT_BOUND
-> SOURCE_ENTRYPOINT_VERIFIED
-> CORE_WORKFLOW_EXECUTING
-> ADAPTER_COMPLETE
```

Ausnahmezustand:

```text
BLOCKED
```

### 1.2 Laufzeit-Oberflächen

Der Agent führt diese Zustandsflächen jederzeit explizit:

```text
- adapter_activation_state = inactive | active
- core_load_state = unresolved | path_resolved | fully_loaded | contract_bound
- entrypoint_state = unverified | verified | failed
- delegation_state = inactive | active
- adapter_completion_state = pending | reported
```

### 1.3 Mindestnachweise

Jeder Übergang ist erst erlaubt, wenn sein Nachweis als explizite
Arbeitsoberfläche vorliegt:

```text
- adapter_presence_acknowledged
- core_path_resolved
- core_fully_read
- core_contract_bound
- source_entrypoint_help_verified
- delegation_active
- non_duplication_verified
- adapter_completion_reported
```

Ein Übergang ohne seinen Nachweis ist verboten. Ein Nachweis ersetzt keinen
späteren Nachweis: `core_fully_read` beweist nicht den Entrypoint, und
`source_entrypoint_help_verified` beweist keine Delegationsdisziplin.

### 1.4 Pre-Action-Embargo

Vor dem Zustand `CORE_WORKFLOW_EXECUTING` sind ausschließlich diese
Bootstrap-Operationen erlaubt:

```text
- den relativen Core-Pfad auflösen und die Core-Datei lesen;
- die Source-Entrypoint-Prüfung gemäß [3.2] ausführen;
- Bootstrap-Statusmeldungen mit dem Symbol 🔌 ausgeben.
```

Jede andere Operation — fachliche Analyse der Benutzeraufgabe, Datei-Suche
über den Core-Pfad hinaus, Datei-Edits, Staging, Commits, Branch-Operationen
oder rohes Git — ist bis `CORE_WORKFLOW_EXECUTING` embargoed.

### 1.5 Invalidierung und Neuankerung

Der gebundene Zustand verliert seine Gültigkeit, und der Agent kehrt zum
frühesten betroffenen Zustand zurück, wenn:

```text
- die Core-Datei während der Sitzung verändert wurde oder ihr Inhalt nicht
  mehr vollständig dem gebundenen Vertrag entspricht
  -> Rückkehr zu ADAPTER_ACTIVATED, Core erneut vollständig lesen;
- der Source-Entrypoint nach zuvor erfolgreicher Prüfung fehlschlägt, etwa
  weil eigene Edits am CLI-Quellcode die Ausführung gebrochen haben
  -> entrypoint_state = failed, Zustand BLOCKED bis zur erneuten
     Help-first-Verifikation;
- die Sitzung oder der Repository-Kontext gewechselt wird
  -> vollständige Neuinitialisierung ab ADAPTER_ACTIVATED.
```

Eine zwischengespeicherte Hilfe, ein früher gelesener Core-Stand oder eine
frühere Entrypoint-Verifikation wird nach einer Invalidierung niemals
wiederverwendet.

## [2] PROJEKTSPEZIFISCHE BINARY-BINDUNG
[INTENT: ANWEISUNG]

Der portable Core verwendet die logische Binary-Oberfläche:

```text
git-governance <endpoint> ...
```

In diesem Quellrepository wird jeder vom Core verlangte Binary-Aufruf exakt
über den Source-Entrypoint ausgeführt:

```text
go run -mod=readonly ./cmd/git-governance <endpoint> ...
```

Diese Bindung ersetzt nur das ausführbare Präfix. Sie darf weder:

```text
- einen Endpoint umbenennen;
- Argumente, Flags oder Werte ergänzen;
- Flags, Regexe oder technische Limits hart kodieren;
- Help-Reanchoring überspringen;
- einen CLI-Workflow durch rohe Git-, GitHub- oder Shell-Orchestrierung ersetzen;
- den Core durch Repository-Dokumentation oder Quellcode substituieren.
```

Für jeden Core-Aufruf gilt deshalb projektlokal:

```text
Core Help:
git-governance <endpoint> --help

Adapter Help:
go run -mod=readonly ./cmd/git-governance <endpoint> --help

Core Runtime:
git-governance <endpoint> <ausschließlich-aus-help-abgeleitete-Eingaben>

Adapter Runtime:
go run -mod=readonly ./cmd/git-governance <endpoint> <dieselben-ausschließlich-aus-help-abgeleiteten-Eingaben>
```

Globale Ausführungsmodi, Ausgabeformate, Dry-Run-Verhalten,
Mutationsbestätigungen, Provider-Auswahl, Zeitlimits und alle Endpoint-Flags
werden bei jeder Invocation aus der aktuellen Root- beziehungsweise
Endpoint-Hilfe abgeleitet. Diese Datei speichert sie absichtlich nicht.

## [3] ADAPTER-INITIALISIERUNG
[INTENT: ANWEISUNG]

### 3.1 Relative Core-Auflösung

Die Auflösung erfolgt ausschließlich relativ zum Verzeichnis dieser Datei:

```text
workflow/
├── prompt.md
└── core/
    └── prompt.md
```

Absolute Pfade, benutzerspezifische Home-Verzeichnisse, Laufwerksbuchstaben,
externe Projekte, Repository-Dokumentation und RAG-Quellen sind keine
Laufzeitabhängigkeit dieses Prompts.

Der Übergang `ADAPTER_ACTIVATED -> CORE_PATH_RESOLVED` setzt den Nachweis
`core_path_resolved` voraus: Die Core-Datei existiert am relativen Pfad und
ist lesbar.

### 3.2 Vollständiges Core-Lesen und Vertragsbindung

Der Agent liest `core/prompt.md` vollständig — nicht auszugsweise, nicht
zusammengefasst, nicht aus dem Gedächtnis. Der Übergang
`CORE_PATH_RESOLVED -> CORE_FULLY_LOADED` setzt `core_fully_read` voraus; der
Übergang `CORE_FULLY_LOADED -> CORE_CONTRACT_BOUND` setzt voraus, dass der
vollständige Inhalt als `CORE_WORKFLOW_CONTRACT` übernommen wurde
(`core_contract_bound`).

Ist die Datei nicht vollständig lesbar oder kann der Inhalt nicht als Vertrag
gebunden werden, ist der Zustand `BLOCKED`; es gibt keinen Ersatz über externe
Dokumentation, Quellcode-Lektüre oder frühere Prompt-Kopien.

### 3.3 Source-Entrypoint prüfen

Nach vollständiger Vertragsbindung prüft der Agent über die im Core definierte
Help-first-Sequenz, ob dieser Quellrepository-Entrypoint verfügbar ist:

```text
go run -mod=readonly ./cmd/git-governance --help
```

Der Übergang `CORE_CONTRACT_BOUND -> SOURCE_ENTRYPOINT_VERIFIED` setzt den
Nachweis `source_entrypoint_help_verified` voraus: Die Hilfe wurde erfolgreich
ausgeführt und zeigt die erwartete CLI-Oberfläche. Fehlt Go, kann das Modul
nicht ausgeführt werden oder liefert der Entrypoint keine gültige CLI-Hilfe,
bleibt der Workflow `BLOCKED`; der Agent baut weder ein Fallback-Binary noch
verwendet er einen anderen Git- oder GitHub-Client als Ersatz.

Erst mit dem Übergang nach `CORE_WORKFLOW_EXECUTING` wird
`delegation_state = active` gesetzt und das Pre-Action-Embargo aus [1.4]
aufgehoben.

## [4] CORE-DELEGATION
[INTENT: ANWEISUNG]

Nach `SOURCE_ENTRYPOINT_VERIFIED` gelten alle weiteren Entscheidungen,
Zustandsübergänge, Entscheidungsmatrizen, Shared-Line-Guards,
Mutations-Embargos, Ausführungsebenen, Scratch-Entscheidungen,
Endpoint-Auswahl, Quality-Gates, Commit-Regeln, Release- und Hotfix-Grenzen
ausschließlich aus `CORE_WORKFLOW_CONTRACT`.

Insbesondere übernimmt der Agent aus dem Core unverändert:

```text
- Help-first-Laufzeitvertrag;
- Audit- und sichtbare Statuskommunikation einschließlich Bereichs-Symbole;
- Branch-Kontext-Prüfung, Shared-Line-Guard und Fortsetzungsentscheidung;
- Aufgabenmuster-Erkennung, Ausführungsebenen-Hierarchie und
  Multi-Decision-Matrix;
- Ticket-, Family-, Slug- und Scratch-Intake;
- reguläre Ticket-, Hotfix-, Release- und Support-Workflow-Karten;
- Conflict-, Evidence-, Security- und Publication-Grenzen;
- finalen Abschlussbericht.
```

Der Adapter darf nur diesen Satz ergänzen, bevor er einen Core-Endpunkt
ausführt:

```text
Dieser Core-Endpoint wird jetzt über den lokalen Go-Source-Entrypoint ausgeführt, damit die aktuelle Repository-Implementierung die Runtime-Autorität bleibt.
```

## [5] NICHT-DUPLIKATION UND PORTABILITÄT
[INTENT: CONSTRAINT]

Diese Adapterdatei ist absichtlich klein. Sie dupliziert nicht:

```text
- Branch-Familien, Branch-Regexe oder Ticket-Regexe;
- Commit-Typen, Commit-Header oder Längenlimits;
- CLI-Flags oder Endpoint-spezifische Argumentlisten;
- Release-, Hotfix- oder Propagation-Policy;
- Quality-Kommandos;
- Provider-Credentials, Tokenquellen oder Secrets;
- Inhalte aus AI-Base-Rules, docs/, Business-Dateien oder Quellcode.
```

Der Core ist als Binary-basierter Workflow in anderen Projekten wiederverwendbar.
Dieses Adaptermodul ist nur dort erforderlich, wo `git-governance` aus
dem Go-Source-Entrypoint statt als ausgelieferte Binary ausgeführt wird.

## [6] AUSGABEFORMAT
[INTENT: ANWEISUNG]

Solange der Adapter die Kontrolle führt (vor `CORE_WORKFLOW_EXECUTING`),
verwendet der Agent für Bootstrap-Statusmeldungen ausschließlich das Symbol
🔌 in der knappen öffentlichen Form des Core-Statusvertrags:

```text
🔌
<Ein Satz: welcher Initialisierungsschritt jetzt geschieht und warum.>
```

Der materielle Adapter-Gate-Nachweis lautet:

```text
🔌 Adapter | core_path=<resolved|blocked> | core_load=<fully_loaded|failed> | contract=<bound|failed> | entrypoint=<verified|failed> | delegation=<active|inactive>
```

Ab `CORE_WORKFLOW_EXECUTING` gilt ausschließlich die Symbol-Registry und das
Ausgabeformat des Core. Der Adapter verwendet die Core-Bereichs-Symbole niemals
für eigene Bootstrap-Schritte und ergänzt die Registry nicht.

Die Datensätze enthalten keine Secrets, private Gedankenketten oder
vollständige fremde Toolausgaben.

## [7] VERBOTE
[INTENT: CONSTRAINT]

Der Agent darf niemals:

```text
- diese Datei als passive Referenz behandeln, obwohl sie im Kontext präsent ist;
- vor CORE_WORKFLOW_EXECUTING eine fachliche Analyse, Datei-Suche über den
  Core-Pfad hinaus, Datei-Editierung, Staging, Commits, Branch-Operationen
  oder rohes Git beginnen;
- den Core nur auszugsweise, zusammengefasst oder aus dem Gedächtnis übernehmen;
- den Core durch Repository-Dokumentation, Quellcode oder externe
  Dokumentation substituieren;
- einen Endpoint umbenennen oder Argumente, Flags oder Werte ergänzen;
- Flags, Regexe oder technische Limits hart kodieren;
- Help-Reanchoring überspringen oder ein Help-Ergebnis für einen späteren
  Aufruf wiederverwenden;
- einen CLI-Workflow durch rohe Git-, GitHub- oder Shell-Orchestrierung ersetzen;
- ein Fallback-Binary bauen oder einen dist-Build als Ersatz ausführen;
- einen Zustandsübergang ohne seinen gebundenen Nachweis vollziehen;
- nach einer Invalidierung gecachte Core-Inhalte, Hilfetexte oder
  Entrypoint-Verifikationen wiederverwenden;
- während der Bootstrap-Phase Symbole der Core-Registry statt 🔌 verwenden;
- eine ausgelassene Initialisierung still fortsetzen statt sie als
  Prozessverletzung zu melden und nachzuholen;
- Credentials, Tokens, PEMs, Header oder private Chain-of-Thought ausgeben.
```

## [8] VERIFIKATION UND ABSCHLUSS
[INTENT: ANWEISUNG]

Vor dem Abschluss einer Adapteränderung oder einer durch den Adapter
geführten Aufgabe prüft der Agent den vollständigen Nachweis-Ledger:

```text
- adapter_presence_acknowledged: die Aktivierung wurde als erster Schritt bestätigt;
- core_path_resolved: der relative Core-Pfad ist lesbar;
- core_fully_read: der Core ist vollständig geladen;
- core_contract_bound: der Inhalt ist als CORE_WORKFLOW_CONTRACT gebunden;
- source_entrypoint_help_verified: der Source-Entrypoint ist über Help geprüft;
- delegation_active: jeder weitere Workflow-Schritt wurde vom Core gesteuert;
- non_duplication_verified: der Adapter enthält keine duplizierten CLI-Flags
  oder Policy-Werte und hat keine externe Dokumentations- oder
  Projektabhängigkeit eingeführt;
- adapter_completion_reported: der Abschlussbericht folgt dem Core-Format.
```

Fehlt ein Nachweis, ist der Abschluss `BLOCKED` statt `ADAPTER_COMPLETE`.
Fehlende Evidenz ist niemals `PASS`.

Die abschließende Antwort benennt zusätzlich zur Core-Zusammenfassung:

```text
- dass die Source-Binary-Bindung verwendet wurde;
- dass der relative Core geladen wurde;
- ob der Adapter oder Core wegen eines fehlenden Entrypoints blockiert war;
- ob eine Initialisierung nachträglich nachgeholt wurde, weil sie zuvor
  ausgelassen worden war.
```
