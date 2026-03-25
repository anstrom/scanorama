import { createContext, useContext, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { WsManager } from "./ws";
import type { WsStatus } from "./ws";

// ── Context ────────────────────────────────────────────────────────────────────

interface WsContextValue {
  manager: WsManager | null;
  status: WsStatus;
}

const INITIAL: WsContextValue = { manager: null, status: "disconnected" };

const WsContext = createContext<WsContextValue>(INITIAL);

// ── Provider ───────────────────────────────────────────────────────────────────

export function WsProvider({
  apiKey,
  children,
}: {
  apiKey: string;
  children: ReactNode;
}) {
  // Bundle manager + status into one state object so we never read a ref during
  // render and never call setState synchronously in the effect body.
  // The effect only subscribes; the manager is delivered to state via the
  // onStatusChange callback, which fires asynchronously after connect().
  const [ctx, setCtx] = useState<WsContextValue>(INITIAL);

  useEffect(() => {
    const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${wsProtocol}//${window.location.host}/api/v1/ws`;
    const mgr = new WsManager(url, apiKey);

    // Wire status updates — the callback fires asynchronously (on WS events),
    // so React never sees setState called synchronously inside the effect body.
    const unsub = mgr.onStatusChange((s: WsStatus) => {
      setCtx({ manager: mgr, status: s });
    });

    // Defer connect() by one tick so the very first setStatus("connecting")
    // call — which happens synchronously inside connect() — is not a
    // synchronous setState within the effect setup body.
    const timerId = setTimeout(() => {
      mgr.connect();
    }, 0);

    return () => {
      clearTimeout(timerId);
      unsub();
      mgr.disconnect();
      setCtx(INITIAL);
    };
  }, [apiKey]);

  return <WsContext.Provider value={ctx}>{children}</WsContext.Provider>;
}

// ── Hooks ──────────────────────────────────────────────────────────────────────
//
// Note: eslint-plugin-react-refresh may warn that this file mixes a component
// (WsProvider) with hook exports. This is a dev-only fast-refresh heuristic
// and does not affect production builds or runtime behaviour.

export function useWs(): WsContextValue {
  return useContext(WsContext);
}

export function useWsStatus(): WsStatus {
  return useContext(WsContext).status;
}
