# Qualitätsprüfung – Befunde und Bearbeitungsplan

Prüfdatum: 31.10.2026  
Umsetzungsstand: Q-01 bis Q-14 im Arbeitsstand umgesetzt. Die Befunde unten dokumentieren den ursprünglichen Prüfstand; Zeilenangaben und Hinweise auf frühere Testerwartungen sind historisch. Hardware- und Supervisor-Abnahme stehen weiterhin aus.

## Umsetzung und erneute Validierung (31.10.2026)

| Befund | Änderung | Nachweis |
| --- | --- | --- |
| Q-01 / Q-07 | Atomare Dateiersetzung (temporäre Datei, Sync, Rename, 0600); Fehler vor Laufzeitübernahme; pro Betriebsart genau eine autoritative Datei | Manager-/API-Fehlertests; vorhandene Datei bleibt bei echtem Schreibfehler erhalten (Linux, Benutzer 1000) |
| Q-02 | Beschreibbares Docker-Datenverzeichnis unter `/data`; Konfiguration unter `/data/config.yaml` | Gebautes Image als UID 1000: Mapping und Log-Level speichern, Container neu erstellen, beide Werte und Dateirechte prüfen |
| Q-03 | `ModifyConfig` serialisiert Lesen, Ändern, Prüfen, Speichern und Anwenden; alle schreibenden Handler verwenden diese Operation | Gleichzeitige HTTP-Anlagen und Manager-Änderungen; nach Laden alle 12 Mappings vorhanden |
| Q-04 | Abbrechbare Exchange-Warteschlange, Kontextprüfung vor dem Senden, kontextgebundene Schreibfrist, Vorrang für wartende Stop-Befehle | Abgelaufener Befehl erscheint nicht auf der simulierten Verbindung; Queue-Test prüft Stop vor wartenden Bewegungsbefehlen |
| Q-05 | Unbekannte Position = -1; Alter und Verbindung bestimmen `stale`/`valid`; unbekannte Loxone-Werte liefern 503/ERROR; UDP überspringt unbekannte Positionen | Backend-/Frontend-Tests für unbekannte Positionen, fehlende Sensoren und Verbindungsverlust |
| Q-06 | Session- und Node-Zuordnung, eindeutige Antworten und vollständige Sammlung über alle Chunks; kein neuer Gesamtzeitstempel bei Teilantwort | Simulatorfälle: fehlende Antwort, Duplikat, falsche Session; vorherige Werte bleiben erhalten und werden ungültig markiert |
| Q-08 | Angenommene Ziele getrennt vom Istwert; Polling-Fehler entwerten Anzeigen; HTTP-Abfragen sind zeitlich begrenzt | Dashboard-Test für Annahme ohne Positionsrückmeldung und Verbindungsverlust nach erfolgreicher Abfrage |
| Q-09 | Add-on startet mit `-managed-config`; HA verwaltet Zugangsdaten, Token und Log-Level; API lehnt deren Änderung ab; nur Loxone-Sidecar wird gespeichert | Tests für verwaltete Einstellungen, unveränderte HA-Datei und Sidecar-Schreibfehler; README/DOCS angepasst |
| Q-10 | Initiale und periodische Refreshes senden Zustands-Snapshots, auch ohne Wertänderung; Mapping-Liste auf 1000 begrenzt | UDP-Test für wiederholte identische Positionen und Sensorwerte |
| Q-11 | Beschrifteter Zielregler mit Keyboard-/Blur-Commit und Unterdrückung doppelter Commits; Stop auch bei unsicherem Bewegungsstatus erreichbar | Tastatur-, Maus- und Doppel-Commit-Tests |
| Q-12 | Editierbare direkte Gateway-Adresse; Query-Token URL-kodiert; fehlender Browser-Token bedeutet nicht automatisch deaktivierte Authentifizierung | Anleitungstest mit reservierten Token-Zeichen und eigener Gateway-Adresse |
| Q-13 | SLIP-Puffer auf 514 kodierte Bytes begrenzt (257 dekodierte Bytes); Verwerfen bis zur nächsten Begrenzung | Übergrosser Datenstrom, danach erfolgreiche gültige Antwort |
| Q-14 | Regressionstests und CI für Container-Persistenz sowie npm-/Go-Sicherheitsaudits ergänzt | Linux-Race-Check, Coverage, Frontend-Tests/Build, lokaler Container-Smoke-Test |

