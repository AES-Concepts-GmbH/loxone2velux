import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as api from './api';

function jsonResponse(body: unknown, status = 200, contentType = 'application/json'): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': contentType },
  });
}

describe('token storage', () => {
  it('starts empty, stores and clears the token', () => {
    expect(api.getToken()).toBe('');
    api.setToken('abc');
    expect(api.getToken()).toBe('abc');
    expect(localStorage.getItem('api_token')).toBe('abc');
    api.setToken('');
    expect(api.getToken()).toBe('');
    expect(localStorage.getItem('api_token')).toBeNull();
  });
});

describe('fetchJSON via getNodes', () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal('fetch', fetchMock);
    window.history.replaceState({}, '', '/');
  });

  it('builds the URL relative to the page and sends the bearer token', async () => {
    window.history.replaceState({}, '', '/api/hassio_ingress/xyz//');
    api.setToken('secret-token');
    fetchMock.mockResolvedValue(jsonResponse({ nodes: [], count: 0 }));

    const result = await api.getNodes();

    expect(result.count).toBe(0);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/hassio_ingress/xyz/api/nodes');
    expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer secret-token');
  });

  it('sends no Authorization header without a token', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ nodes: [], count: 0 }));
    await api.getNodes();
    const [, init] = fetchMock.mock.calls[0];
    expect((init?.headers as Record<string, string>).Authorization).toBeUndefined();
  });

  it('throws AuthError on 401', async () => {
    fetchMock.mockResolvedValue(new Response('Unauthorized', { status: 401 }));
    await expect(api.getNodes()).rejects.toBeInstanceOf(api.AuthError);
  });

  it('uses the error and details fields of an error response', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ error: 'Failed to open node', details: 'not connected' }, 503));
    await expect(api.openNode(1)).rejects.toThrow('Failed to open node: not connected');
  });

  it('falls back to the status text when the error body is not JSON', async () => {
    fetchMock.mockResolvedValue(new Response('boom', { status: 500, statusText: 'Internal Server Error' }));
    await expect(api.closeNode(1)).rejects.toThrow('Internal Server Error');
  });

  it('rejects HTML responses (wrong proxy route)', async () => {
    fetchMock.mockResolvedValue(new Response('<html></html>', { status: 200, headers: { 'content-type': 'text/html' } }));
    await expect(api.getSensorStatus()).rejects.toThrow(/Expected JSON/);
  });

  it('posts the position body', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ success: true, node_id: 3 }));
    const res = await api.setNodePosition(3, 42);
    expect(res.success).toBe(true);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/nodes/3/position');
    expect(init?.method).toBe('POST');
    expect(JSON.parse(init?.body as string)).toEqual({ position: 42 });
  });

  it('calls the remaining endpoints with the right method and path', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(jsonResponse({})));
    await api.stopNode(2);
    await api.refreshSensorStatus();
    await api.reconnectGateway();
    await api.getNode(5);
    await api.getConfig();
    await api.updateConfig({ logging: { level: 'debug' } });
    const calls = fetchMock.mock.calls.map(([url, init]) => `${init?.method ?? 'GET'} ${url}`);
    expect(calls).toEqual([
      'POST /api/nodes/2/stop',
      'POST /api/sensors/refresh',
      'POST /api/reconnect',
      'GET /api/nodes/5',
      'GET /api/config',
      'POST /api/config',
    ]);
  });
});

describe('getHealth and isAuthRequired', () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal('fetch', fetchMock);
    window.history.replaceState({}, '', '/');
  });

  it('returns the health payload', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ status: 'ok', connected: true, node_count: 2, version: '1.3.0' }));
    const h = await api.getHealth();
    expect(h.connected).toBe(true);
    expect(fetchMock.mock.calls[0][0]).toBe('/health');
  });

  it('fails on non-200 and on non-JSON health responses', async () => {
    fetchMock.mockResolvedValueOnce(new Response('', { status: 502 }));
    await expect(api.getHealth()).rejects.toThrow(/Health 502/);
    fetchMock.mockResolvedValueOnce(new Response('<html>', { status: 200, headers: { 'content-type': 'text/html' } }));
    await expect(api.getHealth()).rejects.toThrow(/Health: got text\/html/);
  });

  it('reports whether auth is required', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ required: true }));
    expect(await api.isAuthRequired()).toBe(true);
    fetchMock.mockResolvedValueOnce(new Response('', { status: 500 }));
    expect(await api.isAuthRequired()).toBe(false);
  });
});
