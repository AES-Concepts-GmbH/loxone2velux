import { describe, it, expect } from 'vitest';
import { getNodeTypeIcon, isWindowType, isShutterType } from './index';

describe('node type helpers', () => {
  it('maps known types to icons and unknown types to the default', () => {
    expect(getNodeTypeIcon('Window Opener')).toBe('window');
    expect(getNodeTypeIcon('Roller Shutter')).toBe('shutter');
    expect(getNodeTypeIcon('Dual Shutter')).toBe('shutter');
    expect(getNodeTypeIcon('Exterior Venetian Blind')).toBe('blind');
    expect(getNodeTypeIcon('Horizontal Awning')).toBe('awning');
    expect(getNodeTypeIcon('Curtain Track')).toBe('curtain');
    expect(getNodeTypeIcon('Ventilation Point')).toBe('ventilation');
    expect(getNodeTypeIcon('Light')).toBe('light');
    expect(getNodeTypeIcon('Garage Opener')).toBe('garage');
    expect(getNodeTypeIcon('Unknown (0x7FC0)')).toBe('device');
  });

  it('classifies windows and shutters', () => {
    expect(isWindowType('Window Opener')).toBe(true);
    expect(isWindowType('Roller Shutter')).toBe(false);
    expect(isShutterType('Roller Shutter')).toBe(true);
    expect(isShutterType('Louver Blind')).toBe(true);
    expect(isShutterType('Light')).toBe(false);
  });
});
