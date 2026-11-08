"use client";

// Árvore de Dispositivos com os Comentários, para a equipe. Na Versão em
// análise (canComment) dá para comentar cada Dispositivo, editar, apagar e
// confirmar Comentários trazidos. Permissões vêm da API; aqui só mostramos.
import { useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError } from "@/lib/team/ui";
import { charCount, fmtDateTime, paragraphs } from "@/lib/labels";

type Provision = components["schemas"]["TeamProvision"];
type Comment = components["schemas"]["TeamComment"];

// Limite do contrato (openapi: CreateCommentRequest).
const COMMENT_MAX = 3000;

const KIND_STYLE: Partial<Record<Provision["kind"], string>> = {
  heading: "pt-3 font-semibold uppercase tracking-wide text-stone-700",
  ementa: "italic",
  justification: "border-t border-stone-200 pt-3",
};

type Props = {
  provisions: Provision[];
  comments?: Comment[];
  canComment?: boolean;
  /** Caminho da Versão: /api/team/propositions/{id}/versions/{versionId} */
  versionPath?: string;
  people?: Record<string, string>;
  reload?: () => Promise<unknown>;
};

export default function CommentedTree({ provisions, comments = [], canComment = false, versionPath, people = {}, reload }: Props) {
  const byAddr = new Map<string, Comment[]>();
  for (const c of comments) byAddr.set(c.provision_address, [...(byAddr.get(c.provision_address) ?? []), c]);
  const [open, setOpen] = useState<string>(); // endereço com o formulário de novo Comentário aberto

  return (
    <ol className="space-y-3">
      {provisions.map((pr) => {
        const cs = byAddr.get(pr.address) ?? [];
        return (
          <li
            key={pr.address}
            id={`d-${pr.address}`}
            className={`[overflow-wrap:anywhere] ${KIND_STYLE[pr.kind] ?? ""}`}
            style={{ paddingLeft: `${Math.min(pr.depth, 4) * 1.25}rem` }}
          >
            {pr.kind !== "heading" && (
              <span className="mr-2 inline-block rounded bg-stone-100 px-1.5 py-0.5 text-xs font-medium not-italic text-stone-700">
                {pr.label}
              </span>
            )}
            <span className="whitespace-pre-line leading-relaxed">{pr.text}</span>

            {cs.length > 0 && (
              <div className="mt-2 space-y-2 not-italic">
                {cs.map((c) => (
                  <CommentBox key={c.id} c={c} canEdit={canComment} versionPath={versionPath} people={people} reload={reload} />
                ))}
              </div>
            )}

            {canComment && pr.commentable && versionPath && (
              open === pr.address ? (
                <NewComment
                  label={pr.label}
                  path={`${versionPath}/comments`}
                  address={pr.address}
                  onDone={async () => {
                    setOpen(undefined);
                    await reload?.();
                  }}
                  onCancel={() => setOpen(undefined)}
                />
              ) : (
                <button
                  type="button"
                  className="btn-ghost mt-1 block min-h-11 px-3 not-italic"
                  aria-label={`Comentar ${pr.label}`}
                  onClick={() => setOpen(pr.address)}
                >
                  Comentar
                </button>
              )
            )}
          </li>
        );
      })}
    </ol>
  );
}

function errMessage(e: unknown, fallback: string) {
  return e instanceof ApiError ? e.message : fallback;
}

function CommentField({ id, value, onChange, label }: { id: string; value: string; onChange: (v: string) => void; label: string }) {
  const count = charCount(value);
  return (
    <div className="space-y-1">
      <label htmlFor={id} className="label">{label}</label>
      <textarea
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={5}
        aria-describedby={`${id}-count`}
        aria-invalid={count > COMMENT_MAX ? true : undefined}
        className="input text-base leading-relaxed"
      />
      <p id={`${id}-count`} className={count > COMMENT_MAX ? "text-sm text-red-800" : "text-sm text-stone-600"}>
        {count.toLocaleString("pt-BR")} de {COMMENT_MAX.toLocaleString("pt-BR")} caracteres
      </p>
    </div>
  );
}

