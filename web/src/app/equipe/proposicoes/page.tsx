"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError, RequireTeam } from "@/lib/team/ui";
import { REVIEW_STATE_LABEL, chamberLabel, fmtDateTime } from "@/lib/labels";

type TeamProposition = components["schemas"]["TeamProposition"];

function List() {
  const { api } = useTeamSession();
  const [items, setItems] = useState<TeamProposition[]>();
  const [error, setError] = useState<string>();

  useEffect(() => {
    api<{ propositions: TeamProposition[] }>("/api/team/propositions")
      .then((r) => setItems(r.propositions))
      .catch((e) => setError(e instanceof ApiError ? e.message : "Não foi possível carregar."));
    // só ao abrir a página
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold">Proposições</h1>
        <Link href="/equipe/proposicoes/importar" className="btn-lg">Importar</Link>
      </div>
      <FormError message={error} />
      {!items && !error && <p role="status" className="text-stone-600">Carregando…</p>}
      {items?.length === 0 && <p className="card">Nenhuma Proposição ainda. Comece importando uma da Câmara ou do Senado.</p>}
      {items && items.length > 0 && (
        <ul className="divide-y divide-stone-200 rounded-lg border border-stone-200 bg-white">
          {items.map((p) => {
            const suggested = p.links.filter((l) => l.state === "suggested").length;
            const failing = p.links.some((l) => l.state === "confirmed" && l.last_sync_error);
            return (
              <li key={p.id} className="space-y-1 px-4 py-3">
                <p className="flex flex-wrap items-center gap-2">
                  <Link href={`/equipe/proposicoes/${p.id}`} className="font-semibold hover:underline">{p.name}</Link>
                  <span className="rounded bg-stone-100 px-1.5 py-0.5 text-xs font-medium text-stone-700">{REVIEW_STATE_LABEL[p.review_state]}</span>
                  {suggested > 0 && <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-900">{suggested} vínculo(s) a conferir</span>}
                  {failing && <span className="rounded bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-800">erro de sincronização</span>}
                </p>
                <p className="line-clamp-2 text-sm text-stone-700">{p.summary}</p>
                <p className="text-xs text-stone-600">
                  {chamberLabel(p.origin_chamber)} · atualizada em {fmtDateTime(p.updated_at)}
                </p>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

export default function Page() {
  return <RequireTeam>{() => <List />}</RequireTeam>;
}
