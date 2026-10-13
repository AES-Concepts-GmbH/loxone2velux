# Loxone2Velux Gateway

## Über dieses Add-on

Dieses Add-on stellt eine Brücke zwischen Loxone Hausautomation und dem Velux
KLF-200 Gateway her. Es ermöglicht die Steuerung von Velux Fenstern, Rollläden
und Jalousien über Loxones HTTP Virtual Outputs oder das integrierte Web-Interface.

## Funktionen

- Steuerung von Velux Fenstern, Rollläden und Jalousien via REST API
- Web-Dashboard mit Echtzeit-Statusanzeige
- Loxone-kompatible HTTP-Endpunkte (einfache GET-Requests)
- UDP-Feedback an den Loxone Miniserver (Position, Zustand, Sensoren)
- Regen- und Windsensor-Status vom KLF-200
- Automatische Wiederverbindung und Keep-alive zum KLF-200
- Persistente Konfiguration der Loxone-Mappings

## Konfiguration

Die Konfiguration erfolgt ausschliesslich über die Add-on-Einstellungen in
Home Assistant. Mappings und UDP-Feedback werden über die REST API verwaltet
(siehe unten) und überleben Add-on-Neustarts.

### KLF-200 Einstellungen

- **klf200_host** (erforderlich): IP-Adresse oder Hostname des KLF-200
- **klf200_password** (erforderlich): WLAN-Passwort auf der Rückseite des KLF-200
- **klf200_port** (Standard: 51200): TCP-Port des KLF-200
- **reconnect_interval** (Standard: 30): Sekunden zwischen Reconnect-Versuchen
- **refresh_interval** (Standard: 300): Sekunden zwischen Status-Aktualisierungen
  (Geräteliste und Sensoren)

### Weitere Einstellungen

- **log_level** (Standard: info): Log-Level (debug, info, warn, error)
- **api_token** (empfohlen): API-Token für Authentifizierung, mindestens
  16 Zeichen. Leer lassen um Authentifizierung zu deaktivieren. **Ohne Token
  kann jedes Gerät im Netzwerk die Fenster über Port 8099 steuern.**
  Token erzeugen z. B. mit `openssl rand -hex 32`.

## Web-Interface

Das Web-Interface ist über die Home Assistant Sidebar erreichbar (Ingress).
Das Dashboard zeigt alle verbundenen Velux Geräte mit Echtzeit-Positionsanzeige.
Geräte können geöffnet, geschlossen, gestoppt oder auf eine bestimmte Position
gefahren werden.

Ist ein `api_token` gesetzt, fragt das Dashboard beim ersten Aufruf nach dem
Token und speichert ihn im Browser.

## Loxone Integration

Das Add-on ist für den Loxone Miniserver direkt (ohne Ingress) unter
`http://<HA_IP>:8099` erreichbar. Konfiguriere Virtual Outputs für folgende
Endpunkte:

### Steuerung

| Aktion        | URL                                              |
| ------------- | ------------------------------------------------ |
| Öffnen        | `http://<HA_IP>:8099/loxone/node/{id}/open`      |
| Schliessen    | `http://<HA_IP>:8099/loxone/node/{id}/close`     |
| Stopp         | `http://<HA_IP>:8099/loxone/node/{id}/stop`      |
| Position      | `http://<HA_IP>:8099/loxone/node/{id}/set/{pct}` |
| Position lesen| `http://<HA_IP>:8099/loxone/node/{id}/position`  |

Die Antwort ist `OK` bzw. `ERROR` (Position lesen: Zahl 0–100).
Position 0 = offen, 100 = geschlossen.

### Sensoren

| Daten         | URL                                              | Antwort      |
| ------------- | ------------------------------------------------ | ------------ |
| Alle Sensoren | `http://<HA_IP>:8099/loxone/sensors`             | `regen;wind` |
| Nur Regen     | `http://<HA_IP>:8099/loxone/sensors/rain`        | `0` / `1`    |
| Nur Wind      | `http://<HA_IP>:8099/loxone/sensors/wind`        | `0` / `1`    |

Ersetze `{id}` mit der Velux Node-ID (siehe Dashboard) und `{pct}` mit 0-100.

Falls ein API-Token gesetzt ist, `?token=DEIN_TOKEN` an die URL anhängen
(oder Header `Authorization: Bearer DEIN_TOKEN`).

### UDP-Feedback (Push an den Miniserver)

Statt den Status zu pollen, kann das Add-on Änderungen per UDP an einen
virtuellen UDP-Eingang des Miniservers senden. Format pro Paket:

```
<loxone_id>/<eigenschaft>:<wert>
```

Eigenschaften: `position` (0–100), `target` (0–100), `state` (KLF-200
Node-State), `rain` (0/1), `wind` (0/1).

UDP-Feedback aktivieren (Beispiel mit `curl`, `-H` weglassen ohne Token):

```
curl -X PUT http://<HA_IP>:8099/api/loxone/config/udp \
  -H "Authorization: Bearer DEIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true, "ip": "192.168.1.10", "port": 7777}'
```

Testpaket senden: `POST /api/loxone/config/udp/test`.

### Mappings (Node → Loxone-ID)

Nur gemappte Nodes senden UDP-Feedback. Mappings werden über die REST API
verwaltet und in `/config/loxone2velux/loxone.yaml` gespeichert:

| Methode | Pfad                   | Beschreibung                          |
| ------- | ---------------------- | ------------------------------------- |
| GET     | `/api/mappings`        | Alle Mappings                         |
| POST    | `/api/mappings`        | Mapping anlegen                       |
| PUT     | `/api/mappings/{id}`   | Mapping ändern                        |
| DELETE  | `/api/mappings/{id}`   | Mapping löschen                       |

Beispiel:

```
curl -X POST http://<HA_IP>:8099/api/mappings \
  -H "Authorization: Bearer DEIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "Dachfenster Wohnzimmer", "node_id": 0, "loxone_id": "dachfenster_wz"}'
```

Jede `loxone_id` und jeder aktive Node darf nur einmal gemappt sein.

## Netzwerk

Der KLF-200 muss vom Home Assistant Host auf Port 51200 (TCP/TLS) erreichbar
sein. Stelle sicher, dass deine Netzwerkkonfiguration dies erlaubt.

Port 8099 wird für den direkten Loxone-Zugriff auf dem Host exponiert.

## Support

Issues und Feature-Requests:
https://github.com/stefanbeyeler/loxone2velux/issues
