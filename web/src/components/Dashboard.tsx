import { useState, useEffect, useCallback, useRef } from 'react';
import { NodeList } from './NodeList';
import { LoxoneGuide } from './LoxoneGuide';
import { SensorCard } from './SensorCard';
import * as api from '../services/api';
import { Node, HealthResponse, SensorStatus } from '../types';
import {
  Blinds,
  BookOpen,
  RefreshCw,
  Wifi,
  WifiOff,
  Menu,
  X,
  Server,
  PlugZap,
  KeyRound,
} from 'lucide-react';


type Tab = 'devices' | 'guide';

export function Dashboard() {
  const latestPoll = useRef(0);
  const pollInFlight = useRef(false);
  const [activeTab, setActiveTab] = useState<Tab>('devices');
  const [menuOpen, setMenuOpen] = useState(false);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [targets, setTargets] = useState<Record<number, number>>({});
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [sensors, setSensors] = useState<SensorStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [sensorsRefreshing, setSensorsRefreshing] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Token prompt: shown when the gateway answers 401
  const [authRequired, setAuthRequired] = useState(false);
  const [tokenInput, setTokenInput] = useState('');

  const handleError = useCallback((err: unknown, fallback: string) => {
    if (err instanceof api.AuthError) {
      setAuthRequired(true);
      setError(null);
      return;
    }
    setError(err instanceof Error ? err.message : fallback);
  }, []);

  const fetchData = useCallback(async () => {
    if (pollInFlight.current) return;
    pollInFlight.current = true;
    const poll = ++latestPoll.current;
    setLoading(true);
    setError(null);

    try {
      const [healthData, nodesData, sensorData] = await Promise.all([
        api.getHealth(),
        api.getNodes(),
        api.getSensorStatus().catch((err) => {
          if (err instanceof api.AuthError) throw err;
          return null;
        }),
      ]);

      if (poll !== latestPoll.current) return;
      setHealth(healthData);
      setNodes(nodesData.nodes || []);
      setSensors(sensorData);
      setTargets(current => Object.fromEntries(Object.entries(current).filter(([id, target]) => {
        const node = nodesData.nodes.find(n => n.id === Number(id));
        return node && !node.stale && node.state_str !== "Error" && Math.abs(node.position_percent - target) > 1;
      })));
      setAuthRequired(false);
    } catch (err) {
      if (poll !== latestPoll.current) return;
      setHealth(null);
      setSensors(null);
      setNodes(current => current.map(node => ({ ...node, stale: true })));
      handleError(err, 'Fehler beim Laden');
    } finally {
      pollInFlight.current = false;
      if (poll === latestPoll.current) setLoading(false);
    }
  }, [handleError]);

  const handleTokenSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    api.setToken(tokenInput.trim());
    setTokenInput('');
    fetchData();
  };

  const handleTokenClear = () => {
    api.setToken('');
    setAuthRequired(true);
  };

  const handleRefreshSensors = async () => {
    setSensorsRefreshing(true);
    try {
      const sensorData = await api.refreshSensorStatus();
      setSensors(sensorData);
    } catch (err) {
      setSensors(current => current ? { ...current, valid: false } : null);
      handleError(err, 'Sensor-Abfrage fehlgeschlagen');
    } finally {
      setSensorsRefreshing(false);
    }
  };

  const handleReconnect = async () => {
    setReconnecting(true);
    setError(null);
    try {
      const result = await api.reconnectGateway();
      if (!result.success) {
        setError(result.message || 'Verbindung fehlgeschlagen');
      }
      // Refresh data after reconnect
      setTimeout(fetchData, 1000);
    } catch (err) {
      handleError(err, 'Fehler beim Neuverbinden');
    } finally {
      setReconnecting(false);
    }
  };

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 10000); // Refresh every 10s
    return () => clearInterval(interval);
  }, [fetchData]);

  // Acknowledged targets are separate from measured positions.
  const applyPosition = (id: number, position: number) => {
    setTargets(current => ({ ...current, [id]: position }));
  };

  const handleSetPosition = async (id: number, position: number) => {
    try {
      await api.setNodePosition(id, position);
      applyPosition(id, position);
    } catch (err) {
      handleError(err, 'Fehler');
    }
  };

  const handleOpen = async (id: number) => {
    try {
      await api.openNode(id);
      applyPosition(id, 0);
    } catch (err) {
      handleError(err, 'Fehler');
    }
  };

  const handleClose = async (id: number) => {
    try {
      await api.closeNode(id);
      applyPosition(id, 100);
    } catch (err) {
      handleError(err, 'Fehler');
    }
  };

  const handleStop = async (id: number) => {
    try {
      await api.stopNode(id);
      setTargets(current => {
        const remaining = { ...current };
        delete remaining[id];
        return remaining;
      });
    } catch (err) {
      handleError(err, 'Fehler');
    }
  };

  const isConnected = health?.connected ?? false;

  const tabs = [
    { id: 'devices' as Tab, label: 'Geräte', icon: Blinds, count: nodes.length },
    { id: 'guide' as Tab, label: 'Loxone Anleitung', icon: BookOpen },
  ];

  return (
    <div className="min-h-screen bg-gray-900">
      {/* Header */}
      <header className="bg-gray-800 border-b border-gray-700 sticky top-0 z-10">
        <div className="max-w-6xl mx-auto px-4 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-velux-blue rounded-lg flex items-center justify-center">
                <Blinds size={24} className="text-white" />
              </div>
              <div>
                <h1 className="text-xl font-bold text-white">Loxone2Velux</h1>
                <p className="text-xs text-gray-400">Gateway Service {health?.version ? `v${health.version}` : ''}</p>
              </div>
            </div>

            <div className="flex items-center gap-4">
              {/* Gateway Status */}
              <div className="hidden md:flex items-center gap-2 bg-gray-700/50 px-3 py-1.5 rounded-lg">
                <Server size={16} className="text-velux-blue" />
                <span className="text-sm text-gray-300">KLF-200</span>
              </div>

              {/* Connection Status */}
              <div className="hidden sm:flex items-center gap-2">
                {isConnected ? (
                  <Wifi size={18} className="text-green-500" />
                ) : (
                  <WifiOff size={18} className="text-red-500" />
                )}
                <span className="text-sm text-gray-400">
                  {health === null ? 'Status unbekannt' : isConnected ? 'Verbunden' : 'Getrennt'}
                </span>
              </div>

              {/* Token Button (only shown when a token is stored) */}
              {api.getToken() && (
                <button
                  onClick={handleTokenClear}
                  className="p-2 text-gray-400 hover:text-white transition-colors"
                  title="API-Token ändern"
                >
                  <KeyRound size={20} />
                </button>
              )}

              {/* Reconnect Button */}
              <button
                onClick={handleReconnect}
                disabled={reconnecting}
                className="p-2 text-gray-400 hover:text-yellow-400 transition-colors disabled:opacity-50"
                title="Neu verbinden"
              >
                <PlugZap size={20} className={reconnecting ? 'animate-pulse' : ''} />
              </button>

              {/* Refresh Button */}
              <button
                onClick={fetchData}
                disabled={loading}
                className="p-2 text-gray-400 hover:text-white transition-colors"
                title="Aktualisieren"
              >
                <RefreshCw size={20} className={loading ? 'animate-spin' : ''} />
              </button>

              {/* Mobile Menu Toggle */}
              <button
                onClick={() => setMenuOpen(!menuOpen)}
                className="sm:hidden p-2 text-gray-400"
              >
                {menuOpen ? <X size={24} /> : <Menu size={24} />}
              </button>
            </div>
          </div>

          {/* Desktop Navigation */}
          <nav className="hidden sm:flex gap-1 mt-4">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`
                  flex items-center gap-2 px-4 py-2 rounded-lg transition-colors
                  ${activeTab === tab.id
                    ? 'bg-velux-blue text-white'
                    : 'text-gray-400 hover:text-white hover:bg-gray-700'
                  }
                `}
              >
                <tab.icon size={18} />
                {tab.label}
                {tab.count !== undefined && (
                  <span className={`
                    text-xs px-2 py-0.5 rounded-full
                    ${activeTab === tab.id ? 'bg-white/20' : 'bg-gray-700'}
                  `}>
                    {tab.count}
                  </span>
                )}
              </button>
            ))}
          </nav>

          {/* Mobile Navigation */}
          {menuOpen && (
            <nav className="sm:hidden flex flex-col gap-1 mt-4 pb-2">
              {tabs.map((tab) => (
                <button
                  key={tab.id}
                  onClick={() => {
                    setActiveTab(tab.id);
                    setMenuOpen(false);
                  }}
                  className={`
                    flex items-center gap-3 px-4 py-3 rounded-lg transition-colors
                    ${activeTab === tab.id
                      ? 'bg-velux-blue text-white'
                      : 'text-gray-400 hover:bg-gray-700'
                    }
                  `}
                >
                  <tab.icon size={20} />
                  {tab.label}
                  {tab.count !== undefined && (
                    <span className="text-xs bg-gray-700 px-2 py-0.5 rounded-full ml-auto">
                      {tab.count}
                    </span>
                  )}
                </button>
              ))}
            </nav>
          )}
        </div>
      </header>

      {/* Token Prompt */}
      {authRequired && (
        <div className="bg-amber-900/30 border-b border-amber-500 px-4 py-3">
          <form onSubmit={handleTokenSubmit} className="max-w-6xl mx-auto flex flex-col sm:flex-row sm:items-center gap-3">
            <p className="text-amber-300 text-sm flex-1">
              Das Gateway verlangt einen API-Token (Add-on-Einstellung <code className="bg-gray-900 px-1 rounded">api_token</code>).
            </p>
            <input
              type="password"
              value={tokenInput}
              onChange={(e) => setTokenInput(e.target.value)}
              placeholder="API-Token"
              autoComplete="off"
              className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-1.5 text-sm text-white w-full sm:w-72"
            />
            <button
              type="submit"
              disabled={tokenInput.trim().length === 0}
              className="bg-velux-blue text-white text-sm px-4 py-1.5 rounded-lg disabled:opacity-50"
            >
              Speichern
            </button>
          </form>
        </div>
      )}

      {/* Error Banner */}
      {error && (
        <div className="bg-red-900/30 border-b border-red-500 px-4 py-3">
          <div className="max-w-6xl mx-auto">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        </div>
      )}

      {/* Main Content */}
      <main className="max-w-6xl mx-auto px-4 py-6">
        {activeTab === 'devices' && (
          <div className="space-y-6">
            {/* Sensor Status */}
            <SensorCard
              sensors={sensors}
              onRefresh={handleRefreshSensors}
              isRefreshing={sensorsRefreshing}
            />

            {/* Node List */}
            <NodeList
              nodes={nodes}
              targets={targets}
              loading={loading}
              onSetPosition={handleSetPosition}
              onOpen={handleOpen}
              onClose={handleClose}
              onStop={handleStop}
              onRefresh={fetchData}
            />
          </div>
        )}

        {activeTab === 'guide' && <LoxoneGuide />}
      </main>

      {/* Footer */}
      <footer className="border-t border-gray-800 py-4 mt-8">
        <div className="max-w-6xl mx-auto px-4 text-center text-xs text-gray-500 space-y-1">
          <div>
            Loxone2Velux Gateway v{health?.version || __APP_VERSION__}
          </div>
          <div className="text-gray-600">
            &copy; 2026 Stefan Beyeler
          </div>
        </div>
      </footer>
    </div>
  );
}
