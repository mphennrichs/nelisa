"use client";

// Sessão da equipe no navegador. O token de acesso fica só em memória; o
// refresh fica no cookie HttpOnly que a API grava (via proxy /api). Ao
// abrir a página, tentamos renovar para recuperar a sessão.
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { components } from "@/lib/api/schema";

export type User = components["schemas"]["User"];
export type Role = components["schemas"]["Role"];
type SessionDTO = components["schemas"]["Session"];

export const ROLE_LABEL: Record<Role, string> = { editor: "Editor", reviewer: "Revisor", admin: "Admin" };

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

async function call<T>(path: string, init: RequestInit & { token?: string } = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body) headers.set("Content-Type", "application/json");
  if (init.token) headers.set("Authorization", `Bearer ${init.token}`);
  let res: Response;
  try {
    res = await fetch(path, { ...init, headers, credentials: "same-origin", cache: "no-store" });
  } catch {
    throw new ApiError(0, "network", "Sem conexão. Verifique a internet e tente de novo.");
  }
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(res.status, body?.error?.code ?? "unknown", body?.error?.message ?? "Algo deu errado. Tente de novo.");
  }
  return body as T;
}

// Uma renovação por vez: o refresh rotaciona o token, então duas chamadas
// simultâneas fariam a segunda usar um token já gasto.
let refreshing: Promise<SessionDTO> | null = null;
function refresh() {
  refreshing ??= call<SessionDTO>("/api/team/auth/refresh", { method: "POST" }).finally(() => {
    refreshing = null;
  });
  return refreshing;
}

type State = { status: "loading" } | { status: "anonymous" } | { status: "authenticated"; token: string; user: User };

type Ctx = {
  state: State;
  login(email: string, password: string): Promise<void>;
  bootstrap(input: components["schemas"]["BootstrapRequest"]): Promise<void>;
  logout(): Promise<void>;
  /** Chamada autenticada; renova o token uma vez se ele tiver expirado. */
  api<T>(path: string, init?: RequestInit): Promise<T>;
  setSession(s: SessionDTO): void;
};

const SessionContext = createContext<Ctx | null>(null);

export function TeamSessionProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<State>({ status: "loading" });

  const setSession = useCallback((s: SessionDTO) => {
    setState({ status: "authenticated", token: s.access_token, user: s.user });
  }, []);

  useEffect(() => {
    refresh()
      .then(setSession)
      .catch(() => setState({ status: "anonymous" }));
  }, [setSession]);

  const value = useMemo<Ctx>(
    () => ({
      state,
      setSession,
      async login(email, password) {
        setSession(await call<SessionDTO>("/api/team/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }));
      },
      async bootstrap(input) {
        setSession(await call<SessionDTO>("/api/team/auth/bootstrap", { method: "POST", body: JSON.stringify(input) }));
      },
      async logout() {
        await call("/api/team/auth/logout", { method: "POST" }).catch(() => undefined);
        setState({ status: "anonymous" });
      },
      async api<T>(path: string, init: RequestInit = {}) {
        const token = state.status === "authenticated" ? state.token : "";
        try {
          return await call<T>(path, { ...init, token });
        } catch (e) {
          if (!(e instanceof ApiError) || e.status !== 401) throw e;
          try {
            const s = await refresh();
            setSession(s);
            return await call<T>(path, { ...init, token: s.access_token });
          } catch (again) {
            setState({ status: "anonymous" });
            throw again;
          }
        }
      },
    }),
    [state, setSession],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useTeamSession() {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useTeamSession fora do TeamSessionProvider");
  return ctx;
}
