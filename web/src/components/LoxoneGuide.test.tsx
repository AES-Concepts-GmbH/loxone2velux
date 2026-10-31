import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { LoxoneGuide } from './LoxoneGuide';
import { setToken } from '../services/api';

describe('LoxoneGuide', () => {
  it('encodes reserved token characters and allows a direct gateway address', () => {
    setToken('secret&?#= token');
    render(<LoxoneGuide />);
    const input = screen.getByRole('textbox', { name: 'Direkte Gateway-Adresse' });
    fireEvent.change(input, { target: { value: 'http://192.168.1.20:8099' } });
    expect(localStorage.getItem('gateway-direct-url')).toBe('http://192.168.1.20:8099');
    fireEvent.click(screen.getByRole('button', { name: '2. Virtual Output Commands' }));
    expect(screen.getByText('/loxone/node/<ID>/open?token=secret%26%3F%23%3D%20token')).toBeInTheDocument();
    setToken('');
  });
});
