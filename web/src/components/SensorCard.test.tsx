import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { SensorCard } from './SensorCard';

describe('SensorCard validity', () => {
  it('does not report clear weather before a complete refresh', () => {
    render(<SensorCard sensors={null} onRefresh={() => {}} isRefreshing={false} />);
    expect(screen.queryByText('Kein Regen')).not.toBeInTheDocument();
    expect(screen.queryByText('Kein Wind')).not.toBeInTheDocument();
    expect(screen.getAllByText('Unbekannt / veraltet')).toHaveLength(2);
  });
  it('does not present stale values as current readings', () => {
    render(<SensorCard sensors={{ valid: false, rain_detected: false, wind_detected: false, last_update: new Date().toISOString() }} onRefresh={() => {}} isRefreshing={false} />);
    expect(screen.getAllByText('Unbekannt / veraltet')).toHaveLength(2);
    expect(screen.queryByText('Kein Regen')).not.toBeInTheDocument();
  });
});
