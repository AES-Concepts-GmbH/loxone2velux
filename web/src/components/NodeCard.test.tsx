import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { NodeCard } from './NodeCard';
import { Node } from '../types';

function makeNode(overrides: Partial<Node> = {}): Node {
  return {
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
    ...overrides,
  };
}

function renderCard(node: Node) {
  const handlers = {
    onSetPosition: vi.fn(),
    onOpen: vi.fn(),
    onClose: vi.fn(),
    onStop: vi.fn(),
  };
  const utils = render(<NodeCard node={node} {...handlers} />);
  return { ...utils, handlers };
}

describe('NodeCard', () => {
  it('shows name, id, type and "Offen" for position 0', () => {
    renderCard(makeNode());
    expect(screen.getByText('Rollladen')).toBeInTheDocument();
    expect(screen.getByText('#1')).toBeInTheDocument();
    expect(screen.getByText('Roller Shutter')).toBeInTheDocument();
    expect(screen.getByText('Offen', { selector: 'p.text-lg' })).toBeInTheDocument();
  });

  it('shows "Geschlossen" for 100% and the percentage in between', () => {
    const { unmount } = renderCard(makeNode({ position_percent: 100 }));
    expect(screen.getByText('Geschlossen', { selector: 'p.text-lg' })).toBeInTheDocument();
    unmount();
    renderCard(makeNode({ position_percent: 50 }));
    expect(screen.getAllByText('50%').length).toBeGreaterThan(0);
  });

  it('invokes open, close and preset handlers with Velux positions', () => {
    const { handlers } = renderCard(makeNode({ position_percent: 50 }));
    fireEvent.click(screen.getByTitle('Vollständig öffnen'));
    expect(handlers.onOpen).toHaveBeenCalledWith(1);
    fireEvent.click(screen.getByTitle('Vollständig schließen'));
    expect(handlers.onClose).toHaveBeenCalledWith(1);
    // "25% offen" on a non-inverted device means Velux position 75
    fireEvent.click(screen.getByRole('button', { name: '25%' }));
    expect(handlers.onSetPosition).toHaveBeenCalledWith(1, 75);
  });

  it('sends preset positions as-is for inverted devices', () => {
    const { handlers } = renderCard(makeNode({ inverted: true }));
    fireEvent.click(screen.getByRole('button', { name: '75%' }));
    expect(handlers.onSetPosition).toHaveBeenCalledWith(1, 75);
  });

  it('commits the slider value on mouse up', () => {
    const { handlers } = renderCard(makeNode());
    const slider = screen.getByRole('slider');
    fireEvent.change(slider, { target: { value: '60' } });
    expect(screen.getByText('60%')).toBeInTheDocument();
    fireEvent.mouseUp(slider);
    expect(handlers.onSetPosition).toHaveBeenCalledWith(1, 60);
  });

  it('disables movement controls and enables stop while executing', () => {
    const { handlers } = renderCard(makeNode({ state_str: 'Executing', position_percent: 30 }));
    expect(screen.getByText('Bewegt sich...')).toBeInTheDocument();
    expect(screen.getByTitle('Vollständig öffnen')).toBeDisabled();
    expect(screen.getByRole('slider')).toBeDisabled();
    const stop = screen.getByTitle('Stoppen');
    expect(stop).toBeEnabled();
    fireEvent.click(stop);
    expect(handlers.onStop).toHaveBeenCalledWith(1);
  });

  it('syncs the slider with the gateway position once the node stops moving', () => {
    const { rerender } = renderCard(makeNode({ position_percent: 10 }));
    const slider = screen.getByRole('slider') as HTMLInputElement;
    expect(slider.value).toBe('10');

    // While executing, the slider keeps its value
    rerender(
      <NodeCard node={makeNode({ position_percent: 80, state_str: 'Executing' })}
        onSetPosition={vi.fn()} onOpen={vi.fn()} onClose={vi.fn()} onStop={vi.fn()} />
    );
    expect(slider.value).toBe('10');

    // Once done, it follows the gateway
    rerender(
      <NodeCard node={makeNode({ position_percent: 80, state_str: 'Done' })}
        onSetPosition={vi.fn()} onOpen={vi.fn()} onClose={vi.fn()} onStop={vi.fn()} />
    );
    expect(slider.value).toBe('80');
  });
});
