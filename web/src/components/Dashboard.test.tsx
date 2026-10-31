import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { Dashboard } from './Dashboard';
import * as api from '../services/api';
import { Node } from '../types';

const node: Node = {
  id: 1,
  name: 'Rollladen',
  node_type: 0x80,
  node_type_str: 'Roller Shutter',
  state: 5,
  state_str: 'Done',
  current_position_raw: 0,
  position_percent: 0,
  target_position_raw: 0,
  target_percent: 0,
  velocity: 0,
  last_update: new Date().toISOString(),
  inverted: false,
};

const health = { status: 'ok' as const, connected: true, node_count: 1, version: '1.3.0' };
const sensors = { valid: true, rain_detected: false, wind_detected: false, last_update: new Date().toISOString() };

describe('Dashboard', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(api, 'getHealth').mockResolvedValue(health);
    vi.spyOn(api, 'getNodes').mockResolvedValue({ nodes: [node], count: 1 });
    vi.spyOn(api, 'getSensorStatus').mockResolvedValue(sensors);
  });

  it('renders health, version and nodes', async () => {
    render(<Dashboard />);
    expect(await screen.findByText('Rollladen')).toBeInTheDocument();
    expect(screen.getByText('Gateway Service v1.3.0')).toBeInTheDocument();
    expect(screen.getByText('Verbunden')).toBeInTheDocument();
  });

  it('shows the token prompt on 401 and retries with the entered token', async () => {
    const getNodes = vi.spyOn(api, 'getNodes')
      .mockRejectedValueOnce(new api.AuthError())
      .mockResolvedValue({ nodes: [node], count: 1 });

    render(<Dashboard />);
    const input = await screen.findByPlaceholderText('API-Token');
    const save = screen.getByRole('button', { name: 'Speichern' });
    expect(save).toBeDisabled();

    fireEvent.change(input, { target: { value: '  my-token  ' } });
    expect(save).toBeEnabled();
    fireEvent.click(save);

    expect(api.getToken()).toBe('my-token');
    expect(await screen.findByText('Rollladen')).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByPlaceholderText('API-Token')).not.toBeInTheDocument());
    expect(getNodes).toHaveBeenCalledTimes(2);

    // The key button clears the token and re-opens the prompt
    fireEvent.click(screen.getByTitle('API-Token ändern'));
    expect(api.getToken()).toBe('');
    expect(await screen.findByPlaceholderText('API-Token')).toBeInTheDocument();
  });

  it('shows an error banner when loading fails', async () => {
    vi.spyOn(api, 'getHealth').mockRejectedValue(new Error('Health 502 from /health'));
    render(<Dashboard />);
    expect(await screen.findByText('Health 502 from /health')).toBeInTheDocument();
  });

  it('clears a previously connected status when polling fails', async () => {
    render(<Dashboard />);
    await screen.findByText('Verbunden');
    vi.spyOn(api, 'getHealth').mockRejectedValue(new Error('offline'));
    fireEvent.click(screen.getByTitle('Aktualisieren'));
    expect(await screen.findByText('Status unbekannt')).toBeInTheDocument();
    expect(screen.queryByText('Verbunden')).not.toBeInTheDocument();
    expect(screen.getByText('Veraltet')).toBeInTheDocument();
    expect(screen.queryByText('Kein Regen')).not.toBeInTheDocument();
  });

  it('keeps measured positions while showing accepted targets', async () => {
    const open = vi.spyOn(api, 'openNode').mockResolvedValue({ success: true, node_id: 1 });
    const close = vi.spyOn(api, 'closeNode').mockResolvedValue({ success: true, node_id: 1 });
    const setPos = vi.spyOn(api, 'setNodePosition').mockResolvedValue({ success: true, node_id: 1 });
    const stop = vi.spyOn(api, 'stopNode').mockRejectedValue(new Error('Failed to stop node'));

    render(<Dashboard />);
    await screen.findByText('Rollladen');

    fireEvent.click(screen.getByTitle('Vollständig schließen'));
    await waitFor(() => expect(close).toHaveBeenCalledWith(1));
    expect(await screen.findByText('Befehl angenommen: Ziel 100%')).toBeInTheDocument();
    expect(screen.getByText('Offen', { selector: 'p.text-lg' })).toBeInTheDocument();

    fireEvent.click(screen.getByTitle('Vollständig öffnen'));
    await waitFor(() => expect(open).toHaveBeenCalledWith(1));
    expect(await screen.findByText('Offen', { selector: 'p.text-lg' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '50%' }));
    await waitFor(() => expect(setPos).toHaveBeenCalledWith(1, 50));

    // Stop is disabled while not executing, so call it via a moving node
    vi.spyOn(api, 'getNodes').mockResolvedValue({ nodes: [{ ...node, state_str: 'Executing' }], count: 1 });
    fireEvent.click(screen.getByTitle('Aktualisieren'));
    await waitFor(() => expect(screen.getByTitle('Stoppen')).toBeEnabled());
    fireEvent.click(screen.getByTitle('Stoppen'));
    await waitFor(() => expect(stop).toHaveBeenCalledWith(1));
    expect(await screen.findByText('Failed to stop node')).toBeInTheDocument();
  });

  it('refreshes sensors and reconnects', async () => {
    const refresh = vi.spyOn(api, 'refreshSensorStatus').mockResolvedValue({ ...sensors, rain_detected: true });
    const reconnect = vi.spyOn(api, 'reconnectGateway').mockResolvedValue({ success: false, message: 'reconnect failed: boom' });

    render(<Dashboard />);
    await screen.findByText('Rollladen');

    fireEvent.click(screen.getByTitle('Sensorstatus aktualisieren'));
    await waitFor(() => expect(refresh).toHaveBeenCalled());
    expect(await screen.findByText('Erkannt')).toBeInTheDocument();

    fireEvent.click(screen.getByTitle('Neu verbinden'));
    await waitFor(() => expect(reconnect).toHaveBeenCalled());
    expect(await screen.findByText('reconnect failed: boom')).toBeInTheDocument();
  });

  it('switches to the Loxone guide tab', async () => {
    render(<Dashboard />);
    await screen.findByText('Rollladen');
    fireEvent.click(screen.getAllByRole('button', { name: /Loxone Anleitung/ })[0]);
    expect(await screen.findByText('Loxone Integration')).toBeInTheDocument();
  });

  it('polls periodically', async () => {
    vi.useFakeTimers();
    try {
      const getHealth = vi.spyOn(api, 'getHealth').mockResolvedValue(health);
      render(<Dashboard />);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(10_500);
      });
      expect(getHealth.mock.calls.length).toBeGreaterThanOrEqual(2);
    } finally {
      vi.useRealTimers();
    }
  });
});
