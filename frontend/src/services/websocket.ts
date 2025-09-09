import type { ConnectionStatus, WebSocketMessage, MessageHandler } from '../types';
import { createLogger } from '../utils/logger';

// ============================================================================
// TYPES
// ============================================================================

interface WebSocketConfig {
  readonly url?: string;
  readonly reconnectInterval?: number;
  readonly maxReconnectAttempts?: number;
  readonly heartbeatInterval?: number;
}





// ============================================================================
// WEBSOCKET SERVICE CLASS
// ============================================================================

class WebSocketService {
  private readonly logger = createLogger('WebSocketService');
  private readonly config: Required<WebSocketConfig>;
  private ws: WebSocket | null = null;
  private status: ConnectionStatus = 'disconnected';
  private reconnectAttempts = 0;
  private readonly messageHandlers = new Map<string, Set<MessageHandler>>();
  private readonly statusListeners = new Set<(status: ConnectionStatus) => void>();
  private heartbeatTimer: NodeJS.Timeout | null = null;
  private reconnectTimer: NodeJS.Timeout | null = null;
  private readonly messageQueue: WebSocketMessage[] = [];

  constructor(config: WebSocketConfig = {}) {
    this.config = {
      url: config.url ?? this.getDefaultWebSocketUrl(),
      reconnectInterval: config.reconnectInterval ?? 3000,
      maxReconnectAttempts: config.maxReconnectAttempts ?? 5,
      heartbeatInterval: config.heartbeatInterval ?? 30000,
    };

    this.logger.debug('WebSocket service initialized', {
      config: this.config,
    });
  }

  // ============================================================================
  // PRIVATE METHODS
  // ============================================================================

  private getDefaultWebSocketUrl(): string {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;

    // In development, use the same host and port (Vite proxy will handle routing)
    // In production, construct the URL based on the current location
    const url = `${protocol}//${host}/api/v1/ws`;

    this.logger.debug('WebSocket URL constructed', { url });
    return url;
  }

  private setStatus(newStatus: ConnectionStatus): void {
    if (this.status !== newStatus) {
      const previousStatus = this.status;
      this.status = newStatus;

      this.logger.websocket(`Status changed: ${previousStatus} -> ${newStatus}`);

      this.statusListeners.forEach(listener => {
        try {
          listener(newStatus);
        } catch (error) {
          this.logger.error('Status listener error', undefined, error as Error);
        }
      });
    }
  }

