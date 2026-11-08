"use client";

import { useCallback, useEffect, useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FieldError, FormError, RequireTeam } from "@/lib/team/ui";
import { charCount } from "@/lib/labels";

type Topic = components["schemas"]["Topic"];

// Limite do contrato (openapi: Topic.name maxLength).
const NAME_MAX = 60;

function TopicsAdmin() {
  const { api } = useTeamSession();
  const [topics, setTopics] = useState<Topic[]>();
  const [loadError, setLoadError] = useState<string>();
  const [createError, setCreateError] = useState<string>();
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState("");
  // Tema com a renomeação aberta e o nome sendo digitado
  const [editing, setEditing] = useState<{ id: string; name: string; error?: string }>();
  const [rowError, setRowError] = useState<{ id: string; message: string }>();
  const [busy, setBusy] = useState<string>();

  const load = useCallback(async () => {
    try {
      const r = await api<{ topics: Topic[] }>("/api/team/topics");
      setTopics(r.topics);
      setLoadError(undefined);
    } catch (err) {
      setLoadError(err instanceof ApiError ? err.message : "Não foi possível carregar os Temas.");
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
      await api("/api/team/topics", { method: "POST", body: JSON.stringify({ name: newName }) });
      setNewName("");
      await load();
    } catch (err) {
      setCreateError(err instanceof ApiError ? err.message : "Não foi possível criar o Tema.");
    } finally {
      setCreating(false);
    }
  }

  async function patch(t: Topic, body: { name?: string; active?: boolean }) {
    setBusy(t.id);
    setRowError(undefined);
    try {
      await api(`/api/team/topics/${encodeURIComponent(t.id)}`, { method: "PATCH", body: JSON.stringify(body) });
      setEditing(undefined);
      await load();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : "Não foi possível alterar o Tema.";
      if (body.name !== undefined) setEditing({ id: t.id, name: body.name, error: message });
      else setRowError({ id: t.id, message });
    } finally {
      setBusy(undefined);
    }
  }

  const active = topics?.filter((t) => t.active) ?? [];
  const inactive = topics?.filter((t) => !t.active) ?? [];
  const newCount = charCount(newName);

  function row(t: Topic) {
    const isEditing = editing?.id === t.id;
    return (
      <li key={t.id} className={`space-y-2 px-4 py-3 ${t.active ? "" : "bg-stone-50"}`}>
        {isEditing ? (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void patch(t, { name: editing.name });
            }}
            className="space-y-2"
          >
            <label htmlFor={`name-${t.id}`} className="label">Novo nome para {t.name}</label>
            <input
              id={`name-${t.id}`}
              value={editing.name}
              onChange={(e) => setEditing({ id: t.id, name: e.target.value })}
              required
              autoFocus
              aria-invalid={editing.error ? true : undefined}
              aria-describedby={editing.error ? `name-${t.id}-error` : undefined}
              className="input min-h-11 text-base"
            />
            <FieldError id={`name-${t.id}-error`} message={editing.error} />
            <div className="flex flex-wrap gap-2">
              <button type="submit" disabled={busy === t.id || charCount(editing.name) === 0} className="btn min-h-11 px-4">
                {busy === t.id ? "Salvando…" : "Salvar nome"}
              </button>
              <button type="button" onClick={() => setEditing(undefined)} className="btn-ghost min-h-11 px-4">
                Cancelar
              </button>
            </div>
          </form>
        ) : (
          <div className="flex flex-wrap items-center gap-3">
            <p className={`min-w-0 flex-1 font-medium [overflow-wrap:anywhere] ${t.active ? "" : "text-stone-600"}`}>
              {t.name}
              {!t.active && <span className="ml-2 text-sm font-normal">(desativado)</span>}
            </p>
            <button type="button" onClick={() => setEditing({ id: t.id, name: t.name })} className="btn-ghost min-h-11 px-4">
              Renomear
            </button>
            <button
              type="button"
              disabled={busy === t.id}
              onClick={() => patch(t, { active: !t.active })}
              className="btn-ghost min-h-11 px-4"
            >
              {busy === t.id ? "Salvando…" : t.active ? "Desativar" : "Reativar"}
            </button>
          </div>
        )}
        {rowError?.id === t.id && <FormError message={rowError.message} />}
      </li>
    );
  }

  return (
    <div className="space-y-8">
      <div className="space-y-2">
        <h1 className="text-2xl font-bold">Temas</h1>
        <p className="text-stone-700">
          Lista fechada usada para classificar as Proposições (até 3 por Versão). Tema desativado sai da escolha, mas continua
          nas Versões que já o usam.
        </p>
      </div>

      <section aria-labelledby="novo-tema" className="space-y-3">
        <h2 id="novo-tema" className="text-lg font-semibold">Criar Tema</h2>
        <form onSubmit={onCreate} className="card space-y-3">
          <div>
            <label htmlFor="new-topic" className="label">Nome do Tema</label>
            <input
              id="new-topic"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              required
              autoComplete="off"
              aria-invalid={createError || newCount > NAME_MAX ? true : undefined}
              aria-describedby="new-topic-count new-topic-error"
              className="input min-h-11 text-base"
            />
            <p id="new-topic-count" className={`mt-1 text-sm ${newCount > NAME_MAX ? "text-red-800" : "text-stone-600"}`}>
              {newCount} de {NAME_MAX} caracteres
            </p>
            <FieldError id="new-topic-error" message={createError} />
          </div>
          <button type="submit" disabled={creating || newCount === 0 || newCount > NAME_MAX} className="btn-lg">
            {creating ? "Criando…" : "Criar Tema"}
          </button>
        </form>
      </section>

      <section aria-labelledby="lista-temas" className="space-y-3">
        <h2 id="lista-temas" className="text-lg font-semibold">Temas ativos</h2>
        <FormError message={loadError} />
        {!topics && !loadError && <p role="status" className="text-stone-600">Carregando…</p>}
        {topics && active.length === 0 && <p className="card text-stone-700">Nenhum Tema ativo. Crie o primeiro acima.</p>}
        {active.length > 0 && <ul className="divide-y divide-stone-200 rounded-lg border border-stone-200 bg-white">{active.map(row)}</ul>}
      </section>

      {inactive.length > 0 && (
        <section aria-labelledby="desativados" className="space-y-3">
          <h2 id="desativados" className="text-lg font-semibold">Desativados</h2>
          <ul className="divide-y divide-stone-200 rounded-lg border border-stone-200 bg-white">{inactive.map(row)}</ul>
        </section>
      )}
    </div>
  );
}

export default function TopicsPage() {
  return <RequireTeam admin>{() => <TopicsAdmin />}</RequireTeam>;
}
