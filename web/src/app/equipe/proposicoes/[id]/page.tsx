"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError, RequireTeam } from "@/lib/team/ui";
import { REVIEW_STATE_LABEL, chamberLabel, fmtDate, fmtDateTime, officialText, chamberIn } from "@/lib/labels";

type Detail = components["schemas"]["TeamPropositionDetail"];
type Link_ = components["schemas"]["SourceLink"];

const TIMELINE_PAGE = 20;

function PropositionDetail({ id }: { id: string }) {
  const { api } = useTeamSession();
  const [p, setP] = useState<Detail>();
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState<string>();
  const [busy, setBusy] = useState<string>();
  const [showAll, setShowAll] = useState(false);

  const load = useCallback(async () => {
    try {
      setP(await api<Detail>(`/api/team/propositions/${encodeURIComponent(id)}`));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Não foi possível carregar.");
    }
  }, [api, id]);

  useEffect(() => {
    void load();
    const unavailable = new URLSearchParams(window.location.search).get("indisponivel");
    if (unavailable) {
      setNotice(`Importada. Não foi possível procurar vínculos em: ${unavailable.split(",").map(chamberLabel).join(", ")}. Tente importar de lá mais tarde.`);
    }
    // só ao abrir a página
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function act(key: string, path: string, done: string) {
    setBusy(key);
    setError(undefined);
    setNotice(undefined);
    try {
      await api(path, { method: "POST" });
      setNotice(done);
      await load();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "A ação falhou. Tente de novo.");
    } finally {
      setBusy(undefined);
    }
  }

  if (!p) {
    return error ? <FormError message={error} /> : <p role="status" className="text-stone-600">Carregando…</p>;
  }

  const base = `/api/team/propositions/${encodeURIComponent(p.id)}`;
  const timeline = [...p.timeline].reverse();
  const shown = showAll ? timeline : timeline.slice(0, TIMELINE_PAGE);

  return (
    <div className="space-y-8">
      <p><Link href="/equipe/proposicoes" className="link">← Proposições</Link></p>
      <header className="space-y-2">
        <p className="flex flex-wrap items-center gap-2 text-sm text-stone-600">
          <span className="rounded bg-stone-100 px-1.5 py-0.5 font-medium text-stone-700">{REVIEW_STATE_LABEL[p.review_state]}</span>
          <span>Iniciada {chamberIn(p.origin_chamber)} · apresentada em {fmtDate(p.presented_at)}</span>
        </p>
        <h1 className="text-2xl font-bold">{p.name}</h1>
        <p className="leading-relaxed text-stone-800">{p.official_summary}</p>
      </header>

      <div aria-live="polite" className="space-y-2">
        <FormError message={error} />
        {notice && <p role="status" className="rounded-md border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-900">{notice}</p>}
      </div>

      <section aria-labelledby="revisao" className="card space-y-3">
        <h2 id="revisao" className="text-lg font-semibold">Revisão</h2>
        {p.review_state === "draft" ? (
          <>
            <p className="text-stone-700">
              Criada: ninguém de fora vê. Ao enviar para revisão, ela aparece no site como &ldquo;em análise&rdquo;, só com os
              dados oficiais. Confira antes os vínculos sugeridos.
            </p>
            <button type="button" className="btn-lg" disabled={busy !== undefined} onClick={() => act("submit", `${base}/submit-for-review`, "Enviada para revisão. Já aparece no site como “em análise”.")}>
              {busy === "submit" ? "Enviando…" : "Enviar para revisão"}
            </button>
          </>
        ) : (
          <p className="text-stone-700">
            {p.review_state === "in_review" ? "Em revisão: aparece no site como “em análise”." : "Pronta."}{" "}
            <a href={`/proposicao/${p.slug}`} className="link">Ver como o leitor vê</a>
          </p>
        )}
      </section>

      <section aria-labelledby="vinculos" className="card space-y-3">
        <h2 id="vinculos" className="text-lg font-semibold">Vínculos de fonte</h2>
        <ul className="space-y-3">
          {p.links.map((l: Link_) => (
            <li key={l.id} className="space-y-1 border-t border-stone-200 pt-3 first:border-0 first:pt-0">
              <p className="font-medium">
                {chamberLabel(l.chamber)} · {l.source_name}{" "}
                {l.state === "suggested" && <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-900">sugerido</span>}
              </p>
              <p className="text-sm"><a href={l.source_url} className="link" rel="noopener noreferrer">Conferir {chamberIn(l.chamber)}</a></p>
              {l.state === "confirmed" ? (
                <p className="text-sm text-stone-600">
                  Confirmado em {fmtDateTime(l.confirmed_at)} · última sincronização: {fmtDateTime(l.last_synced_at)}
                  {l.last_sync_error && <span className="block text-red-800">Erro na última sincronização: {l.last_sync_error}</span>}
                </p>
              ) : (
                <div className="flex flex-wrap gap-2 pt-1">
                  <button type="button" className="btn-lg" disabled={busy !== undefined} onClick={() => act(`c-${l.id}`, `${base}/links/${l.id}/confirm`, "Vínculo confirmado. A Tramitação dessa Casa entra na próxima sincronização.")}>
                    {busy === `c-${l.id}` ? "Confirmando…" : "É a mesma Proposição"}
                  </button>
                  <button type="button" className="btn-ghost min-h-12 px-4" disabled={busy !== undefined} onClick={() => act(`r-${l.id}`, `${base}/links/${l.id}/reject`, "Sugestão descartada.")}>
                    {busy === `r-${l.id}` ? "Descartando…" : "Não é a mesma"}
                  </button>
                </div>
              )}
            </li>
          ))}
        </ul>
      </section>

      <section aria-labelledby="dados" className="card space-y-2">
        <h2 id="dados" className="text-lg font-semibold">Dados oficiais</h2>
        <p><span className="font-medium text-stone-600">Situação:</span> {officialText(p.status) || "—"}{p.superseded && " (prejudicada)"}</p>
        {p.attached_to && (
          <p>
            <span className="font-medium text-stone-600">Apensada a:</span>{" "}
            {p.attached_to.id ? <Link href={`/equipe/proposicoes/${p.attached_to.id}`} className="link">{p.attached_to.name}</Link> : `${p.attached_to.name} (não importada)`}
          </p>
        )}
        {p.official_text_url && <p><a href={p.official_text_url} className="link" rel="noopener noreferrer">Documento integral</a></p>}
        <p className="text-sm text-stone-600">Autoria: {p.authors.slice(0, 6).join(", ")}{p.authors.length > 6 && ` e mais ${p.authors.length - 6}`}</p>
      </section>

      <section aria-labelledby="tramitacao" className="space-y-3">
        <h2 id="tramitacao" className="text-lg font-semibold">Tramitação ({timeline.length} eventos)</h2>
        <ol className="space-y-3">
          {shown.map((e, i) => (
            <li key={`${e.chamber}-${e.at}-${i}`} className="border-l-2 border-stone-300 pl-4">
              <p className="text-sm text-stone-600">{fmtDate(e.at)} · {chamberLabel(e.chamber)}{e.body && ` · ${e.body}`}</p>
              <p className="font-medium">{e.description}</p>
              {e.dispatch && e.dispatch !== e.description && <p className="text-stone-700 [overflow-wrap:anywhere]">{e.dispatch}</p>}
            </li>
          ))}
        </ol>
        {timeline.length > TIMELINE_PAGE && (
          <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setShowAll((v) => !v)}>
            {showAll ? "Mostrar só os mais recentes" : `Mostrar todos os ${timeline.length} eventos`}
          </button>
        )}
      </section>
    </div>
  );
}

export default function Page() {
  const { id } = useParams<{ id: string }>();
  return <RequireTeam>{() => <PropositionDetail id={id} />}</RequireTeam>;
}
