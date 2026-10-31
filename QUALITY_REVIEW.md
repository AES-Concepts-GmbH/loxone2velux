# Qualitätsprüfung – Befunde und Bearbeitungsplan

Prüfdatum: 31.10.2026  
Status: Alle nachfolgenden Punkte sind offen. Es wurden im Rahmen der Prüfung keine Fehlerbehebungen umgesetzt.

## Umfang und Validierung

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

- [ ] Beheben und prüfen
- Fundstelle: `internal/configmgr/manager.go:86` und Sidecar-Speicherung ab Zeile 96.
- Befund: `UpdateConfig` protokolliert Fehler beim Speichern der Hauptdatei oder der Loxone-Sidecar-Datei, übernimmt die Konfiguration trotzdem im Speicher und gibt `nil` zurück. API-Aufrufer erhalten dadurch eine Erfolgsmeldung für nicht dauerhaft gespeicherte Änderungen.
- Auswirkung: Einstellungen und Mappings können nach einem Neustart verschwinden. Auch Tokenänderungen können sich im laufenden Betrieb und nach Neustart unterschiedlich auswirken.
- Umsetzung: Persistenzfehler an den Aufrufer weitergeben. Festlegen, wie Änderungen an Hauptdatei und Sidecar konsistent gespeichert werden und wann die Laufzeitkonfiguration übernommen wird.
- Prüfkriterium: Ein nicht beschreibbarer Speicherort führt zu einer eindeutigen Fehlerantwort. Eine als erfolgreich bestätigte Änderung bleibt nach erneutem Laden erhalten.
- Testhinweis: `TestUpdateConfigSurvivesUnwritablePath` in `internal/configmgr/manager_test.go` erwartet derzeit ausdrücklich die Übernahme im Speicher trotz Schreibfehler; dieses Verhalten muss bewusst angepasst werden.

### Q-02 – Docker-Konfiguration verhindert persistente API-Änderungen

- [ ] Beheben und prüfen
- Fundstellen: `docker-compose.yml:11`, `Dockerfile:36` und `Dockerfile:53`.
- Befund: Compose bindet `/app/config.yaml` schreibgeschützt ein. Das Verzeichnis `/app` erhält keine Schreibrechte für den später verwendeten Benutzer `appuser`. Die daneben abgelegte `loxone.yaml` kann daher im vorgesehenen Standardbetrieb ebenfalls nicht angelegt werden.
- Auswirkung: Über die API vorgenommene Konfigurations- und Mappingänderungen können nicht dauerhaft gespeichert werden. Q-01 verdeckt dies zusätzlich.
- Umsetzung: Ein dediziertes beschreibbares Datenverzeichnis mit passenden Besitzrechten und persistentem Volume verwenden; Dateipfade, Startparameter und Anleitung darauf abstimmen.
- Prüfkriterium: Im tatsächlich gebauten Container als `appuser` ein Mapping und eine Konfigurationsänderung speichern, Container neu erstellen und den Erhalt beider Änderungen prüfen.

### Q-03 – Gleichzeitige Änderungen können sich überschreiben

- [ ] Beheben und prüfen
- Fundstellen: `internal/api/handlers.go:445`, `:589`, `:613`, `:645`, `:689`; `internal/configmgr/manager.go:57` und `:76`.
- Befund: Handler lesen eine vollständige Konfigurationskopie, verändern diese und speichern sie später zurück. Lesen und Schreiben sind jeweils geschützt, der gesamte Lese-Ändere-Schreib-Ablauf jedoch nicht.
- Auswirkung: Zwei gleichzeitige Mapping-Anlagen können erfolgreich antworten, obwohl die zuletzt gespeicherte Kopie die andere Änderung wieder entfernt. Auch Änderungen an unterschiedlichen Konfigurationsfeldern können verloren gehen.
- Umsetzung: Eine transaktionale Änderungsfunktion im Manager oder eine Versionsprüfung mit Konfliktantwort einführen.
- Prüfkriterium: Zwei gezielt parallel ausgeführte Änderungen bleiben beide erhalten oder eine wird ausdrücklich als Konflikt abgewiesen. Keine stillen Überschreibungen.

### Q-04 – Abgelaufene Requests können noch Gerätebefehle senden