Ausserdem: UDP-Testmeldungen geben Socketfehler zurück; Konfigurationswechsel bereiten den UDP-Socket vor dem Speichern vor. Hintergrundoperationen übernehmen den Service-Kontext und werden beim Stop abgebrochen. HTTP-Startfehler führen nach geordnetem Shutdown zu einem Fehler-Exit.

Bestehende Standalone-Sidecars werden einmalig in die vollständige Konfiguration übernommen und danach als `loxone.yaml.legacy` gesichert. Eine fehlgeschlagene Migration beendet den Start, statt alte Mappings stillschweigend zu verwerfen; Regressionstests prüfen Übernahme, spätere Änderungen und fehlgeschlagenes Speichern.

Validierung des neuen Stands:

- Go 1.26.10, Linux, UID 1000: `go test -race -coverprofile=... -coverpkg=./... ./...` bestanden; Gesamt-Coverage 85,9 %. `go vet ./...` ohne Befund.
- 34 Frontend-Tests in sechs Dateien bestanden; Produktionsbuild und zusätzlicher Coverage-Lauf erfolgreich.
- Vite 8.3.5, Vitest/Coverage 5.0.5, React-Plugin 6.1.2 und Tailwind 3.4.19; aktualisierter Lockfile. `npm audit` meldet 0 bekannte Schwachstellen.
- Gepatchtes Go-Build-Image 1.26.10; `govulncheck@v1.9.0` ohne bekannte Schwachstellen. Der vorherige Go-1.26-Build wurde wegen [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809) aktualisiert.
- Docker-Image lokal gebaut; Persistenztest nach Container-Neuerstellung erfolgreich. Shell-Syntax, Compose-Konfiguration und Modul-Konsistenz ebenfalls ohne Befund.
- Frontend-Build läuft auf der nativen Builder-Architektur, damit ARMv7-Add-ons keine nicht verfügbare Rolldown-musl-Bindung benötigen.
- Kein Test gegen reale KLF-200-Geräte oder einen laufenden HA-Supervisor. Stop priorisiert wartende Exchanges; ein bereits laufender Exchange wird nicht mitten im Protokoll unterbrochen. Das Add-on muss mit einer neuen Veröffentlichung gebaut werden, damit sein Source-Tag diese Änderungen enthält.

## Ursprünglicher Umfang und damalige Validierung

Geprüft wurden Go-Backend, React-Frontend, vorhandene Tests, Docker-Konfiguration, Home-Assistant-Add-on und Dokumentation. Die Befunde stammen aus einer statischen Codeprüfung; die beschriebenen Fehlerszenarien wurden nicht jeweils durch zusätzliche Reproduktionstests nachgewiesen.

Ergebnisse der ausgeführten Prüfungen:

- Alle Go-Tests (`go test ./...`) bestanden nach Freigabe lokaler TCP-/UDP-Verbindungen. Der erste Lauf war durch die Sandbox blockiert; diese Fehler sind keine Projektfehler.
- `go vet ./...` ohne Befund.
- Alle 28 Frontend-Tests in vier Testdateien bestanden.
- Frontend-Produktionsbuild (`npm run build`) erfolgreich.
- Race-Check (`go test -race ./...`) nicht ausführbar, da CGo in der lokalen Umgebung deaktiviert war.
- Kein Test gegen reale KLF-200-Hardware, einen laufenden Home-Assistant-Supervisor oder gebaute Docker-Container.
- Kein vollständiger Audit der Abhängigkeiten auf bekannte Sicherheitslücken.

Bestehende Tests sichern wesentliche Standardabläufe ab, decken die folgenden Randfälle jedoch nicht ausreichend ab. Die Zeilenangaben beziehen sich auf den Stand der Prüfung und können sich bei Änderungen verschieben.

