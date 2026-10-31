import { Node } from '../types';
import { NodeCard } from './NodeCard';
import { Blinds, RefreshCw } from 'lucide-react';

interface NodeListProps {
  nodes: Node[];
  targets?: Record<number, number>;
  loading: boolean;
  onSetPosition: (id: number, position: number) => void;
  onOpen: (id: number) => void;
  onClose: (id: number) => void;
  onStop: (id: number) => void;
  onRefresh: () => void;
}

export function NodeList({
  nodes,
  targets = {},
  loading,
  onSetPosition,
  onOpen,
  onClose,
  onStop,
  onRefresh,
}: NodeListProps) {
  if (loading && nodes.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16">
        <RefreshCw size={48} className="text-velux-blue animate-spin mb-4" />
        <p className="text-gray-400">Geräte werden geladen...</p>
      </div>
    );
  }

  if (nodes.length === 0) {
    return (
      <div className="bg-gray-800 rounded-xl p-8 text-center">
        <div className="w-16 h-16 bg-gray-700 rounded-full flex items-center justify-center mx-auto mb-4">
          <Blinds size={32} className="text-gray-500" />
        </div>
        <h3 className="text-lg font-medium text-white mb-2">Keine Geräte gefunden</h3>
        <p className="text-gray-400 mb-4">
          Stelle sicher, dass der KLF-200 Gateway verbunden ist und Geräte angelernt wurden.
        </p>
        <button
          onClick={onRefresh}
          className="px-4 py-2 bg-velux-blue hover:bg-velux-dark text-white rounded-lg transition-colors"
        >
          Erneut versuchen
        </button>
      </div>
    );
  }

  // Group nodes by type
  const windowNodes = nodes.filter(n => n.node_type_str === 'Window Opener');
  const shutterNodes = nodes.filter(n =>
    ['Roller Shutter', 'Dual Shutter', 'Swinging Shutter'].includes(n.node_type_str)
  );
  const blindNodes = nodes.filter(n =>
    ['Interior Venetian Blind', 'Exterior Venetian Blind', 'Louver Blind'].includes(n.node_type_str)
  );
  const otherNodes = nodes.filter(n =>
    !['Window Opener', 'Roller Shutter', 'Dual Shutter', 'Swinging Shutter',
      'Interior Venetian Blind', 'Exterior Venetian Blind', 'Louver Blind'].includes(n.node_type_str)
  );

  const renderSection = (title: string, sectionNodes: Node[]) => {
    if (sectionNodes.length === 0) return null;

    return (
      <section className="mb-8">
        <h2 className="text-lg font-semibold text-white mb-4">{title}</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {sectionNodes.map((node) => (
            <NodeCard
              key={node.id}
              node={node}
              acceptedTarget={targets[node.id]}
              onSetPosition={onSetPosition}
              onOpen={onOpen}
              onClose={onClose}
              onStop={onStop}
            />
          ))}
        </div>
      </section>
    );
  };

  return (
    <div>
      {renderSection('Fenster', windowNodes)}
      {renderSection('Rollläden', shutterNodes)}
      {renderSection('Jalousien', blindNodes)}
      {renderSection('Weitere Geräte', otherNodes)}
    </div>
  );
}