  private setupHeartbeat(): void {
    this.clearHeartbeat();

    this.heartbeatTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.send('ping', {});
      }
    }, this.config.heartbeatInterval);

    this.logger.debug('Heartbeat configured', {
      interval: this.config.heartbeatInterval,
    });
  }

  private clearHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
    }

    if (this.reconnectAttempts >= this.config.maxReconnectAttempts) {
      this.logger.error('Max reconnection attempts reached', {
        attempts: this.reconnectAttempts,
        maxAttempts: this.config.maxReconnectAttempts,
      });
      this.setStatus('error');
      return;
    }

    this.setStatus('reconnecting');

    // Exponential backoff with jitter
    const delay = Math.min(
      this.config.reconnectInterval * Math.pow(2, this.reconnectAttempts) +
      Math.random() * 1000,
      30000
    );

    this.logger.debug('Scheduling reconnection', {
      attempt: this.reconnectAttempts + 1,
      delay,
    });

    this.reconnectTimer = setTimeout(() => {
      this.reconnectAttempts++;
      this.connect().catch(error => {
        this.logger.error('Reconnection failed', undefined, error);
      });
    }, delay);
  }

  private processMessageQueue(): void {
    let processed = 0;
    while (this.messageQueue.length > 0 && this.ws?.readyState === WebSocket.OPEN) {
      const message = this.messageQueue.shift();
      if (message) {
        try {
          this.ws.send(JSON.stringify(message));
          processed++;
        } catch (error) {
          this.logger.error('Failed to send queued message', { message }, error as Error);
          break;
        }
      }
    }

    if (processed > 0) {
      this.logger.debug('Processed message queue', {
        processed,
        remaining: this.messageQueue.length,
      });
    }
  }

  private handleMessage(event: MessageEvent): void {
    try {
      const message = JSON.parse(event.data) as WebSocketMessage;

      // Handle heartbeat responses
      if (message.type === 'pong') {
        this.logger.debug('Received heartbeat response');
        return;
      }

      this.logger.debug('Received WebSocket message', {
        type: message.type,
        timestamp: message.timestamp,
      });

      // Dispatch to type-specific handlers
      const handlers = this.messageHandlers.get(message.type);
      if (handlers) {
        handlers.forEach(handler => {
          try {
            handler(message);
          } catch (error) {
            this.logger.error('Message handler error', {
              messageType: message.type,
            }, error as Error);
          }
        });
      }

      // Dispatch to wildcard handlers
      const wildcardHandlers = this.messageHandlers.get('*');
      if (wildcardHandlers) {
        wildcardHandlers.forEach(handler => {
          try {
            handler(message);
          } catch (error) {
            this.logger.error('Wildcard handler error', {
              messageType: message.type,
            }, error as Error);
          }
        });
      }
    } catch (error) {
      this.logger.error('Failed to parse WebSocket message', {
        rawData: event.data,
      }, error as Error);
    }
  }

  // ============================================================================
  // PUBLIC METHODS
  // ============================================================================

  async connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        resolve();
        return;
      }

      this.setStatus('connecting');
      this.logger.info('Connecting to WebSocket', { url: this.config.url });

      try {
        this.logger.info('Attempting WebSocket connection', { url: this.config.url });
        this.ws = new WebSocket(this.config.url);

        // Set up connection timeout
        const connectionTimeout = setTimeout(() => {
          if (this.ws && this.ws.readyState === WebSocket.CONNECTING) {
            this.logger.error('WebSocket connection timeout');
            this.ws.close();
            this.setStatus('error');
            reject(new Error('WebSocket connection timeout'));
          }
        }, 10000); // 10 second timeout

        this.ws.onopen = (): void => {
          clearTimeout(connectionTimeout);
          this.logger.info('WebSocket connection established');
          this.setStatus('connected');
          this.reconnectAttempts = 0;
          this.setupHeartbeat();
          this.processMessageQueue();
          resolve();
        };

        this.ws.onclose = (event: CloseEvent): void => {
          clearTimeout(connectionTimeout);
          this.logger.warn('WebSocket connection closed', {
            code: event.code,
            reason: event.reason,
            wasClean: event.wasClean,
            url: this.config.url,
          });

          this.clearHeartbeat();

          if (event.wasClean) {
            this.setStatus('disconnected');
          } else {
            this.scheduleReconnect();
          }
        };

        this.ws.onerror = (error: Event): void => {
          clearTimeout(connectionTimeout);
          this.logger.error('WebSocket connection error', {
            url: this.config.url,
            readyState: this.ws?.readyState,
            error
          });
          this.clearHeartbeat();
          this.setStatus('error');
          reject(new Error(`WebSocket connection failed to ${this.config.url}`));
        };

        this.ws.onmessage = (event: MessageEvent): void => {
          this.handleMessage(event);
        };

      } catch (error) {
        this.logger.error('Failed to create WebSocket connection', { url: this.config.url }, error as Error);
        this.setStatus('error');
        reject(error);
      }
    });
  }

  disconnect(): void {
    this.logger.info('Disconnecting WebSocket');

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    this.clearHeartbeat();

    if (this.ws) {
      this.ws.close(1000, 'Client disconnect');
      this.ws = null;
    }

    this.setStatus('disconnected');
    this.reconnectAttempts = 0;
  }

  send(type: string, payload: unknown): boolean {
    const message: WebSocketMessage = {
      type,
      payload,
      timestamp: new Date().toISOString(),
    };

    if (this.ws?.readyState === WebSocket.OPEN) {
      try {
        this.ws.send(JSON.stringify(message));
        this.logger.debug('Sent WebSocket message', {
          type,
          timestamp: message.timestamp,
        });
        return true;
      } catch (error) {
        this.logger.error('Failed to send WebSocket message', {
          type,
        }, error as Error);
        return false;
      }
    } else {
      // Queue message for when connection is restored
      this.messageQueue.push(message);
      this.logger.debug('Queued WebSocket message', {
        type,
        queueSize: this.messageQueue.length,
      });

      // Try to reconnect if not already connecting
      if (this.status === 'disconnected') {
        this.connect().catch(error => {
          this.logger.error('Auto-reconnect failed', undefined, error);
        });
      }

      return false;
    }
  }

  on(messageType: string, handler: MessageHandler): () => void {
    if (!this.messageHandlers.has(messageType)) {
      this.messageHandlers.set(messageType, new Set());
    }

    this.messageHandlers.get(messageType)!.add(handler);

    this.logger.debug('Registered message handler', { messageType });

    // Return unsubscribe function
    return (): void => {
      const handlers = this.messageHandlers.get(messageType);
      if (handlers) {
        handlers.delete(handler);
        if (handlers.size === 0) {
          this.messageHandlers.delete(messageType);
        }
      }
      this.logger.debug('Unregistered message handler', { messageType });
    };
  }

  off(messageType: string, handler?: MessageHandler): void {
    if (handler) {
      const handlers = this.messageHandlers.get(messageType);
      if (handlers) {
        handlers.delete(handler);
        if (handlers.size === 0) {
          this.messageHandlers.delete(messageType);
        }
      }
    } else {
      this.messageHandlers.delete(messageType);
    }

    this.logger.debug('Removed message handler(s)', { messageType });
  }

  onStatusChange(listener: (status: ConnectionStatus) => void): () => void {
    this.statusListeners.add(listener);

    this.logger.debug('Registered status listener');

    // Return unsubscribe function
    return (): void => {
      this.statusListeners.delete(listener);
      this.logger.debug('Unregistered status listener');
    };
  }

  getStatus(): ConnectionStatus {
    return this.status;
  }

  isConnected(): boolean {
    return this.status === 'connected';
  }

  getQueuedMessageCount(): number {
    return this.messageQueue.length;
  }

  clearMessageQueue(): void {
    const count = this.messageQueue.length;
    this.messageQueue.length = 0;

    if (count > 0) {
      this.logger.info('Cleared message queue', { clearedCount: count });
    }
  }

  // ============================================================================
  // CLEANUP
  // ============================================================================

  destroy(): void {
    this.logger.info('Destroying WebSocket service');
    this.disconnect();
    this.messageHandlers.clear();
    this.statusListeners.clear();
    this.clearMessageQueue();
  }
}

// ============================================================================
// SINGLETON INSTANCE
// ============================================================================

export const webSocketService = new WebSocketService();

// ============================================================================
// CLEANUP ON PAGE UNLOAD
// ============================================================================

if (typeof window !== 'undefined') {
  window.addEventListener('beforeunload', () => {
    webSocketService.destroy();
  });
}

// ============================================================================
// EXPORTS
// ============================================================================

export default webSocketService;
export type { WebSocketConfig, MessageHandler };
