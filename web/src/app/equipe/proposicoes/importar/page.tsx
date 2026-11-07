"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError, RequireTeam } from "@/lib/team/ui";
import { chamberLabel, fmtDate, officialText, chamberIn, chamberThe } from "@/lib/labels";

type SearchResult = components["schemas"]["SourceSearchResult"];
type Hit = components["schemas"]["SourceSearchHit"];
type ImportResult = components["schemas"]["ImportResult"];

const KINDS = ["PL", "PEC", "PLP", "MPV", "PDL", "PLN"];

function Importer() {
  const { api } = useTeamSession();
  const router = useRouter();
  const [results, setResults] = useState<SearchResult[]>();
  const [error, setError] = useState<string>();
  const [searching, setSearching] = useState(false);
  const [importing, setImporting] = useState<string>();

  async function onSearch(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const q = new URLSearchParams({ kind: String(fd.get("kind")), number: String(fd.get("number")), year: String(fd.get("year")) });
    setSearching(true);
    setError(undefined);
    setResults(undefined);
    try {
      const r = await api<{ results: SearchResult[] }>(`/api/team/sources/search?${q}`);
      setResults(r.results);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "A busca falhou. Tente de novo.");
    } finally {
      setSearching(false);
    }
  }

  async function onImport(chamber: string, hit: Hit) {
    setImporting(`${chamber}-${hit.external_id}`);
    setError(undefined);
    try {
      const r = await api<ImportResult>("/api/team/propositions/import", {
        method: "POST",
        body: JSON.stringify({ chamber, external_id: hit.external_id }),
      });
      const q = r.suggestions_unavailable.length ? `?indisponivel=${r.suggestions_unavailable.join(",")}` : "";
      router.push(`/equipe/proposicoes/${r.proposition_id}${q}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "A importação falhou. Tente de novo.");
      setImporting(undefined);
    }
  }

  return (
    <div className="space-y-6">
      <p><Link href="/equipe/proposicoes" className="link">← Proposições</Link></p>
      <h1 className="text-2xl font-bold">Importar Proposição</h1>
      <p className="text-stone-700">
        Busque pelo nome oficial. A Proposição entra como &ldquo;criado&rdquo;, invisível ao público, com a Tramitação da
        Casa escolhida. Se a outra Casa tiver um registro com o mesmo nome, ele aparece como vínculo sugerido para você conferir.
      </p>
      <form onSubmit={onSearch} className="card grid gap-4 sm:grid-cols-[1fr_1fr_1fr_auto] sm:items-end">
        <div>
          <label htmlFor="kind" className="label">Tipo</label>
          <select id="kind" name="kind" defaultValue="PL" className="input min-h-11 text-base">
            {KINDS.map((k) => <option key={k} value={k}>{k}</option>)}
          </select>
        </div>
        <div>
          <label htmlFor="number" className="label">Número</label>
          <input id="number" name="number" type="number" inputMode="numeric" min={1} required className="input min-h-11 text-base" />
        </div>
        <div>
          <label htmlFor="year" className="label">Ano</label>
          <input id="year" name="year" type="number" inputMode="numeric" min={1900} max={2100} required className="input min-h-11 text-base" />
        </div>
        <button type="submit" disabled={searching} className="btn-lg">{searching ? "Buscando…" : "Buscar"}</button>
      </form>
      <FormError message={error} />

      {results && (
        <section aria-label="Resultados" className="space-y-4" aria-live="polite">
          {results.map((r) => (
            <div key={r.chamber} className="card space-y-3">
              <h2 className="font-semibold">{chamberLabel(r.chamber)}</h2>
              {r.unavailable && <p className="text-sm text-amber-900">{chamberThe(r.chamber)} não respondeu. Tente de novo em alguns minutos.</p>}
              {!r.unavailable && r.hits.length === 0 && <p className="text-sm text-stone-600">Nada encontrado nesta Casa.</p>}
              {r.hits.map((h) => {
                const key = `${r.chamber}-${h.external_id}`;
                return (
                  <div key={key} className="space-y-2 border-t border-stone-200 pt-3 first:border-0 first:pt-0">
                    <p className="font-medium">
                      {h.name} <span className="text-sm font-normal text-stone-600">· apresentada em {fmtDate(h.presented_at)}</span>
                    </p>
                    <p className="text-stone-800">{h.summary}</p>
                    {h.status && <p className="text-sm text-stone-600">Situação: {officialText(h.status)}</p>}
                    <p className="text-sm"><a href={h.source_url} className="link" rel="noopener noreferrer">Ver {chamberIn(r.chamber)}</a></p>
                    {h.linked_proposition_id ? (
                      <p className="text-sm">
                        Já {h.link_state === "suggested" ? "sugerida" : "vinculada"} a uma Proposição:{" "}
                        <Link href={`/equipe/proposicoes/${h.linked_proposition_id}`} className="link">abrir</Link>
                      </p>
                    ) : (
                      <button type="button" onClick={() => onImport(r.chamber, h)} disabled={importing !== undefined} className="btn-lg">
                        {importing === key ? "Importando…" : "Importar desta Casa"}
                      </button>
                    )}
                  </div>
                );
              })}
            </div>
          ))}
        </section>
      )}
    </div>
  );
}

export default function Page() {
  return <RequireTeam>{() => <Importer />}</RequireTeam>;
}
