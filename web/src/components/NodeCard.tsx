import { useEffect, useState } from 'react';
import { Node } from '../types';
import {
  Blinds,
  Square,
  Maximize2,
  Minimize2,
} from 'lucide-react';

interface NodeCardProps {
  node: Node;
  onSetPosition: (id: number, position: number) => void;
  onOpen: (id: number) => void;
  onClose: (id: number) => void;
  onStop: (id: number) => void;
}

export function NodeCard({
  node,
  onSetPosition,
  onOpen,
  onClose,
  onStop,
}: NodeCardProps) {
  const [localPosition, setLocalPosition] = useState(node.position_percent);

  // Sync the slider with the gateway position once the node is no longer moving.
  useEffect(() => {
    if (node.state_str !== 'Executing') {
      setLocalPosition((current) =>
        Math.abs(current - node.position_percent) > 1 ? node.position_percent : current
      );
    }
  }, [node.position_percent, node.state_str]);

  const handlePositionChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setLocalPosition(parseFloat(e.target.value));
  };

  const handlePositionCommit = () => {
    onSetPosition(node.id, localPosition);
  };

  const isExecuting = node.state_str === 'Executing';
  // For inverted types (window openers): 0% = closed, 100% = open
  // For normal types (shutters): 0% = open, 100% = closed
  const isOpen = node.inverted
    ? node.position_percent > 90
    : node.position_percent < 10;
  const isClosed = node.inverted
    ? node.position_percent < 10
    : node.position_percent > 90;

  // Get icon based on type
  const getIcon = () => {
    return Blinds;
  };

  const Icon = getIcon();

  // Get position display
  const getPositionText = () => {
    if (isOpen) return 'Offen';
    if (isClosed) return 'Geschlossen';
    return `${Math.round(node.position_percent)}%`;
  };

  // Get status color
  const getStatusColor = () => {
    if (isExecuting) return 'text-yellow-500';
    if (node.state_str === 'Error') return 'text-red-500';
    return 'text-green-500';
  };

  return (
    <div className={`
      bg-gray-800 rounded-xl p-4 transition-all duration-200
      ${isExecuting ? 'ring-2 ring-yellow-500' : ''}
    `}>
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <div className={`
            w-10 h-10 rounded-full flex items-center justify-center transition-colors
            ${isOpen ? 'bg-velux-blue' : isClosed ? 'bg-gray-600' : 'bg-velux-blue/60'}
          `}>
            <Icon size={20} className="text-white" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="font-medium text-white">{node.name}</h3>
              <span className="px-1.5 py-0.5 bg-gray-700 rounded text-xs text-gray-400 font-mono">#{node.id}</span>
            </div>
            <p className="text-xs text-gray-400">{node.node_type_str}</p>
          </div>
        </div>

        <div className="text-right">
          <p className="text-lg font-semibold text-white">{getPositionText()}</p>
          <p className={`text-xs ${getStatusColor()}`}>
            {isExecuting ? 'Bewegt sich...' : node.state_str}
          </p>
        </div>
      </div>

      {/* Position Slider */}
      <div className="mb-4">
        <div className="flex justify-between text-xs text-gray-400 mb-2">
          <span>Position</span>
          <span>{Math.round(localPosition)}%</span>
        </div>
        <div className="relative">
          <input
            type="range"
            min="0"
            max="100"
            value={localPosition}
            onChange={handlePositionChange}
            onMouseUp={handlePositionCommit}
            onTouchEnd={handlePositionCommit}
            className="w-full"
            disabled={isExecuting}
          />
          {/* Visual indicator - labels depend on device type */}
          <div className="flex justify-between text-xs text-gray-500 mt-1">
            <span>{node.inverted ? 'Geschlossen' : 'Offen'}</span>
            <span>{node.inverted ? 'Offen' : 'Geschlossen'}</span>
          </div>
        </div>

        {/* Preset Position Buttons - percentage means "how open" */}
        <div className="flex gap-2 mt-3">
          {[25, 50, 75].map((openPercent) => {
            // Convert "percentage open" to Velux position
            // For inverted (window openers): 0=closed, 100=open → send as-is
            // For non-inverted (shutters): 0=open, 100=closed → invert
            const veluxPosition = node.inverted ? openPercent : 100 - openPercent;
            // Current open percentage for highlighting
            const currentOpenPercent = node.inverted ? node.position_percent : 100 - node.position_percent;

            return (
              <button
                key={openPercent}
                onClick={() => onSetPosition(node.id, veluxPosition)}
                disabled={isExecuting}
                className={`
                  flex-1 py-1.5 text-sm rounded-lg transition-colors
                  ${Math.abs(currentOpenPercent - openPercent) < 5
                    ? 'bg-velux-blue text-white'
                    : 'bg-gray-700 hover:bg-gray-600 text-gray-300'
                  }
                  disabled:opacity-50
                `}
              >
                {openPercent}%
              </button>
            );
          })}
        </div>
      </div>

      {/* Quick Actions */}
      <div className="flex gap-2">
        <button
          onClick={() => onOpen(node.id)}
          disabled={isExecuting}
          className="flex-1 flex items-center justify-center gap-1 px-3 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors disabled:opacity-50"
          title="Vollständig öffnen"
        >
          <Maximize2 size={16} />
          <span className="text-sm">Öffnen</span>
        </button>

        <button
          onClick={() => onStop(node.id)}
          disabled={!isExecuting}
          className={`
            flex items-center justify-center px-3 py-2 rounded-lg transition-colors
            ${isExecuting
              ? 'bg-yellow-600 hover:bg-yellow-500 text-white'
              : 'bg-gray-700 text-gray-500 cursor-not-allowed'
            }
          `}
          title="Stoppen"
        >
          <Square size={16} />
        </button>

        <button
          onClick={() => onClose(node.id)}
          disabled={isExecuting}
          className="flex-1 flex items-center justify-center gap-1 px-3 py-2 bg-gray-700 hover:bg-gray-600 text-white rounded-lg transition-colors disabled:opacity-50"
          title="Vollständig schließen"
        >
          <Minimize2 size={16} />
          <span className="text-sm">Schließen</span>
        </button>
      </div>

      {/* Loxone API Info */}
      <div className="mt-4 pt-4 border-t border-gray-700">
        <p className="text-xs text-gray-500 font-mono truncate">
          /loxone/node/{node.id}/set/&#123;0-100&#125;
        </p>
      </div>
    </div>
  );
}
