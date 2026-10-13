# Changelog

## 1.3.0

Korrekturen aus dem Code-Review:

- Fix: KLF-200-Protokoll – `GW_COMMAND_RUN_STATUS_NTF` wurde an falschen Byte-Offsets gelesen (Run-Status/Status-Reply)
- Fix: KLF-200-Protokoll – Command-IDs für Limitation-Status (`0x0312`–`0x0314`), Originator-Werte (Regen = 2, Wind = 9) und Sonderwert für Stop (`0xD200`) korrigiert; Regen-/Windsensor funktioniert damit erstmals korrekt
- Fix: `GW_COMMAND_SEND_CFM` Status 0 bedeutet "abgelehnt" und wird jetzt als Fehler gemeldet
- Fix: Node-Typ-Tabelle an die Spezifikation angepasst (Fensteröffner, Rollläden, Markisen usw. werden korrekt benannt), unbekannte Subtypen fallen auf den Basistyp zurück
- Sicherheit: TLS-Zertifikat des KLF-200 wird jetzt tatsächlich gegen die Velux-CA geprüft (bisher wirkungslos); abgelaufene Velux-CA wird toleriert
- Sicherheit: `GET /api/config` gibt Passwort und API-Token nicht mehr zurück; Konfigdateien werden mit `0600` geschrieben; Warnung im Log, wenn kein API-Token gesetzt ist
- Fix: Web-UI funktioniert mit gesetztem API-Token (Token-Eingabe im Dashboard, wird im Browser gespeichert)
- Fix: API-Token- und Log-Level-Änderungen über `POST /api/config` wirken sofort statt erst nach Neustart
- Fix: Fallback auf Umgebungsvariablen (`KLF200_HOST`, `KLF200_PASSWORD`) ohne Konfigdatei funktioniert jetzt
- Fix: Reconnect-Race, bei der eine alte Lese-Schleife die neue Verbindung als getrennt markierte
- Fix: Hintergrund-Loops starten auch ohne KLF-200-Konfiguration, spätere Konfiguration per API wird vollständig übernommen
- Fix: unvollständige Node-Listen bei Timeout ersetzen den Cache nicht mehr
- Neu: Keep-alive (`GW_GET_STATE_REQ`) alle 60 s, tote Verbindungen werden erkannt und neu aufgebaut
- Neu: Sensorstatus wird pro Node gehalten und beim periodischen Refresh mit aktualisiert
- Neu: Mappings werden validiert (doppelte IDs, doppelte Loxone-IDs, doppelt gemappte Nodes → HTTP 400)
- Add-on: Passwort/Token werden JSON-escaped in die Konfiguration geschrieben; Build verwendet das Git-Tag `v<version>`, sofern vorhanden
- Add-on: `api_token` muss leer oder mindestens 16 Zeichen lang sein (Schema)
- Doku: Port 8099, UDP-Feedback und Mapping-API dokumentiert; README und LICENSE ergänzt
- Intern: Unit-Tests für Protokoll, Konfiguration und Mappings; CI-Workflow; ungenutzte Abhängigkeiten und Code entfernt; `package-lock.json` für reproduzierbare Frontend-Builds

## 1.2.0

- Fix: Positionsanzeige/UDP-Feedback wurde nach Run-Status-Meldungen fälschlich auf 0 % gesetzt
- Fix: Loxone-Mappings und UDP-Feedback überleben jetzt Add-on-Neustarts (separate `loxone.yaml`)
- Fix: `Stop`-Kommando wartet nun auf Bestätigung des KLF-200 und meldet Fehler
- Fix: Sensor-Refresh läuft nicht mehr in einen toten Sammel-Loop (schneller, korrekt)
- KLF-200-Operationen werden serialisiert, damit Antworten nicht vertauscht werden
- KLF-200-Einstellungsänderungen lösen automatisch einen Reconnect aus
- Sicherheit: KLF-200-Passwort wird nicht mehr im Debug-Log ausgegeben
- Interne Aufräumarbeiten (Data-Race im Reader, Config-Kopien, toter Code)

## 1.1.0

- UDP-Feedback an Loxone Miniserver (Echtzeit-Push von Position, State, Sensoren)
- Node-zu-Loxone Mapping-System mit CRUD-API (`/api/mappings`)
- Loxone-Konfiguration via API (`/api/loxone/config`)
- UDP-Test-Endpoint (`/api/loxone/config/udp/test`)
- Versionsnummer im Frontend-Header angezeigt

## 1.0.8

- Korrekter s6-overlay v3 longrun Service statt CMD/ENTRYPOINT
- HA Base Image wiederhergestellt (s6-overlay Kompatibilität)
- bashio durch /bin/sh + jq ersetzt für zuverlässiges Options-Parsing
- Liest HA-Optionen direkt aus /data/options.json

## 1.0.0

- Initial Home Assistant Add-on Release
- Web-Dashboard mit Echtzeit-Geräteüberwachung
- Loxone-kompatible REST API Endpunkte
- Regen- und Windsensor-Unterstützung
- Automatische KLF-200 Wiederverbindung
- Persistente Konfiguration
- Home Assistant Ingress Support