## Priorität hoch

### Q-01 – Speicherfehler werden als Erfolg gemeldet

- [x] Implementiert und durch Regressionstests geprüft (siehe Umsetzungsstand)
- Fundstelle: `internal/configmgr/manager.go:86` und Sidecar-Speicherung ab Zeile 96.
- Befund: `UpdateConfig` protokolliert Fehler beim Speichern der Hauptdatei oder der Loxone-Sidecar-Datei, übernimmt die Konfiguration trotzdem im Speicher und gibt `nil` zurück. API-Aufrufer erhalten dadurch eine Erfolgsmeldung für nicht dauerhaft gespeicherte Änderungen.
- Auswirkung: Einstellungen und Mappings können nach einem Neustart verschwinden. Auch Tokenänderungen können sich im laufenden Betrieb und nach Neustart unterschiedlich auswirken.
- Umsetzung: Persistenzfehler an den Aufrufer weitergeben. Festlegen, wie Änderungen an Hauptdatei und Sidecar konsistent gespeichert werden und wann die Laufzeitkonfiguration übernommen wird.
- Prüfkriterium: Ein nicht beschreibbarer Speicherort führt zu einer eindeutigen Fehlerantwort. Eine als erfolgreich bestätigte Änderung bleibt nach erneutem Laden erhalten.
- Testhinweis: `TestUpdateConfigSurvivesUnwritablePath` in `internal/configmgr/manager_test.go` erwartet derzeit ausdrücklich die Übernahme im Speicher trotz Schreibfehler; dieses Verhalten muss bewusst angepasst werden.

### Q-02 – Docker-Konfiguration verhindert persistente API-Änderungen

- [x] Implementiert und durch Regressionstests geprüft (siehe Umsetzungsstand)
- Fundstellen: `docker-compose.yml:11`, `Dockerfile:36` und `Dockerfile:53`.
- Befund: Compose bindet `/app/config.yaml` schreibgeschützt ein. Das Verzeichnis `/app` erhält keine Schreibrechte für den später verwendeten Benutzer `appuser`. Die daneben abgelegte `loxone.yaml` kann daher im vorgesehenen Standardbetrieb ebenfalls nicht angelegt werden.
- Auswirkung: Über die API vorgenommene Konfigurations- und Mappingänderungen können nicht dauerhaft gespeichert werden. Q-01 verdeckt dies zusätzlich.
- Umsetzung: Ein dediziertes beschreibbares Datenverzeichnis mit passenden Besitzrechten und persistentem Volume verwenden; Dateipfade, Startparameter und Anleitung darauf abstimmen.
- Prüfkriterium: Im tatsächlich gebauten Container als `appuser` ein Mapping und eine Konfigurationsänderung speichern, Container neu erstellen und den Erhalt beider Änderungen prüfen.

### Q-03 – Gleichzeitige Änderungen können sich überschreiben

- [x] Implementiert und durch Regressionstests geprüft (siehe Umsetzungsstand)
- Fundstellen: `internal/api/handlers.go:445`, `:589`, `:613`, `:645`, `:689`; `internal/configmgr/manager.go:57` und `:76`.
- Befund: Handler lesen eine vollständige Konfigurationskopie, verändern diese und speichern sie später zurück. Lesen und Schreiben sind jeweils geschützt, der gesamte Lese-Ändere-Schreib-Ablauf jedoch nicht.
- Auswirkung: Zwei gleichzeitige Mapping-Anlagen können erfolgreich antworten, obwohl die zuletzt gespeicherte Kopie die andere Änderung wieder entfernt. Auch Änderungen an unterschiedlichen Konfigurationsfeldern können verloren gehen.
- Umsetzung: Eine transaktionale Änderungsfunktion im Manager oder eine Versionsprüfung mit Konfliktantwort einführen.
- Prüfkriterium: Zwei gezielt parallel ausgeführte Änderungen bleiben beide erhalten oder eine wird ausdrücklich als Konflikt abgewiesen. Keine stillen Überschreibungen.

