import { useState } from 'react';
import {
  ChevronDown,
  ChevronRight,
  Copy,
  Check,
} from 'lucide-react';
import { getToken } from '../services/api';

interface CodeBlockProps {
  code: string;
  language?: string;
}

function CodeBlock({ code }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="relative group">
      <pre className="bg-gray-900 rounded-lg p-4 overflow-x-auto text-sm">
        <code className="text-gray-300">{code}</code>
      </pre>
      <button
        onClick={handleCopy}
        className="absolute top-2 right-2 p-2 bg-gray-700 hover:bg-gray-600 rounded-lg opacity-0 group-hover:opacity-100 transition-opacity"
      >
        {copied ? (
          <Check size={16} className="text-green-400" />
        ) : (
          <Copy size={16} className="text-gray-400" />
        )}
      </button>
    </div>
  );
}

interface AccordionProps {
  title: string;
  children: React.ReactNode;
  defaultOpen?: boolean;
}

function Accordion({ title, children, defaultOpen = false }: AccordionProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);

  return (
    <div className="border border-gray-700 rounded-lg overflow-hidden">
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-full flex items-center justify-between p-4 bg-gray-800 hover:bg-gray-750 transition-colors"
      >
        <span className="font-medium text-white">{title}</span>
        {isOpen ? (
          <ChevronDown size={20} className="text-gray-400" />
        ) : (
          <ChevronRight size={20} className="text-gray-400" />
        )}
      </button>
      {isOpen && (
        <div className="p-4 bg-gray-800/50 border-t border-gray-700">
          {children}
        </div>
      )}
    </div>
  );
}

