"use client";

import { useEffect, useRef } from "react";

// Widget do Cloudflare Turnstile, renderizado explicitamente. Quem decide
// se o token vale é a API (siteverify no Go); aqui só se obtém o token.

type RenderOptions = {
  sitekey: string;
  action: string;
  language: string;
  theme: "light" | "dark" | "auto";
  size: "normal" | "flexible" | "compact";
  callback: (token: string) => void;
  "expired-callback": () => void;
  "error-callback": () => void;
};

type TurnstileApi = {
  render: (el: HTMLElement, opts: RenderOptions) => string;
  reset: (id: string) => void;
  remove: (id: string) => void;
};

declare global {
  interface Window {
    turnstile?: TurnstileApi;
  }
}

const SCRIPT_SRC = "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";

let scriptPromise: Promise<TurnstileApi> | undefined;

// Carrega o script uma vez por página, mesmo com vários widgets.
function loadTurnstile(): Promise<TurnstileApi> {
  if (window.turnstile) return Promise.resolve(window.turnstile);
  scriptPromise ??= new Promise<TurnstileApi>((resolve, reject) => {
    const s = document.createElement("script");
    s.src = SCRIPT_SRC;
    s.async = true;
    s.onload = () => (window.turnstile ? resolve(window.turnstile) : reject(new Error("turnstile ausente")));
    s.onerror = () => {
      scriptPromise = undefined;
      s.remove();
      reject(new Error("falha ao carregar o turnstile"));
    };
    document.head.appendChild(s);
  });
  return scriptPromise;
}

type Props = {
  siteKey: string;
  action: string;
  /** Recebe o token, ou null quando ele vence ou o desafio falha. */
  onToken: (token: string | null) => void;
  /** O script não carregou (bloqueador, rede). */
  onLoadError: () => void;
  /** Mudar este número pede um token novo (o token vale uma vez só). */
  resetKey: number;
  describedBy?: string;
};

export default function Turnstile({ siteKey, action, onToken, onLoadError, resetKey, describedBy }: Props) {
  const box = useRef<HTMLDivElement>(null);
  const widget = useRef<string | undefined>(undefined);
  // Callbacks atuais sem re-renderizar o widget a cada render do formulário.
  const handlers = useRef({ onToken, onLoadError });
  handlers.current = { onToken, onLoadError };

  useEffect(() => {
    let cancelled = false;
    loadTurnstile()
      .then((api) => {
        if (cancelled || !box.current) return;
        widget.current = api.render(box.current, {
          sitekey: siteKey,
          action,
          language: "pt-BR",
          theme: "light",
          size: "flexible",
          callback: (t) => handlers.current.onToken(t),
          "expired-callback": () => handlers.current.onToken(null),
          "error-callback": () => handlers.current.onToken(null),
        });
      })
      .catch(() => {
        if (!cancelled) handlers.current.onLoadError();
      });
    return () => {
      cancelled = true;
      if (widget.current) window.turnstile?.remove(widget.current);
      widget.current = undefined;
    };
  }, [siteKey, action]);

  useEffect(() => {
    if (resetKey === 0 || !widget.current) return;
    handlers.current.onToken(null);
    window.turnstile?.reset(widget.current);
  }, [resetKey]);

  // Altura reservada (65px é a do widget) para o formulário não pular.
  return <div ref={box} className="min-h-[65px] max-w-md" aria-describedby={describedBy} />;
}