### Q-04 – Abgelaufene Requests können noch Gerätebefehle senden

- [x] Implementiert und durch Regressionstests geprüft (siehe Umsetzungsstand)
- Fundstellen: `internal/klf200/client.go:415` und `:643`; weitere Exchanges verwenden ebenfalls `exchangeMu`.
- Befund: Das Warten auf `exchangeMu` berücksichtigt den Request-Kontext nicht. Nach Erwerb des Locks wird der Befehl gesendet, bevor beim Warten auf die Antwort die Kontextüberschreitung ausgewertet wird. `sendRaw` verwendet einen eigenen Schreibtimeout statt des Request-Kontexts.
- Auswirkung: Ein bereits abgebrochener oder abgelaufener HTTP-Request kann später noch eine Gerätebewegung auslösen. Stop-Befehle warten zudem hinter anderen Exchanges.
- Umsetzung: Abbrechbare Warteschlange oder kontextabhängige Serialisierung verwenden. Kontext unmittelbar vor dem Senden prüfen und Schreibfristen berücksichtigen. Priorisierung von Stop-Befehlen vorsehen.
- Prüfkriterium: Einen Exchange blockieren, einen zweiten Gerätebefehl mit kurzer Frist einreihen und die Frist ablaufen lassen. Der zweite Befehl darf anschliessend nicht auf der Verbindung erscheinen. Stop-Latenz unter Last prüfen.

## Priorität mittel

### Q-05 – Unbekannte Messwerte erscheinen als eindeutige Zustände

- [x] Implementiert und durch Regressionstests geprüft (siehe Umsetzungsstand)
- Fundstellen: `internal/klf200/types.go:330`, `web/src/components/SensorCard.tsx:42` und `:54`.
- Befund: `PositionToPercent` wandelt auch `PositionUnknown` in 100 Prozent um. Die Sensoranzeige verwendet bei fehlenden Sensordaten dieselbe Darstellung wie bei ausdrücklich negativen Regen-/Windwerten.
- Auswirkung: Eine unbekannte Position kann als „geschlossen“ erscheinen; noch nicht abgefragte Sensoren werden als „Kein Regen“ beziehungsweise „Kein Wind“ dargestellt.
- Umsetzung: Gültigkeit und Aktualität ausdrücklich modellieren. Unbekannte oder veraltete Werte in API und UI von gültigen Messwerten unterscheiden. Loxone-Endpunkte und UDP-Format bei der Anpassung berücksichtigen.
- Prüfkriterium: Unbekannte Rohpositionen, fehlende Sensorwerte und veraltete Rückmeldungen zeigen keinen bestätigten geschlossenen beziehungsweise wetterfreien Zustand.
- Testhinweis: `internal/klf200/commands_test.go` erwartet derzeit für `PositionUnknown` den Wert 100; diese Erwartung entsprechend überarbeiten.

### Q-06 – Unvollständige Sensorantworten können als Erfolg gelten

- [x] Implementiert und durch Regressionstests geprüft (siehe Umsetzungsstand)
- Fundstelle: `internal/klf200/client.go:553` und Aktualisierung des Zeitstempels in `RefreshSensorStatus`.
- Befund: Nach mindestens einer empfangenen Limitation-Meldung wird ein späterer Timeout als Erfolg behandelt. Beim Zählen werden weder die Session-ID noch eindeutige angefragte Node-IDs geprüft. Danach kann der Gesamtstatus einen neuen Aktualisierungszeitpunkt erhalten.
- Auswirkung: Teilweise alte Sensorwerte können als frisch abgefragt erscheinen; doppelte oder fremde Meldungen können die erwartete Anzahl vorzeitig erfüllen.
- Umsetzung: Antworten der Session und den angefragten Nodes zuordnen. Vollständigkeit je Node erfassen und Teilantworten ausdrücklich behandeln.
- Prüfkriterium: Tests mit fehlender Antwort, doppelter Meldung und falscher Session-ID dürfen keinen vollständigen erfolgreichen Refresh melden.

### Q-07 – Konfigurationsdateien werden nicht atomar geschrieben