- [ ] Beheben und prüfen
- Fundstellen: `internal/klf200/client.go:415` und `:643`; weitere Exchanges verwenden ebenfalls `exchangeMu`.
- Befund: Das Warten auf `exchangeMu` berücksichtigt den Request-Kontext nicht. Nach Erwerb des Locks wird der Befehl gesendet, bevor beim Warten auf die Antwort die Kontextüberschreitung ausgewertet wird. `sendRaw` verwendet einen eigenen Schreibtimeout statt des Request-Kontexts.
- Auswirkung: Ein bereits abgebrochener oder abgelaufener HTTP-Request kann später noch eine Gerätebewegung auslösen. Stop-Befehle warten zudem hinter anderen Exchanges.
- Umsetzung: Abbrechbare Warteschlange oder kontextabhängige Serialisierung verwenden. Kontext unmittelbar vor dem Senden prüfen und Schreibfristen berücksichtigen. Priorisierung von Stop-Befehlen vorsehen.
- Prüfkriterium: Einen Exchange blockieren, einen zweiten Gerätebefehl mit kurzer Frist einreihen und die Frist ablaufen lassen. Der zweite Befehl darf anschliessend nicht auf der Verbindung erscheinen. Stop-Latenz unter Last prüfen.

## Priorität mittel

### Q-05 – Unbekannte Messwerte erscheinen als eindeutige Zustände

- [ ] Beheben und prüfen
- Fundstellen: `internal/klf200/types.go:330`, `web/src/components/SensorCard.tsx:42` und `:54`.
- Befund: `PositionToPercent` wandelt auch `PositionUnknown` in 100 Prozent um. Die Sensoranzeige verwendet bei fehlenden Sensordaten dieselbe Darstellung wie bei ausdrücklich negativen Regen-/Windwerten.
- Auswirkung: Eine unbekannte Position kann als „geschlossen“ erscheinen; noch nicht abgefragte Sensoren werden als „Kein Regen“ beziehungsweise „Kein Wind“ dargestellt.
- Umsetzung: Gültigkeit und Aktualität ausdrücklich modellieren. Unbekannte oder veraltete Werte in API und UI von gültigen Messwerten unterscheiden. Loxone-Endpunkte und UDP-Format bei der Anpassung berücksichtigen.
- Prüfkriterium: Unbekannte Rohpositionen, fehlende Sensorwerte und veraltete Rückmeldungen zeigen keinen bestätigten geschlossenen beziehungsweise wetterfreien Zustand.
- Testhinweis: `internal/klf200/commands_test.go` erwartet derzeit für `PositionUnknown` den Wert 100; diese Erwartung entsprechend überarbeiten.

### Q-06 – Unvollständige Sensorantworten können als Erfolg gelten

- [ ] Beheben und prüfen
- Fundstelle: `internal/klf200/client.go:553` und Aktualisierung des Zeitstempels in `RefreshSensorStatus`.
- Befund: Nach mindestens einer empfangenen Limitation-Meldung wird ein späterer Timeout als Erfolg behandelt. Beim Zählen werden weder die Session-ID noch eindeutige angefragte Node-IDs geprüft. Danach kann der Gesamtstatus einen neuen Aktualisierungszeitpunkt erhalten.
- Auswirkung: Teilweise alte Sensorwerte können als frisch abgefragt erscheinen; doppelte oder fremde Meldungen können die erwartete Anzahl vorzeitig erfüllen.
- Umsetzung: Antworten der Session und den angefragten Nodes zuordnen. Vollständigkeit je Node erfassen und Teilantworten ausdrücklich behandeln.
- Prüfkriterium: Tests mit fehlender Antwort, doppelter Meldung und falscher Session-ID dürfen keinen vollständigen erfolgreichen Refresh melden.

### Q-07 – Konfigurationsdateien werden nicht atomar geschrieben

- [ ] Beheben und prüfen
- Fundstelle: `internal/config/config.go:226`.
- Befund: `os.WriteFile` überschreibt die Zieldatei direkt. Hauptkonfiguration und Sidecar werden unabhängig voneinander geschrieben.
- Auswirkung: Ein Prozessabbruch oder Schreibfehler kann eine leere beziehungsweise unvollständige Datei hinterlassen; beide Dateien können unterschiedliche Konfigurationsstände enthalten.
- Umsetzung: Temporäre Datei im Zielverzeichnis schreiben und atomar ersetzen; Dateirechte erhalten. Konsistenz und Wiederherstellung für das Dateipaar gemeinsam mit Q-01 planen.
- Prüfkriterium: Bei simuliertem Schreibfehler bleibt die bisherige Datei gültig und lesbar. Ein unterbrochener Speichervorgang hinterlässt einen definierten wiederherstellbaren Zustand.

### Q-08 – Dashboard vermischt Ziel- und Istposition

- [ ] Beheben und prüfen
- Fundstelle: `web/src/components/Dashboard.tsx:119`.
- Befund: Nach Annahme eines Befehls setzt `applyPosition` die angezeigte Istposition sofort auf den Zielwert. Bei fehlgeschlagenem Polling bleibt ausserdem der letzte erfolgreiche Verbindungsstatus erhalten.
- Auswirkung: Noch fahrende oder blockierte Geräte können bereits als vollständig offen/geschlossen erscheinen. Ein alter Status kann weiterhin „Verbunden“ anzeigen.
- Umsetzung: Zielposition, Befehlsannahme und rückgemeldete Istposition getrennt darstellen. Aktualität des Verbindungsstatus erfassen und veraltete Werte kennzeichnen.
- Prüfkriterium: Nach Befehlsannahme bleibt die Istposition bis zur Rückmeldung unverändert. Bei ausbleibenden erfolgreichen Abfragen wird der Verbindungsstatus als unbekannt oder veraltet angezeigt.

