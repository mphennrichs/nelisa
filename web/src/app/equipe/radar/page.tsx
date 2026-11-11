"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useCallback, useEffect, useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError, RequireTeam } from "@/lib/team/ui";
import { chamberIn, chamberLabel, fmtDate, fmtDateTime } from "@/lib/labels";

type Item = components["schemas"]["RadarItem"];
type ImportResult = components["schemas"]["ImportResult"];
type Filter = "pauta" | "apresentadas";

const FILTERS: { key?: Filter; label: string }[] = [
  { label: "Todas" },
  { key: "pauta", label: "Em pauta" },
  { key: "apresentadas", label: "Apresentadas" },
];

function isFilter(s: string | null): s is Filter {
  return s === "pauta" || s === "apresentadas";
}

const keyOf = (it: Item) => `${it.chamber}-${it.external_id}`;

function Radar() {
  const { api } = useTeamSession();
  const router = useRouter();
  const params = useSearchParams();
  const raw = params.get("ver");
  const filter = isFilter(raw) ? raw : undefined;
  const [data, setData] = useState<{ items: Item[]; last_scan_at: string | null }>();
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState<string>();
  const [busy, setBusy] = useState<string>();
  const [rowError, setRowError] = useState<{ key: string; message: string }>();

  const load = useCallback(async () => {
    try {
      setData(await api<{ items: Item[]; last_scan_at: string | null }>("/api/team/radar"));
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Não foi possível carregar o Radar.");
    }
  }, [api]);

  useEffect(() => {
    void load();
  }, [load]);

  async function onImport(it: Item) {
    const key = keyOf(it);
    setBusy(key);
    setRowError(undefined);
    try {
      const r = await api<ImportResult>("/api/team/propositions/import", {
        method: "POST",
        body: JSON.stringify({ chamber: it.chamber, external_id: it.external_id }),
      });
      const q = r.suggestions_unavailable.length ? `?indisponivel=${r.suggestions_unavailable.join(",")}` : "";
      router.push(`/equipe/proposicoes/${r.proposition_id}${q}`);
    } catch (e) {
      setRowError({ key, message: e instanceof ApiError ? e.message : "A importação falhou. Tente de novo." });
      setBusy(undefined);
    }
  }

  async function onDismiss(it: Item) {
    const key = keyOf(it);
    setBusy(key);
    setRowError(undefined);
    try {
      await api(`/api/team/radar/${it.chamber}/${encodeURIComponent(it.external_id)}/dismiss`, { method: "POST" });
      setData((d) => d && { ...d, items: d.items.filter((x) => keyOf(x) !== key) });
      setNotice(`${it.name} dispensada. Volta ao Radar se entrar numa nova pauta do Plenário.`);
    } catch (e) {
      setRowError({ key, message: e instanceof ApiError ? e.message : "Não foi possível dispensar. Tente de novo." });
    } finally {
      setBusy(undefined);
    }
  }

  async function onScan() {
    setBusy("scan");
    setError(undefined);
    try {
      await api("/api/team/radar/scan", { method: "POST" });
      setNotice("Busca pedida. Leva alguns minutos; recarregue a página depois.");
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Não foi possível pedir a busca.");
    } finally {
      setBusy(undefined);
    }
  }

  const items = data?.items.filter((it) =>
    filter === "pauta" ? it.agenda_at : filter === "apresentadas" ? !it.agenda_at : true,
  );

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <h1 className="text-2xl font-bold">Radar</h1>
        <p className="text-stone-700">
          PL, PEC e PLP na pauta do Plenário da Câmara ou do Senado nos próximos 7 dias, ou apresentadas nos últimos 30, que
          ainda não estão no Nelisa. Nada entra sozinho: importe o que vale acompanhar e dispense o resto.
        </p>
        <div className="flex flex-wrap items-center gap-3 text-sm text-stone-600">
          <span>
            {data?.last_scan_at ? `Última busca: ${fmtDateTime(data.last_scan_at)}.` : data ? "Ainda sem busca." : ""} A
            busca automática roda todo dia às 6h.
          </span>
          <button type="button" onClick={onScan} disabled={busy !== undefined} className="btn-ghost min-h-11 px-4">
            {busy === "scan" ? "Pedindo…" : "Buscar agora"}
          </button>
        </div>
      </header>

      <nav aria-label="Filtrar o Radar" className="flex flex-wrap gap-2">
        {FILTERS.map((f) => {
          const active = f.key === filter;
          return (
            <Link
              key={f.label}
              href={f.key ? `/equipe/radar?ver=${f.key}` : "/equipe/radar"}
              aria-current={active ? "page" : undefined}
              className={`${active ? "btn" : "btn-ghost"} min-h-11 px-4`}
            >
              {f.label}
            </Link>
          );
        })}
      </nav>

      <div aria-live="polite" className="space-y-2">
        <FormError message={error} />
        {notice && (
          <p role="status" className="rounded-md border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-900">
            {notice}
          </p>
        )}
      </div>

      {!data && !error && <p role="status" className="text-stone-600">Carregando…</p>}
      {items?.length === 0 && (
        <p className="card text-stone-700">
          {filter === "pauta"
            ? "Nenhuma PL, PEC ou PLP nova na pauta do Plenário."
            : filter === "apresentadas"
              ? "Nenhuma apresentação nova."
              : "Nada novo no Radar: o que está em pauta ou foi apresentado já está no Nelisa ou foi dispensado."}
        </p>
      )}
      {items && items.length > 0 && (
        <ul className="divide-y divide-stone-200 rounded-lg border border-stone-200 bg-white">
          {items.map((it) => {
            const key = keyOf(it);
            return (
              <li key={key} className="space-y-2 px-4 py-4">
                <p className="flex flex-wrap items-center gap-2">
                  <span className="text-lg font-semibold">{it.name}</span>
                  <span className="rounded bg-stone-100 px-1.5 py-0.5 text-xs font-medium text-stone-700">{chamberLabel(it.chamber)}</span>
                  {it.agenda_at ? (
                    <span className="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-900">
                      Em pauta no Plenário em {fmtDateTime(it.agenda_at)}
                    </span>
                  ) : (
                    <span className="rounded bg-sky-100 px-1.5 py-0.5 text-xs font-medium text-sky-900">
                      Apresentada em {fmtDate(it.presented_at)}
                    </span>
                  )}
                </p>
                {it.summary && <p className="line-clamp-3 text-stone-800">{it.summary}</p>}
                {it.source_url && (
                  <p className="text-sm">
                    <a href={it.source_url} className="link" rel="noopener noreferrer">
                      Ver {chamberIn(it.chamber)}
                    </a>
                  </p>
                )}
                <div className="flex flex-wrap gap-2">
                  <button type="button" onClick={() => onImport(it)} disabled={busy !== undefined} className="btn min-h-11 px-4">
                    {busy === key ? "Aguarde…" : "Importar"}
                  </button>
                  <button
                    type="button"
                    onClick={() => onDismiss(it)}
                    disabled={busy !== undefined}
                    aria-label={`Dispensar ${it.name}`}
                    className="btn-ghost min-h-11 px-4"
                  >
                    Dispensar
                  </button>
                </div>
                {rowError?.key === key && <FormError message={rowError.message} />}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

export default function RadarPage() {
  return (
    <RequireTeam>
      {() => (
        <Suspense fallback={<p role="status" className="text-stone-600">Carregando…</p>}>
          <Radar />
        </Suspense>
      )}
    </RequireTeam>
  );
}
