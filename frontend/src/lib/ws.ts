export type WsStatus = "connecting" | "connected" | "disconnected" | "error"

export interface WsMessage {
  type: string
  timestamp: string
  data: unknown
  request_id?: string
}

type MessageHandler = (msg: WsMessage) => void

export class WsManager {
  private url: string
  private apiKey: string
  private ws: WebSocket | null = null
  private status: WsStatus = "disconnected"
  private statusListeners: Set<(s: WsStatus) => void> = new Set()
  private messageListeners: Map<string, Set<MessageHandler>> = new Map()
  private retryTimeout: ReturnType<typeof setTimeout> | null = null
  private retryCount = 0
  private readonly maxRetries = 10
  private readonly baseDelay = 1000
  private readonly maxDelay = 30_000
  private shouldReconnect = true

  constructor(url: string, apiKey: string) {
    this.url = url
    this.apiKey = apiKey
  }

  connect(): void {
    if (
      this.ws?.readyState === WebSocket.OPEN ||
      this.ws?.readyState === WebSocket.CONNECTING
    ) {
      return
    }

    this.setStatus("connecting")

    const sep = this.url.includes("?") ? "&" : "?"
    const fullUrl = `${this.url}${sep}api_key=${encodeURIComponent(this.apiKey)}`
    const ws = new WebSocket(fullUrl)
    this.ws = ws

    ws.onopen = () => {
      this.retryCount = 0
      this.setStatus("connected")
    }

    ws.onclose = () => {
      this.ws = null
      this.setStatus("disconnected")
      if (this.shouldReconnect) {
        this.scheduleReconnect()
      }
    }

    ws.onerror = () => {
      this.setStatus("error")
    }

    ws.onmessage = (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(ev.data as string) as WsMessage
        const handlers = this.messageListeners.get(msg.type)
        if (handlers) {
          handlers.forEach((h) => h(msg))
        }
        // Also fire wildcard listeners
        const wildcards = this.messageListeners.get("*")
        if (wildcards) {
          wildcards.forEach((h) => h(msg))
        }
      } catch {
        // Ignore parse errors
      }
    }
  }

  disconnect(): void {
    this.shouldReconnect = false
    if (this.retryTimeout !== null) {
      clearTimeout(this.retryTimeout)
      this.retryTimeout = null
    }
    this.ws?.close()
    this.ws = null
  }

  /**
   * Subscribe to messages of a given type. Pass "*" to receive all messages.
   * Returns an unsubscribe function.
   */
  on(type: string, handler: MessageHandler): () => void {
    if (!this.messageListeners.has(type)) {
      this.messageListeners.set(type, new Set())
    }
    this.messageListeners.get(type)!.add(handler)
    return () => {
      this.messageListeners.get(type)?.delete(handler)
    }
  }

  /**
   * Subscribe to status changes. Returns an unsubscribe function.
   */
  onStatusChange(cb: (s: WsStatus) => void): () => void {
    this.statusListeners.add(cb)
    return () => {
      this.statusListeners.delete(cb)
    }
  }

  getStatus(): WsStatus {
    return this.status
  }

  private setStatus(s: WsStatus): void {
    this.status = s
    this.statusListeners.forEach((cb) => cb(s))
  }

  private scheduleReconnect(): void {
    if (this.retryCount >= this.maxRetries) {
      this.setStatus("error")
      return
    }
    const delay = Math.min(
      this.baseDelay * Math.pow(2, this.retryCount),
      this.maxDelay,
    )
    this.retryCount++
    this.retryTimeout = setTimeout(() => {
      this.connect()
    }, delay)
  }
}