function NewComment({
  label,
  path,
  address,
  onDone,
  onCancel,
}: {
  label: string;
  path: string;
  address: string;
  onDone: () => Promise<void>;
  onCancel: () => void;
}) {
  const { api } = useTeamSession();
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const tooLong = charCount(body) > COMMENT_MAX;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    try {
      await api(path, { method: "POST", body: JSON.stringify({ provision_address: address, body }) });
      await onDone();
    } catch (err) {
      setError(errMessage(err, "Não foi possível salvar o Comentário."));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="mt-2 space-y-2 rounded-md border border-stone-300 p-3 not-italic">
      <CommentField id={`new-${address}`} value={body} onChange={setBody} label={`Comentário sobre ${label}`} />
      <p className="text-sm text-stone-600">Público depois da aprovação. Escreva para quem não é da área: o que muda, para quem.</p>
      <FormError message={error} />
      <div className="flex flex-wrap gap-2">
        <button type="submit" className="btn-lg" disabled={busy || body.trim() === "" || tooLong}>
          {busy ? "Salvando…" : "Salvar Comentário"}
        </button>
        <button type="button" className="btn-ghost min-h-12 px-4" disabled={busy} onClick={onCancel}>
          Cancelar
        </button>
      </div>
    </form>
  );
}

function CommentBox({
  c,
  canEdit,
  versionPath,
  people,
  reload,
}: {
  c: Comment;
  canEdit: boolean;
  versionPath?: string;
  people: Record<string, string>;
  reload?: () => Promise<unknown>;
}) {
  const { api } = useTeamSession();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(c.body);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [busy, setBusy] = useState<"save" | "delete" | "confirm">();
  const [error, setError] = useState<string>();
  const path = versionPath ? `${versionPath}/comments/${encodeURIComponent(c.id)}` : "";
  const names = c.authors.map((id) => people[id] ?? "pessoa removida").join(", ");

  async function call(kind: "save" | "delete" | "confirm", fn: () => Promise<unknown>) {
    setBusy(kind);
    setError(undefined);
    try {
      await fn();
      setEditing(false);
      setConfirmDelete(false);
      await reload?.();
    } catch (e) {
      if (e instanceof ApiError && e.code === "stale_content") {
        setError("Outra pessoa mudou este Comentário. Seu texto não foi gravado; copie-o e recarregue a página para ver o atual.");
      } else {
        setError(errMessage(e, "A ação falhou. Tente de novo."));
      }
    } finally {
      setBusy(undefined);
    }
  }

  return (
    <div className="rounded-md border-l-4 border-sky-700 bg-sky-50 px-3 py-2 text-stone-900">
      <p className="text-sm text-stone-700">
        Comentário · {names} · {fmtDateTime(c.updated_at)}
        {c.needs_confirmation && (
          <span className="ml-2 rounded bg-amber-100 px-1.5 py-0.5 text-xs font-medium text-amber-900">trazido da Versão anterior, a confirmar</span>
        )}
      </p>
      {editing ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void call("save", () => api(path, { method: "PUT", body: JSON.stringify({ body: draft, revision: c.revision }) }));
          }}
          className="mt-2 space-y-2"
        >
          <CommentField id={`edit-${c.id}`} value={draft} onChange={setDraft} label="Editar Comentário" />
          <div className="flex flex-wrap gap-2">
            <button type="submit" className="btn-lg" disabled={busy !== undefined || draft.trim() === "" || charCount(draft) > COMMENT_MAX || draft === c.body}>
              {busy === "save" ? "Salvando…" : "Salvar"}
            </button>
            <button type="button" className="btn-ghost min-h-12 px-4" onClick={() => { setEditing(false); setDraft(c.body); }}>
              Cancelar
            </button>
          </div>
        </form>
      ) : (
        <div className="mt-1 space-y-2 leading-relaxed">
          {paragraphs(c.body).map((p, i) => (
            <p key={i} className="whitespace-pre-line">{p}</p>
          ))}
        </div>
      )}
      <FormError message={error} />
      {canEdit && path && !editing && (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          {c.needs_confirmation && (
            <button
              type="button"
              className="btn-lg"
              disabled={busy !== undefined}
              onClick={() => call("confirm", () => api(`${path}/confirm`, { method: "POST", body: JSON.stringify({ revision: c.revision }) }))}
            >
              {busy === "confirm" ? "Confirmando…" : "Confirmar Comentário"}
            </button>
          )}
          <button type="button" className="btn-ghost min-h-11 px-3" onClick={() => setEditing(true)}>Editar</button>
          {confirmDelete ? (
            <>
              <span className="text-sm">Apagar este Comentário?</span>
              <button
                type="button"
                className="btn-ghost min-h-11 border-red-400 px-3 text-red-800"
                disabled={busy !== undefined}
                onClick={() => call("delete", () => api(path, { method: "DELETE", body: JSON.stringify({ revision: c.revision }) }))}
              >
                {busy === "delete" ? "Apagando…" : "Sim, apagar"}
              </button>
              <button type="button" className="btn-ghost min-h-11 px-3" onClick={() => setConfirmDelete(false)}>Não</button>
            </>
          ) : (
            <button type="button" className="btn-ghost min-h-11 px-3" onClick={() => setConfirmDelete(true)}>Apagar</button>
          )}
        </div>
      )}
    </div>
  );
}