- [x] Implementiert und durch Regressionstests geprüft (siehe Umsetzungsstand)
- Fundstelle: `internal/config/config.go:226`.
- Befund: `os.WriteFile` überschreibt die Zieldatei direkt. Hauptkonfiguration und Sidecar werden unabhängig voneinander geschrieben.
- Auswirkung: Ein Prozessabbruch oder Schreibfehler kann eine leere beziehungsweise unvollständige Datei hinterlassen; beide Dateien können unterschiedliche Konfigurationsstände enthalten.
- Umsetzung: Temporäre Datei im Zielverzeichnis schreiben und atomar ersetzen; Dateirechte erhalten. Konsistenz und Wiederherstellung für das Dateipaar gemeinsam mit Q-01 planen.
- Prüfkriterium: Bei simuliertem Schreibfehler bleibt die bisherige Datei gültig und lesbar. Ein unterbrochener Speichervorgang hinterlässt einen definierten wiederherstellbaren Zustand.

### Q-08 – Dashboard vermischt Ziel- und Istposition

- [x] Implementiert und durch Regressionstests geprüft (siehe Umsetzungsstand)
- Fundstelle: `web/src/components/Dashboard.tsx:119`.
- Befund: Nach Annahme eines Befehls setzt `applyPosition` die angezeigte Istposition sofort auf den Zielwert. Bei fehlgeschlagenem Polling bleibt ausserdem der letzte erfolgreiche Verbindungsstatus erhalten.
- Auswirkung: Noch fahrende oder blockierte Geräte können bereits als vollständig offen/geschlossen erscheinen. Ein alter Status kann weiterhin „Verbunden“ anzeigen.
- Umsetzung: Zielposition, Befehlsannahme und rückgemeldete Istposition getrennt darstellen. Aktualität des Verbindungsstatus erfassen und veraltete Werte kennzeichnen.
- Prüfkriterium: Nach Befehlsannahme bleibt die Istposition bis zur Rückmeldung unverändert. Bei ausbleibenden erfolgreichen Abfragen wird der Verbindungsstatus als unbekannt oder veraltet angezeigt.

## Weitere Verbesserungsmöglichkeiten

### Q-09 – Zuständigkeit für Home-Assistant-Einstellungen klären

- [x] Implementiert (siehe Umsetzungsstand)
- Fundstelle: `loxone2velux/run.sh:55`.
- Befund: Die Hauptkonfiguration wird bei jedem Add-on-Start aus den HA-Optionen neu erzeugt. Änderungen an KLF-200-Zugangsdaten, API-Token oder Log-Level über die REST-API werden dadurch beim Neustart ersetzt. Nur die Loxone-Einstellungen besitzen eine separate Sidecar-Persistenz.
- Umsetzung: Eine eindeutige Quelle für diese Einstellungen festlegen. Falls HA-Optionen massgeblich bleiben, dies in API-Verhalten und Dokumentation kenntlich machen.
- Prüfkriterium: Die bestätigte Gültigkeitsdauer einer API-Änderung entspricht dem Verhalten nach Add-on-Neustart.

### Q-10 – UDP-Zustandsabgleich ergänzen

- [x] Implementiert (siehe Umsetzungsstand)
- Fundstellen: `internal/gateway/service.go:189`, `:306` und `:327`.
- Befund: Vollständige Node-Refreshes ersetzen den Cache, senden aber keinen Positionsabgleich an Loxone. Sensorfeedback wird bei Änderungen gesendet.
- Auswirkung: Nach einem Miniserver-Neustart oder verlorenen UDP-Paketen können unveränderte Zustände auf Empfängerseite fehlen, bis neue Ereignisse auftreten.
- Umsetzung: Initialen beziehungsweise periodischen Snapshot oder einen explizit anforderbaren Zustandsabgleich ergänzen. Paketmenge begrenzen.
- Prüfkriterium: Ein neu gestarteter Empfänger erhält auch ohne Gerätebewegung den aktuellen Zustand aller aktiv gemappten Nodes.

