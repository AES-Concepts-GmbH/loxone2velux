// Node types from KLF-200
export type NodeType =
  | 'Interior Venetian Blind'
  | 'Roller Shutter'
  | 'Awning Blind'
  | 'Window Opener'
  | 'Garage Opener'
  | 'Light'
  | 'Gate Lock'
  | 'Window Lock'
  | 'Vertical Exterior Awning'
  | 'Dual Shutter'
  | 'Heating Control'
  | 'On/Off Switch'
  | 'Horizontal Awning'
  | 'Exterior Venetian Blind'
  | 'Louver Blind'
  | 'Curtain Track'
  | 'Ventilation Point'
  | 'Exterior Heating'
  | 'Swinging Shutter'
  | string;

// Node state
export type NodeState =
  | 'Non-Executing'
  | 'Error'
  | 'Not Used'
  | 'Waiting for Power'
  | 'Executing'
  | 'Done'
  | 'Unknown';

// Velux Node/Device
export interface Node {
  id: number;
  name: string;
  node_type: number;
  node_type_str: NodeType;
  state: number;
  state_str: NodeState;
  current_position_raw: number;
  position_percent: number;
  target_position_raw: number;
  target_percent: number;
  velocity: number;
  last_update: string;
  inverted: boolean; // true for window openers (0%=closed, 100%=open)
}

// API Response types
export interface NodesResponse {
  nodes: Node[];
  count: number;
}

export interface NodeResponse {
  id: number;
  name: string;
  node_type: number;
  node_type_str: NodeType;
  state: number;
  state_str: NodeState;
  current_position_raw: number;
  position_percent: number;
  target_position_raw: number;
  target_percent: number;
  velocity: number;
  last_update: string;
}

export interface HealthResponse {
  status: 'ok' | 'degraded';
  connected: boolean;
  node_count: number;
  version: string;
}

export interface CommandResponse {
  success: boolean;
  message?: string;
  node_id: number;
}

export interface ErrorResponse {
  error: string;
  code: number;
  details?: string;
}

export interface PositionRequest {
  position: number;
}

// Sensor status
export interface SensorStatus {
  rain_detected: boolean;
  wind_detected: boolean;
  last_update: string;
}

// Node-to-Loxone mapping
export interface NodeMapping {
  id: string;
  name: string;
  node_id: number;
  loxone_id: string;
  enabled: boolean;
}

export interface UDPFeedbackConfig {
  enabled: boolean;
  ip: string;
  port: number;
}

// Configuration as returned by GET /api/config (secrets are never returned)
export interface GatewayConfig {
  klf200: {
    host: string;
    port: number;
    password_set: boolean;
    reconnect_interval: string;
    refresh_interval: string;
  };
  server: {
    host: string;
    port: number;
    api_token_set: boolean;
  };
  loxone: {
    udp_feedback: UDPFeedbackConfig;
    mappings: NodeMapping[];
  };
  logging: {
    level: string;
    format: string;
  };
}

// Partial update for POST /api/config; omitted fields stay unchanged
export interface GatewayConfigUpdate {
  klf200?: {
    host?: string;
    port?: number;
    password?: string;
  };
  server?: {
    api_token?: string;
  };
  logging?: {
    level?: string;
  };
}

// Helper to get icon for node type
export function getNodeTypeIcon(type: NodeType): string {
  switch (type) {
    case 'Window Opener':
      return 'window';
    case 'Roller Shutter':
    case 'Dual Shutter':
    case 'Swinging Shutter':
      return 'shutter';
    case 'Interior Venetian Blind':
    case 'Exterior Venetian Blind':
    case 'Louver Blind':
      return 'blind';
    case 'Awning Blind':
    case 'Horizontal Awning':
    case 'Vertical Exterior Awning':
      return 'awning';
    case 'Curtain Track':
      return 'curtain';
    case 'Ventilation Point':
      return 'ventilation';
    case 'Light':
      return 'light';
    case 'Garage Opener':
    case 'Gate Lock':
      return 'garage';
    default:
      return 'device';
  }
}

// Helper to check if node is a window type
export function isWindowType(type: NodeType): boolean {
  return type === 'Window Opener';
}

// Helper to check if node is a shutter/blind type
export function isShutterType(type: NodeType): boolean {
  return [
    'Roller Shutter',
    'Interior Venetian Blind',
    'Exterior Venetian Blind',
    'Awning Blind',
    'Louver Blind',
    'Dual Shutter',
    'Swinging Shutter',
    'Curtain Track',
    'Horizontal Awning',
    'Vertical Exterior Awning',
  ].includes(type);
}
