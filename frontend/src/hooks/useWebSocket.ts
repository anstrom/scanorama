import { useState, useEffect, useCallback, useRef } from 'react';
import { webSocketService } from '../services/websocket';
import type { ConnectionStatus, WebSocketMessage, MessageHandler } from '../types';

export interface UseWebSocketOptions {
  autoConnect?: boolean;
  reconnectOnMount?: boolean;
  messageTypes?: string[];
}

export interface UseWebSocketReturn {
  status: ConnectionStatus;
  isConnected: boolean;
  isConnecting: boolean;
  hasError: boolean;
  queuedMessages: number;
  lastMessage: WebSocketMessage | null;
  lastError: Error | null;
  connect: () => Promise<void>;
  disconnect: () => void;
  sendMessage: (type: string, payload: unknown) => boolean;
  subscribe: (messageType: string, handler: MessageHandler) => () => void;
  clearQueue: () => void;
}

export function useWebSocket(options: UseWebSocketOptions = {}): UseWebSocketReturn {
  const {
    autoConnect = true,
    reconnectOnMount = true,
    messageTypes = ['*']
  } = options;

  const [status, setStatus] = useState<ConnectionStatus>(webSocketService.getStatus());
  const [queuedMessages, setQueuedMessages] = useState(0);
  const [lastMessage, setLastMessage] = useState<WebSocketMessage | null>(null);
  const [lastError, setLastError] = useState<Error | null>(null);

  const handlersRef = useRef<Map<string, MessageHandler>>(new Map());
  const unsubscribersRef = useRef<(() => void)[]>([]);

  // Connection status handler
  const handleStatusChange = useCallback((newStatus: ConnectionStatus) => {
    setStatus(newStatus);

    // Clear error when connection is successful
    if (newStatus === 'connected') {
      setLastError(null);
    }
  }, []);

  // Generic message handler
  const handleMessage = useCallback((message: WebSocketMessage) => {
    setLastMessage(message);

    // Update last seen time for activity tracking
    if (message.timestamp) {
      // Could emit custom events or update global state here
    }
  }, []);

  // Connect function
  const connect = useCallback(async (): Promise<void> => {
    try {
      await webSocketService.connect();
      setLastError(null);
    } catch (error) {
      const err = error instanceof Error ? error : new Error('Connection failed');
      setLastError(err);
      throw err;
    }
  }, []);

  // Disconnect function
  const disconnect = useCallback((): void => {
    webSocketService.disconnect();
    setLastError(null);
  }, []);

  // Send message function
  const sendMessage = useCallback((type: string, payload: unknown): boolean => {
    return webSocketService.send(type, payload);
  }, []);

  // Subscribe to specific message type
  const subscribe = useCallback((messageType: string, handler: MessageHandler): (() => void) => {
    const unsubscribe = webSocketService.on(messageType, handler);
    unsubscribersRef.current.push(unsubscribe);
    handlersRef.current.set(messageType, handler);

    // Return unsubscribe function
    return () => {
      unsubscribe();
      handlersRef.current.delete(messageType);
      const index = unsubscribersRef.current.indexOf(unsubscribe);
      if (index > -1) {
        unsubscribersRef.current.splice(index, 1);
      }
    };
  }, []);

  // Clear message queue
  const clearQueue = useCallback((): void => {
    webSocketService.clearMessageQueue();
    setQueuedMessages(0);
  }, []);

  // Update queued messages count
  const updateQueueCount = useCallback(() => {
    const count = webSocketService.getQueuedMessageCount();
    setQueuedMessages(count);
  }, []);

  // Setup effect
  useEffect(() => {
    // Subscribe to status changes
    const statusUnsubscribe = webSocketService.onStatusChange(handleStatusChange);

    // Subscribe to message types
    const messageUnsubscribers: (() => void)[] = [];

    messageTypes.forEach(messageType => {
      const unsubscribe = webSocketService.on(messageType, handleMessage);
      messageUnsubscribers.push(unsubscribe);
    });

    // Update queue count periodically
    const queueInterval = setInterval(updateQueueCount, 1000);
    updateQueueCount(); // Initial update

    // Auto-connect if enabled
    if (autoConnect && (reconnectOnMount || status === 'disconnected')) {
      connect().catch(_error => {
        // Auto-connect failed, will retry automatically
        // Don't throw here as it's automatic
      });
    }

    // Cleanup function
    return () => {
      statusUnsubscribe();
      messageUnsubscribers.forEach(unsubscribe => unsubscribe());
      unsubscribersRef.current.forEach(unsubscribe => unsubscribe());
      clearInterval(queueInterval);
    };
  }, [autoConnect, reconnectOnMount, handleStatusChange, handleMessage, connect, updateQueueCount, status, messageTypes]);

  // Cleanup on unmount
  useEffect(() => {
    const currentHandlers = handlersRef.current;
    const currentUnsubscribers = unsubscribersRef.current;

    return () => {
      // Clean up all subscriptions
      currentUnsubscribers.forEach(unsubscribe => unsubscribe());
      unsubscribersRef.current = [];
      currentHandlers.clear();
    };
  }, []);

  return {
    status,
    isConnected: status === 'connected',
    isConnecting: status === 'connecting',
    hasError: status === 'error',
    queuedMessages,
    lastMessage,
    lastError,
    connect,
    disconnect,
    sendMessage,
    subscribe,
    clearQueue
  };
}

// Specialized hooks for different message types
export function useWebSocketScans() {
  const webSocket = useWebSocket({
    messageTypes: ['scan_update', 'scan_started', 'scan_completed', 'scan_error']
  });

  const [scans] = useState<unknown[]>([]);
  const [activeScan] = useState<unknown>(null);

  useEffect(() => {
    const unsubscribe = webSocket.subscribe('scan_update', (message) => {
      if (message.type === 'scan_update' && message.payload) {
        // Update scans list or active scan
        // Handle scan updates here
        // TODO: Process scan update data
      }
    });

    return unsubscribe;
  }, [webSocket]);

  return {
    ...webSocket,
    scans,
    activeScan
  };
}

export function useWebSocketDiscovery() {
  const webSocket = useWebSocket({
    messageTypes: ['discovery_update', 'discovery_started', 'discovery_completed', 'discovery_error']
  });

  const [discoveryJobs] = useState<unknown[]>([]);
  const [activeDiscovery] = useState<unknown>(null);

  useEffect(() => {
    const unsubscribe = webSocket.subscribe('discovery_update', (message) => {
      if (message.type === 'discovery_update' && message.payload) {
        // Update discovery jobs
        // Handle discovery updates here
        // TODO: Process discovery update data
      }
    });

    return unsubscribe;
  }, [webSocket]);

  return {
    ...webSocket,
    discoveryJobs,
    activeDiscovery
  };
}

export function useWebSocketHealth() {
  const webSocket = useWebSocket({
    messageTypes: ['system_health', 'health_update', 'status_change']
  });

  const [systemHealth, setSystemHealth] = useState<unknown>(null);
  const [lastHealthUpdate, setLastHealthUpdate] = useState<Date | null>(null);

  useEffect(() => {
    const unsubscribe = webSocket.subscribe('system_health', (message) => {
      if (message.payload) {
        setSystemHealth(message.payload);
        setLastHealthUpdate(new Date());
      }
    });

    return unsubscribe;
  }, [webSocket]);

  return {
    ...webSocket,
    systemHealth,
    lastHealthUpdate
  };
}

export default useWebSocket;