### Q-11 – Positionsregler per Tastatur vollständig bedienbar machen

- [x] Implementiert (siehe Umsetzungsstand)
- Fundstelle: `web/src/components/NodeCard.tsx:119`.
- Befund: Ein Positionsbefehl wird über `onMouseUp` beziehungsweise `onTouchEnd` ausgelöst. Tastaturänderungen des Range-Inputs besitzen keinen entsprechenden Commit-Pfad.
- Umsetzung: Einheitliches, tastaturfähiges Commit-Verhalten mit sinnvoller Begrenzung der Befehlsrate implementieren; zugängliche Beschriftung des Reglers ergänzen.
- Prüfkriterium: Der Regler lässt sich ohne Maus auf einen Wert setzen und löst zuverlässig den vorgesehenen Befehl aus.

### Q-12 – Loxone-Anleitung an tatsächliche Verbindung anpassen

- [x] Implementiert (siehe Umsetzungsstand)
- Fundstellen: `web/src/components/LoxoneGuide.tsx:74` und `:80`.
- Befund: Die Anleitung verwendet fest Port 8099, während der standalone Docker-Betrieb standardmässig Port 8080 nutzt. Tokens werden ohne URL-Encoding in Query-Beispiele eingesetzt. Der Browser-Hostname muss bei Proxy-Zugriff zudem nicht der direkt erreichbaren Gateway-Adresse entsprechen.
- Umsetzung: Direkte Gateway-Adresse konfigurierbar machen oder passend vorbelegen. Query-Tokens mit `encodeURIComponent` kodieren.
- Prüfkriterium: Generierte Beispiele funktionieren für HA, Docker und Tokens mit reservierten URL-Zeichen.

### Q-13 – SLIP-Empfangspuffer begrenzen

- [x] Implementiert (siehe Umsetzungsstand)
- Fundstelle: `internal/klf200/client.go:846`.
- Befund: Zwischen SLIP-Endmarkierungen wächst `frameBuf` ohne Obergrenze.
- Auswirkung: Ein fehlerhafter oder manipulierter Datenstrom kann den Speicherverbrauch unbegrenzt erhöhen.
- Umsetzung: Eine aus dem Protokoll abgeleitete maximale kodierte Framegrösse erzwingen und bei Überschreitung verwerfen beziehungsweise die Verbindung definiert zurücksetzen.
- Prüfkriterium: Ein überlanger Datenstrom ohne Abschlussmarkierung führt nicht zu unbeschränktem Speicherwachstum; nachfolgende gültige Frames werden entsprechend der gewählten Strategie wieder verarbeitet.

### Q-14 – Tests und CI um relevante Fehlerpfade erweitern

- [x] Implementiert (siehe Umsetzungsstand)
- Bestehende Tests zu Persistenzfehlern und unbekannten Positionen bei geänderter Semantik aktualisieren.
- Reproduktionstests für Q-03, Q-04 und Q-06 ergänzen.
- Fehlende beziehungsweise veraltete Sensorwerte und Ziel-/Istposition im Frontend testen.
- Container-Smoke-Test für Schreibrechte, Persistenz und Neustart ergänzen.
- Race-Check in einer geeigneten CGo-Umgebung ausführen; die vorhandene Linux-CI enthält bereits `go test -race`.
- Abhängigkeiten separat auf bekannte Sicherheitslücken prüfen; aus der vorliegenden Prüfung lässt sich kein vollständiges Urteil dazu ableiten.

## Empfohlene Bearbeitungsreihenfolge

1. Q-01, Q-02 und Q-07 gemeinsam lösen: verlässliche Persistenz und funktionierende Docker-Datenhaltung.
2. Q-03 und Q-04: konkurrierende Änderungen und verspätete Gerätebefehle verhindern.
3. Q-05, Q-06 und Q-08: gültige, vollständige und aktuelle Zustände anzeigen.
4. Q-09 bis Q-13: Betriebsverhalten, Integration und Bedienbarkeit verbessern.
5. Q-14 bei jeder Änderung ergänzen und abschliessend alle Checks erneut ausführen.