## Weitere Verbesserungsmöglichkeiten

### Q-09 – Zuständigkeit für Home-Assistant-Einstellungen klären

- [ ] Bearbeiten
- Fundstelle: `loxone2velux/run.sh:55`.
- Befund: Die Hauptkonfiguration wird bei jedem Add-on-Start aus den HA-Optionen neu erzeugt. Änderungen an KLF-200-Zugangsdaten, API-Token oder Log-Level über die REST-API werden dadurch beim Neustart ersetzt. Nur die Loxone-Einstellungen besitzen eine separate Sidecar-Persistenz.
- Umsetzung: Eine eindeutige Quelle für diese Einstellungen festlegen. Falls HA-Optionen massgeblich bleiben, dies in API-Verhalten und Dokumentation kenntlich machen.
- Prüfkriterium: Die bestätigte Gültigkeitsdauer einer API-Änderung entspricht dem Verhalten nach Add-on-Neustart.

### Q-10 – UDP-Zustandsabgleich ergänzen

- [ ] Bearbeiten
- Fundstellen: `internal/gateway/service.go:189`, `:306` und `:327`.
- Befund: Vollständige Node-Refreshes ersetzen den Cache, senden aber keinen Positionsabgleich an Loxone. Sensorfeedback wird bei Änderungen gesendet.
- Auswirkung: Nach einem Miniserver-Neustart oder verlorenen UDP-Paketen können unveränderte Zustände auf Empfängerseite fehlen, bis neue Ereignisse auftreten.
- Umsetzung: Initialen beziehungsweise periodischen Snapshot oder einen explizit anforderbaren Zustandsabgleich ergänzen. Paketmenge begrenzen.
- Prüfkriterium: Ein neu gestarteter Empfänger erhält auch ohne Gerätebewegung den aktuellen Zustand aller aktiv gemappten Nodes.

### Q-11 – Positionsregler per Tastatur vollständig bedienbar machen

- [ ] Bearbeiten
- Fundstelle: `web/src/components/NodeCard.tsx:119`.
- Befund: Ein Positionsbefehl wird über `onMouseUp` beziehungsweise `onTouchEnd` ausgelöst. Tastaturänderungen des Range-Inputs besitzen keinen entsprechenden Commit-Pfad.
- Umsetzung: Einheitliches, tastaturfähiges Commit-Verhalten mit sinnvoller Begrenzung der Befehlsrate implementieren; zugängliche Beschriftung des Reglers ergänzen.
- Prüfkriterium: Der Regler lässt sich ohne Maus auf einen Wert setzen und löst zuverlässig den vorgesehenen Befehl aus.

### Q-12 – Loxone-Anleitung an tatsächliche Verbindung anpassen

- [ ] Bearbeiten
- Fundstellen: `web/src/components/LoxoneGuide.tsx:74` und `:80`.
- Befund: Die Anleitung verwendet fest Port 8099, während der standalone Docker-Betrieb standardmässig Port 8080 nutzt. Tokens werden ohne URL-Encoding in Query-Beispiele eingesetzt. Der Browser-Hostname muss bei Proxy-Zugriff zudem nicht der direkt erreichbaren Gateway-Adresse entsprechen.
- Umsetzung: Direkte Gateway-Adresse konfigurierbar machen oder passend vorbelegen. Query-Tokens mit `encodeURIComponent` kodieren.
- Prüfkriterium: Generierte Beispiele funktionieren für HA, Docker und Tokens mit reservierten URL-Zeichen.

### Q-13 – SLIP-Empfangspuffer begrenzen

- [ ] Bearbeiten
- Fundstelle: `internal/klf200/client.go:846`.
- Befund: Zwischen SLIP-Endmarkierungen wächst `frameBuf` ohne Obergrenze.
- Auswirkung: Ein fehlerhafter oder manipulierter Datenstrom kann den Speicherverbrauch unbegrenzt erhöhen.
- Umsetzung: Eine aus dem Protokoll abgeleitete maximale kodierte Framegrösse erzwingen und bei Überschreitung verwerfen beziehungsweise die Verbindung definiert zurücksetzen.
- Prüfkriterium: Ein überlanger Datenstrom ohne Abschlussmarkierung führt nicht zu unbeschränktem Speicherwachstum; nachfolgende gültige Frames werden entsprechend der gewählten Strategie wieder verarbeitet.

### Q-14 – Tests und CI um relevante Fehlerpfade erweitern

- [ ] Bearbeiten
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
