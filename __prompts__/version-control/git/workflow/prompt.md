# Git-Governance Source-Repository Adapter

## [0] ADAPTER-AUTORITÄT
[INTENT: ANWEISUNG]

Diese Datei ist die projektspezifische Adapterhülle für das
`git-governance`-Quellrepository. Sie ist kein zweiter Workflow-Kern und
enthält keine duplizierte Branch-, Commit-, Release-, Hotfix- oder
CLI-Policy.

Der portable, vollständige Workflow-Kern liegt relativ zu dieser Datei unter:

```text
core/prompt.md
```

Vor jeder Analyse, Entscheidung, Tool-Verwendung oder Mutation MUSS der Agent:

1. den relativen Pfad von dieser Adapterdatei auflösen;
2. `core/prompt.md` vollständig lesen;
3. den vollständigen Inhalt als `CORE_WORKFLOW_CONTRACT` übernehmen;
4. diesen Adapter nur als Laufzeit- und Binary-Bindung anwenden.

Wenn der Core nicht existiert, nicht vollständig lesbar ist oder sein Inhalt
nicht als Workflowvertrag übernommen werden kann, ist der Zustand `BLOCKED`.
Der Agent darf keinen verkürzten Ersatz, keine externe Dokumentation und keine
ältere Prompt-Kopie verwenden.

## [1] PROJEKTSPEZIFISCHE BINARY-BINDUNG
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

## [2] ADAPTER-INITIALISIERUNG
[INTENT: ANWEISUNG]

Der Agent führt die folgende Zustandssequenz aus:

```text
ADAPTER_DISCOVERED
-> CORE_LOADED
-> SOURCE_ENTRYPOINT_BOUND
-> CORE_WORKFLOW_EXECUTING
```

### 2.1 Relative Core-Auflösung

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

### 2.2 Source-Entrypoint prüfen

Nach vollständigem Core-Laden prüft der Agent über die im Core definierte
Help-first-Sequenz, ob dieser Quellrepository-Entrypoint verfügbar ist:

```text
go run -mod=readonly ./cmd/git-governance --help
```

Der Entrypoint ist nur dann gebunden, wenn die Hilfe erfolgreich die erwartete
CLI-Oberfläche zeigt. Fehlt Go, kann das Modul nicht ausgeführt werden oder
liefert der Entrypoint keine gültige CLI-Hilfe, bleibt der Workflow
`BLOCKED`; der Agent baut weder ein Fallback-Binary noch verwendet er einen
anderen Git- oder GitHub-Client als Ersatz.

## [3] CORE-DELEGATION
[INTENT: ANWEISUNG]

Nach `SOURCE_ENTRYPOINT_BOUND` gelten alle weiteren Entscheidungen,
Zustandsübergänge, Entscheidungsmatrizen, Scratch-Entscheidungen,
Endpoint-Auswahl, Quality-Gates, Commit-Regeln, Release- und Hotfix-Grenzen
ausschließlich aus `CORE_WORKFLOW_CONTRACT`.

Insbesondere übernimmt der Agent aus dem Core unverändert:

```text
- Help-first-Laufzeitvertrag;
- Audit- und sichtbare Statuskommunikation;
- Branch- und Fortsetzungsentscheidung;
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

## [4] NICHT-DUPLIKATION UND PORTABILITÄT
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

## [5] VERIFIKATION UND ABSCHLUSS
[INTENT: ANWEISUNG]

Vor dem Abschluss einer Adapteränderung prüft der Agent:

```text
- der relative Core-Pfad ist lesbar;
- der Core ist vollständig geladen;
- der Source-Entrypoint ist über Help geprüft;
- der Adapter enthält keine duplizierten CLI-Flags oder Policy-Werte;
- jeder weitere Workflow-Schritt wurde vom Core gesteuert;
- keine externe Dokumentations- oder Projektabhängigkeit wurde eingeführt.
```

Die abschließende Antwort benennt zusätzlich zur Core-Zusammenfassung:

```text
- dass die Source-Binary-Bindung verwendet wurde;
- dass der relative Core geladen wurde;
- ob der Adapter oder Core wegen eines fehlenden Entrypoints blockiert war.
```
