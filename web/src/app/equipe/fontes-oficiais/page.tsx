"use client";

import { useCallback, useEffect, useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession, type User } from "@/lib/team/session";
import { FieldError, FormError, RequireTeam } from "@/lib/team/ui";
import { SPHERE_LABEL, fmtDate } from "@/lib/labels";

type Source = components["schemas"]["OfficialSource"];
type Sphere = components["schemas"]["Sphere"];

// Limite do contrato (openapi: POST /api/team/official-sources).
const NAME_MAX = 200;

function OfficialSources({ user }: { user: User }) {
  const { api } = useTeamSession();
  const [sources, setSources] = useState<Source[]>();
  const [suffixes, setSuffixes] = useState<string[]>([]);
  const [loadError, setLoadError] = useState<string>();
  const [domain, setDomain] = useState("");
  const [name, setName] = useState("");
  const [sphere, setSphere] = useState<Sphere>("municipal");
  const [createError, setCreateError] = useState<string>();
  const [creating, setCreating] = useState(false);
  const [confirming, setConfirming] = useState<string>();
  const [busy, setBusy] = useState<string>();
  const [rowError, setRowError] = useState<{ id: string; message: string }>();
  const admin = user.role === "admin";

  const load = useCallback(async () => {
    try {
      const r = await api<{ items: Source[]; default_suffixes: string[] }>("/api/team/official-sources");
      setSources(r.items);
      setSuffixes(r.default_suffixes);
      setLoadError(undefined);
    } catch (err) {
      setLoadError(err instanceof ApiError ? err.message : "Não foi possível carregar as Fontes oficiais.");
    }
  }, [api]);

  useEffect(() => {
    void load();
    // só ao abrir a página
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function onCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setCreating(true);
    setCreateError(undefined);
    try {
      await api("/api/team/official-sources", { method: "POST", body: JSON.stringify({ domain, name, sphere }) });
      setDomain("");
      setName("");
      await load();
    } catch (err) {
      setCreateError(err instanceof ApiError ? err.message : "Não foi possível cadastrar.");
    } finally {
      setCreating(false);
    }
  }

  async function remove(s: Source) {
    setBusy(s.id);
    setRowError(undefined);
    try {
      await api(`/api/team/official-sources/${encodeURIComponent(s.id)}`, { method: "DELETE" });
      setConfirming(undefined);
      await load();
    } catch (err) {
      setRowError({ id: s.id, message: err instanceof ApiError ? err.message : "Não foi possível remover." });
    } finally {
      setBusy(undefined);
    }
  }

  return (
    <div className="space-y-8">
      <div className="space-y-2">
        <h1 className="text-2xl font-bold">Fontes oficiais</h1>
        <p className="text-stone-700">
          Domínios aceitos no link de uma Indicação. {suffixes.length > 0 && <>Domínios terminados em {suffixes.map((s) => `.${s}`).join(" e ")} valem sem cadastro.</>}{" "}
          Um domínio cadastrado vale também para os subdomínios dele. Só o Admin remove.
        </p>
      </div>

      <section aria-labelledby="nova-fonte" className="space-y-3">
        <h2 id="nova-fonte" className="text-lg font-semibold">Cadastrar domínio</h2>
        <form onSubmit={onCreate} className="card space-y-3">
          <div>
            <label htmlFor="src-domain" className="label">Domínio</label>
            <input
              id="src-domain"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              required
              placeholder="camaraexemplo.org"
              autoComplete="off"
              spellCheck={false}
              aria-describedby="src-error"
              className="input min-h-11 text-base"
            />
          </div>
          <div>
            <label htmlFor="src-name" className="label">Casa ou órgão</label>
            <input
              id="src-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              maxLength={NAME_MAX}
              autoComplete="off"
              className="input min-h-11 text-base"
            />
          </div>
          <div>
            <label htmlFor="src-sphere" className="label">Esfera</label>
            <select id="src-sphere" value={sphere} onChange={(e) => setSphere(e.target.value as Sphere)} className="input min-h-11 max-w-xs text-base">
              {(Object.keys(SPHERE_LABEL) as Sphere[]).map((s) => (
                <option key={s} value={s}>{SPHERE_LABEL[s]}</option>
              ))}
            </select>
          </div>
          <FieldError id="src-error" message={createError} />
          <button type="submit" disabled={creating || domain.trim() === "" || name.trim() === ""} className="btn-lg">
            {creating ? "Cadastrando…" : "Cadastrar"}
          </button>
        </form>
      </section>

      <section aria-labelledby="lista-fontes" className="space-y-3">
        <h2 id="lista-fontes" className="text-lg font-semibold">Cadastradas</h2>
        <FormError message={loadError} />
        {!sources && !loadError && <p role="status" className="text-stone-600">Carregando…</p>}
        {sources && sources.length === 0 && <p className="card text-stone-700">Nenhum domínio cadastrado além dos padrões.</p>}
        {sources && sources.length > 0 && (
          <ul className="divide-y divide-stone-200 rounded-lg border border-stone-200 bg-white">
            {sources.map((s) => (
              <li key={s.id} className="space-y-2 px-4 py-3">
                <div className="flex flex-wrap items-center gap-3">
                  <div className="min-w-0 flex-1 basis-56">
                    <p className="font-mono font-medium [overflow-wrap:anywhere]">{s.domain}</p>
                    <p className="text-sm text-stone-700">
                      {s.name} · {SPHERE_LABEL[s.sphere]} · desde {fmtDate(s.created_at)}
                    </p>
                  </div>
                  {admin &&
                    (confirming === s.id ? (
                      <>
                        <button type="button" disabled={busy === s.id} onClick={() => remove(s)} aria-label={`Confirmar remoção de ${s.domain}`} className="btn min-h-11 px-4">
                          {busy === s.id ? "Removendo…" : "Confirmar remoção"}
                        </button>
                        <button type="button" onClick={() => setConfirming(undefined)} className="btn-ghost min-h-11 px-4">
                          Cancelar
                        </button>
                      </>
                    ) : (
                      <button type="button" onClick={() => setConfirming(s.id)} aria-label={`Remover ${s.domain}`} className="btn-ghost min-h-11 px-4">
                        Remover
                      </button>
                    ))}
                </div>
                {rowError?.id === s.id && <FormError message={rowError.message} />}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

export default function OfficialSourcesPage() {
  return <RequireTeam>{(user) => <OfficialSources user={user} />}</RequireTeam>;
}
