"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useId, useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError, RequireTeam } from "@/lib/team/ui";
import {
  CHAMBER_LABEL,
  CONTRIBUTION_KIND_LABEL,
  PROGRESS_EVENT_LABEL,
  REJECT_REASON_LABEL,
  SPHERE_LABEL,
  charCount,
  fmtDate,
  fmtDateTime,
} from "@/lib/labels";

type Contribution = components["schemas"]["TeamContribution"];
type Sphere = components["schemas"]["Sphere"];
type Reason = components["schemas"]["RejectReason"];
type Chamber = components["schemas"]["Chamber"];

const SPHERES: Sphere[] = ["federal", "state", "municipal"];
const REASONS = Object.keys(REJECT_REASON_LABEL) as Reason[];
const CHAMBERS = Object.keys(CHAMBER_LABEL) as Chamber[];
const MAX_TEXT = 1000;

const STATE_BADGE: Record<Contribution["state"], { label: string; className: string }> = {
  pending: { label: "Pendente", className: "bg-violet-100 text-violet-900" },
  accepted: { label: "Aceita", className: "bg-emerald-100 text-emerald-900" },
  rejected: { label: "Rejeitada", className: "bg-stone-200 text-stone-800" },
};

/** Link mandado pelo leitor: o domínio em destaque, o endereço inteiro embaixo. O Nelisa nunca o abre. */
function ReaderLink({ url, host }: { url: string; host: string }) {
  return (
    <span className="block">
      <a href={url} className="link font-semibold [overflow-wrap:anywhere]" rel="noopener noreferrer nofollow">
        {host || url}
      </a>
      <span className="block font-mono text-xs text-stone-600 [overflow-wrap:anywhere]">{url}</span>
    </span>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-sm font-medium text-stone-600">{label}</dt>
      <dd className="mt-0.5">{children}</dd>
    </div>
  );
}

