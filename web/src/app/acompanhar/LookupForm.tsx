"use client";

import Link from "next/link";
import { useId, useRef, useState } from "react";
import type { components } from "@/lib/api/schema";
import { CONTRIBUTION_KIND_LABEL, CONTRIBUTION_STATE_LABEL, fmtDate } from "@/lib/labels";

type Status = components["schemas"]["ContributionStatus"];

// Motivos públicos de rejeição (lista fechada da moderação).
const REASON_LABEL: Record<string, string> = {
  source_not_confirmed: "A fonte não confirma a informação.",
  out_of_scope: "Fora do escopo do Nelisa.",
  already_registered: "Já estava cadastrado.",
  duplicate: "Contribuição repetida.",
  incorrect: "A informação não está correta.",
  other: "Outro motivo.",
};

export default function LookupForm() {
  const [code, setCode] = useState("");
  const [status, setStatus] = useState<Status>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const resultRef = useRef<HTMLDivElement>(null);
  const id = useId();

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    setStatus(undefined);
    try {
      // O código vai no corpo, nunca na URL (não fica em log nem em histórico).
      const res = await fetch("/api/public/contributions/lookup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ code }),
      });
      const data = await res.json().catch(() => null);
      if (res.ok) {
        setStatus(data as Status);
        requestAnimationFrame(() => resultRef.current?.focus());
      } else {
        setError(data?.error?.message ?? "Não foi possível consultar. Tente de novo.");
      }
    } catch {
      setError("Sem conexão com o Nelisa. Confira a internet e tente de novo.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <form onSubmit={onSubmit} className="card space-y-3">
        <div>
          <label htmlFor={`${id}-code`} className="mb-1 block font-medium">Código de acompanhamento</label>
          <input
            id={`${id}-code`}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            required
            placeholder="XXXX-XXXX-XXXX"
            autoComplete="off"
            autoCapitalize="characters"
            spellCheck={false}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? `${id}-code-error` : undefined}
            className="input min-h-12 max-w-xs font-mono text-lg tracking-wider"
          />
          {error && (
            <p id={`${id}-code-error`} role="alert" className="mt-1 text-sm text-red-800">
              {error}
            </p>
          )}
        </div>
        <button type="submit" disabled={busy || code.trim() === ""} className="btn-lg">
          {busy ? "Consultando…" : "Consultar"}
        </button>
      </form>

      {status && (
        <div ref={resultRef} tabIndex={-1} className="card space-y-2 outline-none" aria-labelledby={`${id}-result`}>
          <h2 id={`${id}-result`} className="text-xl font-semibold">{CONTRIBUTION_STATE_LABEL[status.state]}</h2>
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-stone-800">
            <dt className="text-stone-600">Tipo</dt>
            <dd>{CONTRIBUTION_KIND_LABEL[status.kind]}</dd>
            <dt className="text-stone-600">Enviada em</dt>
            <dd>{fmtDate(status.submitted_at)}</dd>
            {status.decided_at && (
              <>
                <dt className="text-stone-600">Avaliada em</dt>
                <dd>{fmtDate(status.decided_at)}</dd>
              </>
            )}
            {status.public_reason && (
              <>
                <dt className="text-stone-600">Motivo</dt>
                <dd>{REASON_LABEL[status.public_reason] ?? status.public_reason}</dd>
              </>
            )}
          </dl>
          {status.proposition_slug && (
            <p>
              <Link href={`/proposicao/${status.proposition_slug}`} className="link">Ver a Proposição</Link>
            </p>
          )}
        </div>
      )}
    </div>
  );
}
