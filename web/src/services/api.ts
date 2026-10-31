import {
  Node,
  NodesResponse,
  HealthResponse,
  CommandResponse,
  PositionRequest,
  SensorStatus,
  GatewayConfig,
  GatewayConfigUpdate,
} from '../types';

const TOKEN_STORAGE_KEY = 'api_token';
const REQUEST_TIMEOUT_MS = 15000;

// Build the base path for API requests from the current page URL.
// HA Ingress can produce double slashes (ingress_entry: / appended to
// the proxy base path); we collapse them here.
function getBasePath(): string {
  let path = window.location.pathname.replace(/\/\/+/g, '/');
  if (!path.endsWith('/')) path += '/';
  return path;
}

// --- API token handling -----------------------------------------------------

/** Returns the API token stored in this browser, or '' if none is set. */
export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_STORAGE_KEY) || '';
  } catch {
    return '';
  }
}

/** Stores (or with an empty value removes) the API token in this browser. */
export function setToken(token: string): void {
  try {
    if (token) {
      localStorage.setItem(TOKEN_STORAGE_KEY, token);
    } else {
      localStorage.removeItem(TOKEN_STORAGE_KEY);
    }
  } catch {
    // localStorage unavailable (private mode etc.) - token is session-only
  }
}

/** Thrown when the gateway rejects the request because of a missing/invalid token. */
export class AuthError extends Error {
  constructor(message = 'Authentifizierung erforderlich') {
    super(message);
    this.name = 'AuthError';
  }
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

// Base fetch helper
async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(getBasePath() + url, {
    ...options,
    signal: options?.signal ?? AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders(),
      ...options?.headers,
    },
  });

  if (response.status === 401) {
    throw new AuthError();
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: response.statusText }));
    throw new Error(error.details ? `${error.error}: ${error.details}` : error.error || 'API request failed');
  }

  // Detect HTML response (e.g. SPA fallback or wrong proxy route)
  const ct = response.headers.get('content-type') || '';
  if (!ct.includes('json')) {
    throw new Error(`Expected JSON from ${response.url} but got ${ct}`);
  }

  return response.json();
}

// Health check (no auth required)
export async function getHealth(): Promise<HealthResponse> {
  const response = await fetch(getBasePath() + 'health', { signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) });
  if (!response.ok) {
    throw new Error(`Health ${response.status} from ${response.url}`);
  }
  const ct = response.headers.get('content-type') || '';
  if (!ct.includes('json')) {
    throw new Error(`Health: got ${ct} from ${response.url}`);
  }
  return response.json();
}

// Whether the gateway requires an API token (no auth required for this call)
export async function isAuthRequired(): Promise<boolean> {
  const response = await fetch(getBasePath() + 'api/auth/status', { signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) });
  if (!response.ok) return false;
  const data = (await response.json()) as { required: boolean };
  return data.required;
}

// Get all nodes
export async function getNodes(): Promise<NodesResponse> {
  return fetchJSON<NodesResponse>('api/nodes');
}

// Get single node
export async function getNode(id: number): Promise<Node> {
  return fetchJSON<Node>(`api/nodes/${id}`);
}

// Set node position (0-100%)
export async function setNodePosition(id: number, position: number): Promise<CommandResponse> {
  const body: PositionRequest = { position };
  return fetchJSON<CommandResponse>(`api/nodes/${id}/position`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

// Open node fully
export async function openNode(id: number): Promise<CommandResponse> {
  return fetchJSON<CommandResponse>(`api/nodes/${id}/open`, {
    method: 'POST',
  });
}

// Close node fully
export async function closeNode(id: number): Promise<CommandResponse> {
  return fetchJSON<CommandResponse>(`api/nodes/${id}/close`, {
    method: 'POST',
  });
}

// Stop node movement
export async function stopNode(id: number): Promise<CommandResponse> {
  return fetchJSON<CommandResponse>(`api/nodes/${id}/stop`, {
    method: 'POST',
  });
}

// Get sensor status (rain, wind, etc.)
export async function getSensorStatus(): Promise<SensorStatus> {
  return fetchJSON<SensorStatus>('api/sensors');
}

// Refresh sensor status from KLF-200
export async function refreshSensorStatus(): Promise<SensorStatus> {
  return fetchJSON<SensorStatus>('api/sensors/refresh', {
    method: 'POST',
  });
}

// Configuration API

// Get current configuration (secrets are not returned)
export async function getConfig(): Promise<GatewayConfig> {
  return fetchJSON<GatewayConfig>('api/config');
}

// Update configuration (omitted fields stay unchanged)
export async function updateConfig(config: GatewayConfigUpdate): Promise<GatewayConfig> {
  return fetchJSON<GatewayConfig>('api/config', {
    method: 'POST',
    body: JSON.stringify(config),
  });
}

// Reconnect to KLF-200
export async function reconnectGateway(): Promise<{ success: boolean; message: string }> {
  return fetchJSON<{ success: boolean; message: string }>('api/reconnect', {
    method: 'POST',
  });
}
