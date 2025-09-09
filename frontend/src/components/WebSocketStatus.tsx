import React, { useState, useEffect } from 'react';
import { Wifi, WifiOff, RotateCcw, AlertCircle } from 'lucide-react';
import { webSocketService } from '../services/websocket';
import type { ConnectionStatus } from '../types';

interface WebSocketStatusProps {
  className?: string;
  showDetails?: boolean;
}

const WebSocketStatus: React.FC<WebSocketStatusProps> = ({
  className = '',
  showDetails = false
}) => {
  const [status, setStatus] = useState<ConnectionStatus>(webSocketService.getStatus());
  const [queuedMessages, setQueuedMessages] = useState(0);

  useEffect(() => {
    // Subscribe to status changes
    const unsubscribe = webSocketService.onStatusChange(setStatus);

    // Update queued messages count periodically
    const updateQueue = () => {
      setQueuedMessages(webSocketService.getQueuedMessageCount());
    };

    const queueInterval = setInterval(updateQueue, 1000);
    updateQueue(); // Initial update

    return () => {
      unsubscribe();
      clearInterval(queueInterval);
    };
  }, []);

  const getStatusConfig = (currentStatus: ConnectionStatus) => {
    switch (currentStatus) {
      case 'connected':
        return {
          icon: Wifi,
          text: 'Connected',
          color: 'text-green-400',
          bgColor: 'bg-green-400/10',
          borderColor: 'border-green-400/20',
          pulseColor: 'bg-green-400',
        };
      case 'connecting':
        return {
          icon: RotateCcw,
          text: 'Connecting',
          color: 'text-blue-400',
          bgColor: 'bg-blue-400/10',
          borderColor: 'border-blue-400/20',
          pulseColor: 'bg-blue-400',
          animate: true,
        };
      case 'reconnecting':
        return {
          icon: RotateCcw,
          text: 'Reconnecting',
          color: 'text-yellow-400',
          bgColor: 'bg-yellow-400/10',
          borderColor: 'border-yellow-400/20',
          pulseColor: 'bg-yellow-400',
          animate: true,
        };
      case 'error':
        return {
          icon: AlertCircle,
          text: 'Error',
          color: 'text-red-400',
          bgColor: 'bg-red-400/10',
          borderColor: 'border-red-400/20',
          pulseColor: 'bg-red-400',
        };
      case 'disconnected':
      default:
        return {
          icon: WifiOff,
          text: 'Disconnected',
          color: 'text-gray-400',
          bgColor: 'bg-gray-400/10',
          borderColor: 'border-gray-400/20',
          pulseColor: 'bg-gray-400',
        };
    }
  };

  const statusConfig = getStatusConfig(status);
  const Icon = statusConfig.icon;

  const handleClick = () => {
    if (status === 'disconnected' || status === 'error') {
      webSocketService.connect().catch((_error) => {
        // Handle connection error - could be logged to error service in production
        // TODO: Log error to proper logging service
        // Error handling could be improved with proper error tracking
      });
    }
  };

  const handleClearQueue = () => {
    webSocketService.clearMessageQueue();
    setQueuedMessages(0);
  };

  return (
    <div className={`inline-flex items-center gap-2 ${className}`}>
      {/* Main Status Indicator */}
      <button
        onClick={handleClick}
        disabled={status === 'connecting' || status === 'reconnecting'}
        className={`
          relative inline-flex items-center gap-2 px-3 py-1.5 rounded-full text-sm font-medium
          ${statusConfig.bgColor} ${statusConfig.borderColor} border
          ${status === 'disconnected' || status === 'error'
            ? 'hover:bg-opacity-20 cursor-pointer'
            : 'cursor-default'
          }
          transition-all duration-200 disabled:cursor-not-allowed
        `}
        title={
          status === 'disconnected' || status === 'error'
            ? 'Click to reconnect'
            : `WebSocket ${statusConfig.text.toLowerCase()}`
        }
      >
        {/* Pulse animation for active states */}
        {(status === 'connected' || statusConfig.animate) && (
          <div className="absolute inset-0 rounded-full">
            <div
              className={`
                absolute inset-0 rounded-full ${statusConfig.pulseColor} opacity-20
                ${status === 'connected'
                  ? 'animate-pulse-slow'
                  : 'animate-ping'
                }
              `}
            />
          </div>
        )}

        <Icon
          className={`
            w-4 h-4 relative z-10 ${statusConfig.color}
            ${statusConfig.animate ? 'animate-spin' : ''}
          `}
        />
        <span className={`relative z-10 ${statusConfig.color}`}>
          {statusConfig.text}
        </span>

        {/* Connection indicator dot */}
        <div
          className={`
            relative z-10 w-2 h-2 rounded-full ${statusConfig.pulseColor}
            ${status === 'connected' ? 'animate-pulse' : ''}
          `}
        />
      </button>

      {/* Details Panel */}
      {showDetails && (
        <div className="flex items-center gap-2 text-xs text-gray-400">
          {queuedMessages > 0 && (
            <button
              onClick={handleClearQueue}
              className="flex items-center gap-1 px-2 py-1 bg-yellow-400/10 border border-yellow-400/20 rounded hover:bg-yellow-400/20 transition-colors"
              title="Clear queued messages"
            >
              <span className="text-yellow-400">Queue: {queuedMessages}</span>
            </button>
          )}

          {status === 'error' && (
            <span className="px-2 py-1 bg-red-400/10 border border-red-400/20 rounded text-red-400">
              Connection failed
            </span>
          )}

          {status === 'reconnecting' && (
            <span className="px-2 py-1 bg-yellow-400/10 border border-yellow-400/20 rounded text-yellow-400">
              Retrying...
            </span>
          )}
        </div>
      )}
    </div>
  );
};

export default WebSocketStatus;