export function LoxoneGuide() {
  const token = getToken();
  const hasToken = token.length > 0;
  // Show token suffix only if auth is enabled
  const tokenSuffix = hasToken ? `?token=${encodeURIComponent(token)}` : '';
  // Gateway direct address for Loxone (not via HA Ingress)
  const [gatewayUrl, setGatewayUrl] = useState(() => {
    try {
      return localStorage.getItem('gateway-direct-url') || window.location.origin;
    } catch {
      return window.location.origin;
    }
  });

  return (
    <div className="space-y-6">
      <div className="bg-gray-800 rounded-xl p-6">
        <h2 className="text-xl font-bold text-white mb-2">Loxone Integration</h2>
        <p className="text-gray-400">
          Anleitung zur Einbindung der Velux-Geräte in Loxone über Virtual HTTP Outputs.
        </p>
        <label className="block mt-4 text-sm text-gray-300">
          Direkte Gateway-Adresse (Docker: 8080, HA: 8099)
          <input aria-label="Direkte Gateway-Adresse" type="url" value={gatewayUrl}
            onChange={e => {
              setGatewayUrl(e.target.value);
              try { localStorage.setItem('gateway-direct-url', e.target.value); } catch { /* Session only when storage is unavailable. */ }
            }}
            className="block mt-1 w-full bg-gray-900 border border-gray-700 rounded-lg p-2" />
        </label>
        <p className="text-sm text-gray-400 mt-2">Die Adresse muss vom Miniserver direkt erreichbar sein. Bei Home Assistant die Ingress-Adresse durch die lokale Gateway-Adresse mit Port 8099 ersetzen.</p>
        {!hasToken && (
          <div className="mt-3 p-3 bg-green-900/30 border border-green-700 rounded-lg">
            <p className="text-sm text-green-400">
              Kein Token im Browser gespeichert. Falls das Gateway einen Token verlangt, zuerst anmelden. Die Beispiele enthalten derzeit keinen <code className="bg-gray-900 px-1 rounded">?token=...</code> Zusatz.
            </p>
          </div>
        )}
      </div>

      <div className="space-y-4">
        <Accordion title="1. Virtual HTTP Output erstellen" defaultOpen>
          <div className="space-y-4 text-gray-300">
            <p>
              Erstelle in Loxone Config einen neuen <strong>Virtual HTTP Output</strong>:
            </p>
            <ol className="list-decimal list-inside space-y-2 ml-4">
              <li>Rechtsklick auf "Virtual Outputs" → "Virtuellen HTTP Ausgang hinzufügen"</li>
              <li>Adresse des Gateways eintragen: <code className="bg-gray-900 px-2 py-0.5 rounded">{gatewayUrl}</code></li>
              <li>Keine Authentifizierung konfigurieren (Token wird per URL übergeben)</li>
            </ol>
          </div>
        </Accordion>

        <Accordion title="2. Virtual Output Commands">
          <div className="space-y-4 text-gray-300">
            <p>Füge die folgenden Befehle zum Virtual HTTP Output hinzu:</p>

            <div className="space-y-4">
              <div>
                <h4 className="font-medium text-white mb-2">Position setzen (0-100%)</h4>
                <CodeBlock code={`/loxone/node/<ID>/set/<v>${tokenSuffix}`} />
                <p className="text-sm text-gray-400 mt-2">
                  Ersetze <code>&lt;ID&gt;</code> mit der Geräte-ID und <code>&lt;v&gt;</code> mit dem Analogwert.
                </p>
                <div className="mt-2 p-3 bg-gray-900 rounded-lg text-sm">
                  <p className="text-yellow-400 font-medium mb-1">Position-Semantik (Velux Standard):</p>
                  <ul className="text-gray-400 space-y-1">
                    <li>• <code>0</code> = Vollständig geöffnet</li>
                    <li>• <code>100</code> = Vollständig geschlossen</li>
                    <li>• <code>50</code> = Halb geöffnet/geschlossen</li>
                  </ul>
                </div>
              </div>

              <div>
                <h4 className="font-medium text-white mb-2">Öffnen</h4>
                <CodeBlock code={`/loxone/node/<ID>/open${tokenSuffix}`} />
              </div>

              <div>
                <h4 className="font-medium text-white mb-2">Schließen</h4>
                <CodeBlock code={`/loxone/node/<ID>/close${tokenSuffix}`} />
              </div>

              <div>
                <h4 className="font-medium text-white mb-2">Stoppen</h4>
                <CodeBlock code={`/loxone/node/<ID>/stop${tokenSuffix}`} />
              </div>
            </div>
          </div>
        </Accordion>

        <Accordion title="3. Position abfragen (Virtual HTTP Input)">
          <div className="space-y-4 text-gray-300">
            <p>
              Die aktuelle Position eines Geräts kann als <strong>Virtual HTTP Input</strong> abgefragt werden.
            </p>

            <div>
              <h4 className="font-medium text-white mb-2">Position lesen (0-100)</h4>
              <CodeBlock code={`/loxone/node/<ID>/position${tokenSuffix}`} />
              <p className="text-sm text-gray-400 mt-2">
                Rückgabe: Zahl von <code>0</code> (offen) bis <code>100</code> (geschlossen).
              </p>
            </div>

            <div className="mt-4 p-3 bg-gray-900 rounded-lg">
              <p className="text-sm text-gray-400">
                <span className="text-yellow-400 font-medium">Tipp:</span> In Loxone Config einen <strong>Virtual HTTP Input</strong> erstellen,
                die URL <code>{gatewayUrl}/loxone/node/0/position</code> eintragen
                und als Abfrageintervall z.B. 10 Sekunden einstellen.
              </p>
            </div>
          </div>
        </Accordion>

        <Accordion title="4. Sensor-Abfrage (Regen/Wind)">
          <div className="space-y-4 text-gray-300">
            <p>
              Der KLF-200 kann den Status von Regen- und Windsensoren abfragen. Diese können als Virtual HTTP Input in Loxone eingebunden werden.
            </p>

            <div className="space-y-4">
              <div>
                <h4 className="font-medium text-white mb-2">Alle Sensoren (Format: regen;wind)</h4>
                <CodeBlock code={`/loxone/sensors${tokenSuffix}`} />
                <p className="text-sm text-gray-400 mt-2">
                  Rückgabe: <code>0;0</code> (kein Regen, kein Wind) oder <code>1;0</code> (Regen erkannt) etc.
                </p>
              </div>

              <div>
                <h4 className="font-medium text-white mb-2">Nur Regensensor</h4>
                <CodeBlock code={`/loxone/sensors/rain${tokenSuffix}`} />
                <p className="text-sm text-gray-400 mt-2">
                  Rückgabe: <code>0</code> (trocken) oder <code>1</code> (Regen erkannt)
                </p>
              </div>

              <div>
                <h4 className="font-medium text-white mb-2">Nur Windsensor</h4>
                <CodeBlock code={`/loxone/sensors/wind${tokenSuffix}`} />
                <p className="text-sm text-gray-400 mt-2">
                  Rückgabe: <code>0</code> (kein Wind) oder <code>1</code> (Wind erkannt)
                </p>
              </div>
            </div>

            <div className="mt-4 p-3 bg-gray-900 rounded-lg">
              <p className="text-sm text-gray-400">
                <span className="text-yellow-400 font-medium">Tipp:</span> In Loxone Config einen <strong>Virtual HTTP Input</strong> erstellen
                und als Abfrageintervall z.B. 60 Sekunden einstellen. Die Werte können dann für Automatisierungen verwendet werden
                (z.B. Fenster bei Regen automatisch schließen).
              </p>
            </div>
          </div>
        </Accordion>

        <Accordion title="5. Beispiel: Fenster-Baustein">
          <div className="space-y-4 text-gray-300">
            <p>
              So verbindest du einen Jalousie-Baustein mit dem Gateway:
            </p>
            <ol className="list-decimal list-inside space-y-2 ml-4">
              <li>Erstelle einen <strong>Jalousie</strong> oder <strong>Beschattung</strong> Baustein</li>
              <li>Verbinde die Ausgänge mit dem Virtual HTTP Output:</li>
            </ol>

            <div className="mt-4 bg-gray-900 rounded-lg p-4">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-700">
                    <th className="text-left py-2 text-gray-400">Ausgang</th>
                    <th className="text-left py-2 text-gray-400">HTTP Befehl</th>
                  </tr>
                </thead>
                <tbody className="font-mono">
                  <tr className="border-b border-gray-800">
                    <td className="py-2">AQ (Position)</td>
                    <td className="py-2 text-velux-blue">/loxone/node/0/set/&lt;v&gt;{hasToken && '?token=...'}</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2">QU (Hoch)</td>
                    <td className="py-2 text-velux-blue">/loxone/node/0/open{hasToken && '?token=...'}</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2">QD (Runter)</td>
                    <td className="py-2 text-velux-blue">/loxone/node/0/close{hasToken && '?token=...'}</td>
                  </tr>
                  <tr>
                    <td className="py-2">QS (Stopp)</td>
                    <td className="py-2 text-velux-blue">/loxone/node/0/stop{hasToken && '?token=...'}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </Accordion>

        <Accordion title="6. REST API Referenz">
          <div className="space-y-4 text-gray-300">
            <p>Vollständige API-Endpunkte:</p>

            <div className="bg-gray-900 rounded-lg p-4 overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-700">
                    <th className="text-left py-2 text-gray-400">Methode</th>
                    <th className="text-left py-2 text-gray-400">Endpoint</th>
                    <th className="text-left py-2 text-gray-400">Beschreibung</th>
                  </tr>
                </thead>
                <tbody className="font-mono">
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/health</td>
                    <td className="py-2 font-sans text-gray-400">Status (ohne Auth)</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/api/nodes</td>
                    <td className="py-2 font-sans text-gray-400">Alle Geräte</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/api/nodes/&#123;id&#125;</td>
                    <td className="py-2 font-sans text-gray-400">Einzelnes Gerät</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-yellow-400">POST</td>
                    <td className="py-2">/api/nodes/&#123;id&#125;/position</td>
                    <td className="py-2 font-sans text-gray-400">Position setzen</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-yellow-400">POST</td>
                    <td className="py-2">/api/nodes/&#123;id&#125;/open</td>
                    <td className="py-2 font-sans text-gray-400">Öffnen</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-yellow-400">POST</td>
                    <td className="py-2">/api/nodes/&#123;id&#125;/close</td>
                    <td className="py-2 font-sans text-gray-400">Schließen</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-yellow-400">POST</td>
                    <td className="py-2">/api/nodes/&#123;id&#125;/stop</td>
                    <td className="py-2 font-sans text-gray-400">Stoppen</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/api/sensors</td>
                    <td className="py-2 font-sans text-gray-400">Sensor-Status (JSON)</td>
                  </tr>
                  <tr>
                    <td className="py-2 text-yellow-400">POST</td>
                    <td className="py-2">/api/sensors/refresh</td>
                    <td className="py-2 font-sans text-gray-400">Sensoren aktualisieren</td>
                  </tr>
                </tbody>
              </table>
            </div>

            <h4 className="font-medium text-white mt-6 mb-2">Loxone-Endpoints (einfache Rückgabewerte)</h4>
            <div className="bg-gray-900 rounded-lg p-4 overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-700">
                    <th className="text-left py-2 text-gray-400">Methode</th>
                    <th className="text-left py-2 text-gray-400">Endpoint</th>
                    <th className="text-left py-2 text-gray-400">Rückgabe</th>
                  </tr>
                </thead>
                <tbody className="font-mono">
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/loxone/node/&#123;id&#125;/position</td>
                    <td className="py-2 font-sans text-gray-400">0-100 (Position)</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/loxone/node/&#123;id&#125;/set/&#123;pos&#125;</td>
                    <td className="py-2 font-sans text-gray-400">OK / ERROR</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/loxone/node/&#123;id&#125;/open</td>
                    <td className="py-2 font-sans text-gray-400">OK / ERROR</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/loxone/node/&#123;id&#125;/close</td>
                    <td className="py-2 font-sans text-gray-400">OK / ERROR</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/loxone/node/&#123;id&#125;/stop</td>
                    <td className="py-2 font-sans text-gray-400">OK / ERROR</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/loxone/sensors</td>
                    <td className="py-2 font-sans text-gray-400">regen;wind (0;0)</td>
                  </tr>
                  <tr className="border-b border-gray-800">
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/loxone/sensors/rain</td>
                    <td className="py-2 font-sans text-gray-400">0 oder 1</td>
                  </tr>
                  <tr>
                    <td className="py-2 text-green-400">GET</td>
                    <td className="py-2">/loxone/sensors/wind</td>
                    <td className="py-2 font-sans text-gray-400">0 oder 1</td>
                  </tr>
                </tbody>
              </table>
            </div>

            {hasToken ? (
              <p className="text-sm text-gray-400 mt-4">
                Authentifizierung: <code className="bg-gray-900 px-2 py-0.5 rounded">Authorization: Bearer &#123;token&#125;</code> oder <code className="bg-gray-900 px-2 py-0.5 rounded">?token=&#123;token&#125;</code>
              </p>
            ) : (
              <p className="text-sm text-green-400 mt-4">
                Kein Token im Browser gespeichert. Prüfe in den Gateway-Einstellungen, ob ein API-Token erforderlich ist.
              </p>
            )}
          </div>
        </Accordion>

        <Accordion title="7. Troubleshooting">
          <div className="space-y-4 text-gray-300">
            <div>
              <h4 className="font-medium text-white mb-2">Gateway nicht erreichbar</h4>
              <ul className="list-disc list-inside space-y-1 ml-4 text-gray-400">
                <li>Prüfe ob das Add-on in Home Assistant läuft</li>
                <li>Prüfe die Add-on Logs in Home Assistant</li>
                <li>Stelle sicher, dass die direkte Gateway-Adresse vom Miniserver erreichbar ist</li>
                <li>Gateway-Adresse: <code>{gatewayUrl}</code></li>
              </ul>
            </div>

            <div>
              <h4 className="font-medium text-white mb-2">KLF-200 nicht verbunden</h4>
              <ul className="list-disc list-inside space-y-1 ml-4 text-gray-400">
                <li>Überprüfe IP-Adresse und Passwort in den Add-on Einstellungen</li>
                <li>Das Passwort steht auf der Rückseite des KLF-200</li>
                <li>Stelle sicher, dass der KLF-200 im selben Netzwerk ist</li>
              </ul>
            </div>

            <div>
              <h4 className="font-medium text-white mb-2">401 Unauthorized</h4>
              <ul className="list-disc list-inside space-y-1 ml-4 text-gray-400">
                <li>API Token prüfen (min. 16 Zeichen)</li>
                <li>Token korrekt in URL oder Header übergeben</li>
              </ul>
            </div>
          </div>
        </Accordion>
      </div>
    </div>
  );
}