function Moderation({ id }: { id: string }) {
  const { api } = useTeamSession();
  const router = useRouter();
  const ids = useId();
  const [c, setC] = useState<Contribution>();
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState<string>();
  const [busy, setBusy] = useState<string>();
  const [description, setDescription] = useState("");
  const [rejectNote, setRejectNote] = useState("");
  const [acceptNote, setAcceptNote] = useState("");

  useEffect(() => {
    api<Contribution>(`/api/team/contributions/${encodeURIComponent(id)}`)
      .then((d) => {
        setC(d);
        if (d.event_type) setDescription(PROGRESS_EVENT_LABEL[d.event_type]);
      })
      .catch((e) => setError(e instanceof ApiError ? e.message : "Não foi possível carregar a Contribuição."));
  }, [api, id]);

  async function send(key: string, path: string, method: string, body: unknown, done: string) {
    setBusy(key);
    setError(undefined);
    setNotice(undefined);
    try {
      const d = await api<Contribution>(path, { method, body: JSON.stringify(body) });
      setC(d);
      setNotice(done);
      return d;
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "A ação falhou. Tente de novo.");
      return undefined;
    } finally {
      setBusy(undefined);
    }
  }

  if (!c) {
    return error ? <FormError message={error} /> : <p role="status" className="text-stone-600">Carregando…</p>;
  }

  const base = `/api/team/contributions/${encodeURIComponent(c.id)}`;
  const pending = c.state === "pending";
  const badge = STATE_BADGE[c.state];

  async function onAccept(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const body: Record<string, unknown> = { internal_note: acceptNote };
    if (c!.kind === "progress_update") {
      body.event = {
        chamber: fd.get("chamber"),
        date: fd.get("date"),
        description,
        source_url: fd.get("source_url"),
      };
    }
    const done =
      c!.kind === "progress_update"
        ? "Aceita. O evento já aparece na Tramitação pública, com o link da fonte."
        : c!.kind === "source_request"
          ? "Aceita. O domínio agora é Fonte oficial."
          : "Aceita. Agora importe a Proposição.";
    const d = await send("accept", `${base}/accept`, "POST", body, done);
    if (d && d.kind === "indication") router.push(`/equipe/proposicoes/importar?contribuicao=${d.id}`);
  }

  async function onReject(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const reason = new FormData(e.currentTarget).get("reason");
    if (!reason) {
      setError("Escolha o motivo da rejeição.");
      return;
    }
    await send("reject", `${base}/reject`, "POST", { reason, internal_note: rejectNote }, "Rejeitada. O leitor vê o motivo ao consultar o código.");
  }

  async function onSphere(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const sphere = new FormData(e.currentTarget).get("sphere");
    await send("sphere", `${base}/sphere`, "PUT", { sphere }, "Esfera corrigida.");
  }

  return (
    <div className="space-y-8">
      <p>
        <Link href="/equipe/fila" className="link">← Fila de revisão</Link>
      </p>

      <header className="space-y-2">
        <p className="flex flex-wrap items-center gap-2 text-sm">
          <span className={`rounded px-1.5 py-0.5 font-medium ${badge.className}`}>{badge.label}</span>
          <span className="rounded bg-stone-100 px-1.5 py-0.5 font-medium text-stone-700">{SPHERE_LABEL[c.sphere]}</span>
          <span className="text-stone-600">Enviada em {fmtDateTime(c.submitted_at)}</span>
        </p>
        <h1 className="text-2xl font-bold">{CONTRIBUTION_KIND_LABEL[c.kind]}</h1>
        {c.proposition && (
          <p className="text-stone-800">
            Proposição:{" "}
            <Link href={`/equipe/proposicoes/${c.proposition.id}`} className="link font-medium">
              {c.proposition.name}
            </Link>
          </p>
        )}
      </header>

      <div aria-live="polite" className="space-y-2">
        <FormError message={error} />
        {notice && (
          <p role="status" className="rounded-md border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-900">
            {notice}
          </p>
        )}
      </div>

      <section aria-labelledby={`${ids}-sent`} className="card space-y-4">
        <div>
          <h2 id={`${ids}-sent`} className="text-lg font-semibold">O que o leitor mandou</h2>
          <p className="text-sm text-stone-600">
            Texto como veio, sem formatação. O Nelisa não abre o link: confira você, e só se confiar no domínio.
          </p>
        </div>
        <dl className="space-y-3">
          {c.kind === "progress_update" && c.event_type && (
            <Field label="Evento">
              {PROGRESS_EVENT_LABEL[c.event_type]}
              {c.event_date && ` em ${fmtDate(`${c.event_date}T12:00:00-03:00`)}`}
            </Field>
          )}
          {c.kind === "source_request" && (
            <>
              <Field label="Domínio pedido">
                <span className="font-mono font-semibold [overflow-wrap:anywhere]">{c.domain}</span>
              </Field>
              <Field label="Casa ou órgão">{c.source_name}</Field>
            </>
          )}
          <Field label={c.kind === "indication" ? "Link da Proposição" : c.kind === "source_request" ? "Evidência" : "Fonte"}>
            <ReaderLink url={c.link} host={c.link_host} />
          </Field>
          <Field label="Observação">
            {c.note ? (
              <p className="whitespace-pre-wrap [overflow-wrap:anywhere]">{c.note}</p>
            ) : (
              <p className="text-stone-600">Sem observação.</p>
            )}
          </Field>
        </dl>
      </section>

      {pending ? (
        <>
          {c.kind !== "progress_update" && (
            <section aria-labelledby={`${ids}-sphere`} className="card space-y-3">
              <h2 id={`${ids}-sphere`} className="text-lg font-semibold">Esfera</h2>
              <p className="text-sm text-stone-700">O leitor escolheu a Esfera; corrija se estiver errada. Ela manda na ordem da Fila.</p>
              <form onSubmit={onSphere} className="flex flex-wrap items-end gap-3">
                <div className="min-w-0 basis-48">
                  <label htmlFor={`${ids}-sphere-sel`} className="label">Esfera</label>
                  <select id={`${ids}-sphere-sel`} name="sphere" defaultValue={c.sphere} className="input min-h-11 text-base">
                    {SPHERES.map((s) => (
                      <option key={s} value={s}>{SPHERE_LABEL[s]}</option>
                    ))}
                  </select>
                </div>
                <button type="submit" disabled={busy !== undefined} className="btn-ghost min-h-11 px-4">
                  {busy === "sphere" ? "Salvando…" : "Corrigir Esfera"}
                </button>
              </form>
            </section>
          )}

          <section aria-labelledby={`${ids}-accept`} className="card space-y-4">
            <h2 id={`${ids}-accept`} className="text-lg font-semibold">Aceitar</h2>
            <form onSubmit={onAccept} className="space-y-4">
              {c.kind === "progress_update" && (
                <fieldset className="space-y-4">
                  <legend className="text-sm text-stone-700">
                    Revise o evento como ele vai aparecer na Tramitação pública, marcado como informado por leitor e com o link
                    da fonte.
                  </legend>
                  <div className="grid gap-4 sm:grid-cols-2">
                    <div>
                      <label htmlFor={`${ids}-chamber`} className="label">Casa</label>
                      <select id={`${ids}-chamber`} name="chamber" required defaultValue="" className="input min-h-11 text-base">
                        <option value="" disabled>Escolha a Casa</option>
                        {CHAMBERS.map((ch) => (
                          <option key={ch} value={ch}>{CHAMBER_LABEL[ch]}</option>
                        ))}
                      </select>
                    </div>
                    <div>
                      <label htmlFor={`${ids}-date`} className="label">Data</label>
                      <input id={`${ids}-date`} name="date" type="date" required defaultValue={c.event_date ?? ""} className="input min-h-11 text-base" />
                    </div>
                  </div>
                  <div>
                    <label htmlFor={`${ids}-desc`} className="label">Texto do evento</label>
                    <textarea
                      id={`${ids}-desc`}
                      required
                      rows={3}
                      maxLength={MAX_TEXT}
                      value={description}
                      onChange={(e) => setDescription(e.target.value)}
                      aria-describedby={`${ids}-desc-hint`}
                      className="input text-base"
                    />
                    <p id={`${ids}-desc-hint`} className="mt-1 text-sm text-stone-600">
                      Escrito pela equipe, em linguagem neutra. {charCount(description).toLocaleString("pt-BR")} de{" "}
                      {MAX_TEXT.toLocaleString("pt-BR")} caracteres.
                    </p>
                  </div>
                  <div>
                    <label htmlFor={`${ids}-src`} className="label">Link da fonte</label>
                    <input id={`${ids}-src`} name="source_url" type="url" required defaultValue={c.link} className="input min-h-11 text-base" />
                  </div>
                </fieldset>
              )}
              {c.kind === "source_request" && (
                <p className="text-stone-800">
                  Cadastra <span className="font-mono font-semibold">{c.domain}</span> ({c.source_name}, {SPHERE_LABEL[c.sphere].toLowerCase()})
                  como Fonte oficial. Confira na evidência que o domínio é mesmo da Casa ou órgão.
                </p>
              )}
              {c.kind === "indication" && (
                <p className="text-stone-800">
                  Depois de aceitar, você vai para a Importação. Se a Proposição já está no Nelisa, rejeite como &ldquo;já estava
                  cadastrado&rdquo;.
                </p>
              )}
              <div>
                <label htmlFor={`${ids}-anote`} className="label">Nota interna (opcional)</label>
                <textarea
                  id={`${ids}-anote`}
                  rows={2}
                  maxLength={MAX_TEXT}
                  value={acceptNote}
                  onChange={(e) => setAcceptNote(e.target.value)}
                  className="input text-base"
                />
                <p className="mt-1 text-sm text-stone-600">Só a equipe vê.</p>
              </div>
              <button type="submit" disabled={busy !== undefined} className="btn-lg">
                {busy === "accept"
                  ? "Aceitando…"
                  : c.kind === "progress_update"
                    ? "Aceitar e gravar na Tramitação"
                    : c.kind === "source_request"
                      ? "Aceitar e cadastrar Fonte oficial"
                      : "Aceitar e importar a Proposição"}
              </button>
            </form>
          </section>

          <section aria-labelledby={`${ids}-reject`} className="card space-y-4">
            <h2 id={`${ids}-reject`} className="text-lg font-semibold">Rejeitar</h2>
            <form onSubmit={onReject} className="space-y-4">
              <fieldset>
                <legend className="mb-2 text-sm text-stone-700">Motivo (o leitor vê este texto ao consultar o código)</legend>
                <div className="space-y-1">
                  {REASONS.map((r) => (
                    <label key={r} className="flex min-h-11 cursor-pointer items-center gap-3">
                      <input type="radio" name="reason" value={r} className="size-5 shrink-0" />
                      <span>{REJECT_REASON_LABEL[r]}</span>
                    </label>
                  ))}
                </div>
              </fieldset>
              <div>
                <label htmlFor={`${ids}-rnote`} className="label">Nota interna (opcional)</label>
                <textarea
                  id={`${ids}-rnote`}
                  rows={2}
                  maxLength={MAX_TEXT}
                  value={rejectNote}
                  onChange={(e) => setRejectNote(e.target.value)}
                  className="input text-base"
                />
                <p className="mt-1 text-sm text-stone-600">Nunca é pública.</p>
              </div>
              <button type="submit" disabled={busy !== undefined} className="btn-ghost min-h-11 px-4">
                {busy === "reject" ? "Rejeitando…" : "Rejeitar"}
              </button>
            </form>
          </section>
        </>
      ) : (
        <section aria-labelledby={`${ids}-decision`} className="card space-y-3">
          <h2 id={`${ids}-decision`} className="text-lg font-semibold">Decisão</h2>
          <dl className="space-y-3">
            <Field label="Decidida em">{fmtDateTime(c.decided_at)}</Field>
            {c.public_reason && <Field label="Motivo público">{REJECT_REASON_LABEL[c.public_reason]}</Field>}
            {c.internal_note && (
              <Field label="Nota interna">
                <p className="whitespace-pre-wrap [overflow-wrap:anywhere]">{c.internal_note}</p>
              </Field>
            )}
          </dl>
          {c.kind === "indication" && c.state === "accepted" &&
            (c.created_proposition ? (
              <p>
                Proposição criada:{" "}
                <Link href={`/equipe/proposicoes/${c.created_proposition.id}`} className="link font-medium">
                  {c.created_proposition.name}
                </Link>
              </p>
            ) : (
              <p>
                Ainda sem Proposição.{" "}
                <Link href={`/equipe/proposicoes/importar?contribuicao=${c.id}`} className="link font-medium">
                  Importar a Proposição
                </Link>
              </p>
            ))}
          {c.kind === "progress_update" && c.state === "accepted" && c.proposition && (
            <p className="text-sm text-stone-700">
              O evento manual fica na Tramitação de{" "}
              <Link href={`/equipe/proposicoes/${c.proposition.id}`} className="link">{c.proposition.name}</Link>; lá dá para
              apagá-lo quando o oficial chegar.
            </p>
          )}
        </section>
      )}
    </div>
  );
}

export default function Page() {
  const { id } = useParams<{ id: string }>();
  return <RequireTeam>{() => <Moderation id={id} />}</RequireTeam>;
}
