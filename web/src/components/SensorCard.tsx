import { SensorStatus } from '../types';
import { CloudRain, Wind, RefreshCw } from 'lucide-react';

interface SensorCardProps {
  sensors: SensorStatus | null;
  onRefresh: () => void;
  isRefreshing: boolean;
}

export function SensorCard({ sensors, onRefresh, isRefreshing }: SensorCardProps) {
  const known = sensors?.valid === true;
  const formatLastUpdate = (dateStr: string | undefined) => {
    if (!dateStr || dateStr === '0001-01-01T00:00:00Z') {
      return 'Noch nicht abgefragt';
    }
    const date = new Date(dateStr);
    return date.toLocaleTimeString('de-DE');
  };

  return (
    <div className="bg-gray-800 rounded-xl p-4">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-white">Sensoren</h2>
        <button
          onClick={onRefresh}
          disabled={isRefreshing}
          className="p-2 rounded-lg bg-gray-700 hover:bg-gray-600 text-gray-300 transition-colors disabled:opacity-50"
          title="Sensorstatus aktualisieren"
        >
          <RefreshCw size={16} className={isRefreshing ? 'animate-spin' : ''} />
        </button>
      </div>

      <div className="grid grid-cols-2 gap-4">
        {/* Rain Sensor */}
        <div className={`
          p-4 rounded-lg flex flex-col items-center gap-2 transition-colors
          ${known && sensors?.rain_detected ? 'bg-blue-600' : 'bg-gray-700'}
        `}>
          <CloudRain size={32} className="text-white" />
          <span className="text-white font-medium">Regen</span>
          <span className={`text-sm ${known && sensors?.rain_detected ? 'text-blue-200' : 'text-gray-400'}`}>
            {!known ? 'Unbekannt / veraltet' : sensors?.rain_detected ? 'Erkannt' : 'Kein Regen'}
          </span>
        </div>

        {/* Wind Sensor */}
        <div className={`
          p-4 rounded-lg flex flex-col items-center gap-2 transition-colors
          ${known && sensors?.wind_detected ? 'bg-amber-600' : 'bg-gray-700'}
        `}>
          <Wind size={32} className="text-white" />
          <span className="text-white font-medium">Wind</span>
          <span className={`text-sm ${known && sensors?.wind_detected ? 'text-amber-200' : 'text-gray-400'}`}>
            {!known ? 'Unbekannt / veraltet' : sensors?.wind_detected ? 'Erkannt' : 'Kein Wind'}
          </span>
        </div>
      </div>

      <p className="text-xs text-gray-500 mt-4 text-center">
        Zuletzt aktualisiert: {formatLastUpdate(sensors?.last_update)}
      </p>
    </div>
  );
}
