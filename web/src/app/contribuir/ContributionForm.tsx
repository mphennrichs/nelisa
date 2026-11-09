"use client";

import Link from "next/link";
import { useId, useRef, useState } from "react";
import type { components } from "@/lib/api/schema";
import { CONTRIBUTION_KIND_LABEL, PROGRESS_EVENT_LABEL, SPHERE_LABEL, charCount } from "@/lib/labels";

type Kind = components["schemas"]["ContributionKind"];
type Sphere = components["schemas"]["Sphere"];
type EventType = components["schemas"]["ProgressEventType"];

// Limites do contrato (openapi: POST /api/public/contributions).
const NOTE_MAX = 1000;
const NAME_MAX = 200;

// Campo que recebe cada erro da API; o resto vai para o topo do formulário.
const FIELD_OF: Record<string, string> = {
  invalid_link: "link",
  not_official_source: "link",
  invalid_domain: "domain",
  official_source_exists: "domain",
  invalid_source_name: "source_name",
  invalid_sphere: "sphere",
  invalid_event_type: "event_type",
  invalid_event_date: "event_date",
  note_too_long: "note",
};

type Proposition = { slug: string; name: string };

export default function ContributionForm({ proposition }: { proposition?: Proposition }) {
  const kinds: Kind[] = proposition ? ["progress_update", "indication", "source_request"] : ["indication", "source_request"];
  const [kind, setKind] = useState<Kind>(kinds[0]);
  const [link, setLink] = useState("");
  const [sphere, setSphere] = useState<Sphere | "">("");
  const [eventType, setEventType] = useState<EventType | "">("");
  const [eventDate, setEventDate] = useState("");
  const [domain, setDomain] = useState("");
  const [sourceName, setSourceName] = useState("");
  const [note, setNote] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string>();
  const [sending, setSending] = useState(false);
  const [code, setCode] = useState<string>();
  const [copied, setCopied] = useState(false);
  const resultRef = useRef<HTMLDivElement>(null);
  const id = useId();

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSending(true);
    setErrors({});
    setFormError(undefined);
    const body: Record<string, string> = { kind, note };
    if (kind === "indication") Object.assign(body, { link, sphere });
    if (kind === "progress_update")
      Object.assign(body, { link, proposition_slug: proposition?.slug ?? "", event_type: eventType, event_date: eventDate });
    if (kind === "source_request") Object.assign(body, { domain, link, source_name: sourceName, sphere });
    try {
      const res = await fetch("/api/public/contributions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const data = await res.json().catch(() => null);
      if (res.status === 201 && data?.tracking_code) {
        setCode(data.tracking_code);
        // Leva o foco para o código: é a única coisa que o Leitor precisa guardar.
        requestAnimationFrame(() => resultRef.current?.focus());
        return;
      }
      const err = data?.error as { code?: string; message?: string } | undefined;
      const field = err?.code ? FIELD_OF[err.code] : undefined;
      const message = err?.message ?? "Não foi possível enviar. Tente de novo.";
      if (field) setErrors({ [field]: message });
      else setFormError(message);
    } catch {
      setFormError("Sem conexão com o Nelisa. Confira a internet e tente de novo.");
    } finally {
      setSending(false);
    }
  }

  if (code) {
    return (
      <div ref={resultRef} tabIndex={-1} className="card space-y-4 outline-none" aria-labelledby={`${id}-ok`}>
        <h2 id={`${id}-ok`} className="text-xl font-semibold">Contribuição recebida</h2>
        <p className="text-stone-700">
          Guarde o código abaixo para acompanhar a avaliação. Ele <strong>não pode ser recuperado</strong>: o Nelisa não
          guarda e-mail nem nada que identifique você.
        </p>
        <p className="font-mono text-3xl font-bold tracking-wider" aria-label={`Código de acompanhamento ${code.split("").join(" ")}`}>
          {code}
        </p>
        <div className="flex flex-wrap gap-3">
          <button
            type="button"
            className="btn-lg"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(code);
                setCopied(true);
              } catch {
                setCopied(false);
              }
            }}
          >
            Copiar código
          </button>
          <Link href="/acompanhar" className="btn-ghost min-h-12 px-5 text-base">
            Acompanhar contribuição
          </Link>
        </div>
        <p role="status" className="text-sm text-stone-700">{copied ? "Código copiado." : ""}</p>
      </div>
    );
  }

  const noteCount = charCount(note);
  const err = (f: string) => errors[f];
  const described = (f: string, hint?: boolean) =>
    [hint ? `${id}-${f}-hint` : "", err(f) ? `${id}-${f}-error` : ""].filter(Boolean).join(" ") || undefined;
  const fieldError = (f: string) =>
    err(f) ? (
      <p id={`${id}-${f}-error`} className="mt-1 text-sm text-red-800">
        {err(f)}
      </p>
    ) : null;

  const linkLabel =
    kind === "indication" ? "Link da Proposição" : kind === "progress_update" ? "Link da fonte" : "Link da evidência";
  const linkHint =
    kind === "indication"
      ? "Endereço da página da Proposição no site oficial da Casa (domínio .leg.br, .gov.br ou cadastrado pela equipe)."
      : kind === "progress_update"
        ? "Onde a informação foi publicada: site da Casa, Diário Oficial ou notícia."
        : "Página que mostra que o domínio pertence à Casa ou órgão (por exemplo, o portal de transparência).";

  return (
    <form onSubmit={onSubmit} noValidate className="space-y-6">
      {formError && (
        <div role="alert" className="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-red-800">
          {formError}
        </div>
      )}

      <fieldset className="space-y-2">
        <legend className="mb-2 text-lg font-semibold">O que você quer mandar?</legend>
        {kinds.map((k) => (
          <label key={k} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border border-stone-300 bg-white p-3 has-[:checked]:border-stone-900">
            <input
              type="radio"
              name="kind"
              value={k}
              checked={kind === k}
              onChange={() => {
                setKind(k);
                setErrors({});
                setFormError(undefined);
              }}
              className="mt-1 size-5 shrink-0"
            />
            <span>
              <span className="block font-medium">
                {CONTRIBUTION_KIND_LABEL[k]}
                {k === "progress_update" && proposition ? ` da ${proposition.name}` : ""}
              </span>
              <span className="block text-sm text-stone-700">
                {k === "indication" && "Uma Proposição que o Nelisa ainda não acompanha."}
                {k === "progress_update" && "Um evento que ainda não aparece na Tramitação, como votação marcada ou adiada."}
                {k === "source_request" && "O site de uma Casa legislativa ou órgão que ainda não é aceito como Fonte oficial."}
              </span>
            </span>
          </label>
        ))}
        {!proposition && (
          <p className="text-sm text-stone-700">
            Para informar um evento de Tramitação, abra a página da Proposição e use o botão Contribuir.
          </p>
        )}
      </fieldset>

      {kind === "progress_update" && (
        <>
          <div>
            <label htmlFor={`${id}-event_type`} className="mb-1 block font-medium">Tipo de evento</label>
            <select
              id={`${id}-event_type`}
              value={eventType}
              onChange={(e) => setEventType(e.target.value as EventType)}
              required
              aria-invalid={err("event_type") ? true : undefined}
              aria-describedby={described("event_type")}
              className="input min-h-11 text-base"
            >
              <option value="">Escolha…</option>
              {(Object.keys(PROGRESS_EVENT_LABEL) as EventType[]).map((t) => (
                <option key={t} value={t}>{PROGRESS_EVENT_LABEL[t]}</option>
              ))}
            </select>
            {fieldError("event_type")}
          </div>
          <div>
            <label htmlFor={`${id}-event_date`} className="mb-1 block font-medium">Data do evento</label>
            <input
              id={`${id}-event_date`}
              type="date"
              value={eventDate}
              onChange={(e) => setEventDate(e.target.value)}
              required
              aria-invalid={err("event_date") ? true : undefined}
              aria-describedby={described("event_date")}
              className="input min-h-11 max-w-xs text-base"
            />
            {fieldError("event_date")}
          </div>
        </>
      )}

      {kind === "source_request" && (
        <>
          <div>
            <label htmlFor={`${id}-domain`} className="mb-1 block font-medium">Domínio</label>
            <p id={`${id}-domain-hint`} className="mb-1 text-sm text-stone-700">Só o domínio, sem https://. Ex.: camaraexemplo.org.br</p>
            <input
              id={`${id}-domain`}
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              required
              autoComplete="off"
              spellCheck={false}
              inputMode="url"
              aria-invalid={err("domain") ? true : undefined}
              aria-describedby={described("domain", true)}
              className="input min-h-11 text-base"
            />
            {fieldError("domain")}
          </div>
          <div>
            <label htmlFor={`${id}-source_name`} className="mb-1 block font-medium">Nome da Casa ou órgão</label>
            <input
              id={`${id}-source_name`}
              value={sourceName}
              onChange={(e) => setSourceName(e.target.value)}
              required
              maxLength={NAME_MAX}
              autoComplete="off"
              aria-invalid={err("source_name") ? true : undefined}
              aria-describedby={described("source_name")}
              className="input min-h-11 text-base"
            />
            {fieldError("source_name")}
          </div>
        </>
      )}

      <div>
        <label htmlFor={`${id}-link`} className="mb-1 block font-medium">{linkLabel}</label>
        <p id={`${id}-link-hint`} className="mb-1 text-sm text-stone-700">{linkHint} O Nelisa não abre o link: a equipe confere.</p>
        <input
          id={`${id}-link`}
          type="url"
          value={link}
          onChange={(e) => setLink(e.target.value)}
          required
          placeholder="https://"
          autoComplete="off"
          spellCheck={false}
          aria-invalid={err("link") ? true : undefined}
          aria-describedby={described("link", true)}
          className="input min-h-11 text-base"
        />
        {fieldError("link")}
      </div>

      {kind !== "progress_update" && (
        <fieldset aria-describedby={described("sphere")}>
          <legend className="mb-1 font-medium">Esfera</legend>
          <div className="flex flex-wrap gap-2">
            {(Object.keys(SPHERE_LABEL) as Sphere[]).map((s) => (
              <label key={s} className="flex min-h-11 cursor-pointer items-center gap-2 rounded-md border border-stone-300 bg-white px-3 has-[:checked]:border-stone-900">
                <input type="radio" name="sphere" value={s} checked={sphere === s} onChange={() => setSphere(s)} className="size-5 shrink-0" />
                {SPHERE_LABEL[s]}
              </label>
            ))}
          </div>
          {fieldError("sphere")}
        </fieldset>
      )}

      <div>
        <label htmlFor={`${id}-note`} className="mb-1 block font-medium">Observação (opcional)</label>
        <p id={`${id}-note-hint`} className="mb-1 text-sm text-stone-700">
          Não escreva dados pessoais (nome, e-mail, telefone). A observação só é lida pela equipe.
        </p>
        <textarea
          id={`${id}-note`}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          rows={4}
          aria-invalid={err("note") || noteCount > NOTE_MAX ? true : undefined}
          aria-describedby={[described("note", true), `${id}-note-count`].join(" ")}
          className="input text-base"
        />
        <p id={`${id}-note-count`} className={`mt-1 text-sm ${noteCount > NOTE_MAX ? "text-red-800" : "text-stone-600"}`}>
          {noteCount.toLocaleString("pt-BR")} de {NOTE_MAX.toLocaleString("pt-BR")} caracteres
        </p>
        {fieldError("note")}
      </div>

      <button type="submit" disabled={sending || noteCount > NOTE_MAX} className="btn-lg">
        {sending ? "Enviando…" : "Enviar contribuição"}
      </button>
    </form>
  );
}
