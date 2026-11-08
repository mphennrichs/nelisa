"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError, RequireTeam } from "@/lib/team/ui";
import { SPHERE_LABEL, fmtDateTime } from "@/lib/labels";

type QueueItem = components["schemas"]["QueueItem"];
type Sphere = components["schemas"]["Sphere"];

const SPHERES: Sphere[] = ["federal", "state", "municipal"];

const REASON_LABEL: Record<QueueItem["reason"], string> = {
  review: "Revisão",
  second_review: "Segunda revisão pendente",
};

function isSphere(s: string | null): s is Sphere {
  return SPHERES.includes(s as Sphere);
}

function Queue() {
  const { api } = useTeamSession();
  const params = useSearchParams();
  const raw = params.get("esfera");
  const sphere = isSphere(raw) ? raw : undefined;
  const [result, setResult] = useState<{ sphere?: Sphere; items: QueueItem[] }>();
  const [error, setError] = useState<string>();

  useEffect(() => {
    let alive = true;
    setError(undefined);
    api<{ items: QueueItem[] }>(`/api/team/queue${sphere ? `?sphere=${sphere}` : ""}`)
      .then((r) => alive && setResult({ sphere, items: r.items }))
      .catch((e) => alive && setError(e instanceof ApiError ? e.message : "Não foi possível carregar a Fila."));
    return () => {
      alive = false;
    };
    // recarrega só quando o filtro muda
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sphere]);

  const items = result && result.sphere === sphere ? result.items : undefined;

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">Fila de revisão</h1>

      <nav aria-label="Filtrar por Esfera" className="flex flex-wrap gap-2">
        {[undefined, ...SPHERES].map((s) => {
          const active = s === sphere;
          return (
            <Link
              key={s ?? "todas"}
              href={s ? `/equipe/fila?esfera=${s}` : "/equipe/fila"}
              aria-current={active ? "page" : undefined}
              className={`${active ? "btn" : "btn-ghost"} min-h-11 px-4`}
            >
              {s ? SPHERE_LABEL[s] : "Todas"}
            </Link>
          );
        })}
      </nav>

      <FormError message={error} />
      {!items && !error && <p role="status" className="text-stone-600">Carregando…</p>}
      {items?.length === 0 && (
        <p className="card text-stone-700">
          Nada esperando revisão{sphere ? ` na Esfera ${SPHERE_LABEL[sphere].toLowerCase()}` : ""}. Proposições enviadas para
          revisão aparecem aqui.
        </p>
      )}
      {items && items.length > 0 && (
        <ul className="divide-y divide-stone-200 rounded-lg border border-stone-200 bg-white">
          {items.map((it) => (
            <li key={`${it.proposition_id}-${it.reason}`} className="space-y-2 px-4 py-4">
              <p className="flex flex-wrap items-center gap-2">
                <Link href={`/equipe/proposicoes/${it.proposition_id}`} className="text-lg font-semibold hover:underline">
                  {it.name}
                </Link>
                <span className="rounded bg-stone-100 px-1.5 py-0.5 text-xs font-medium text-stone-700">{SPHERE_LABEL[it.sphere]}</span>
                <span
                  className={`rounded px-1.5 py-0.5 text-xs font-medium ${
                    it.reason === "second_review" ? "bg-amber-100 text-amber-900" : "bg-sky-100 text-sky-900"
                  }`}
                >
                  {REASON_LABEL[it.reason]}
                </span>
              </p>
              <p className="text-sm text-stone-700">
                Versão {it.version.seq}: {it.version.label} · na Fila desde {fmtDateTime(it.since)}
              </p>
              <p className="text-sm text-stone-700">
                {it.claim
                  ? `Com ${it.claim.holder_name} desde ${fmtDateTime(it.claim.claimed_at)}`
                  : "Ninguém assumiu ainda"}
              </p>
              {it.reason === "review" &&
                (it.missing.length > 0 ? (
                  <div className="text-sm">
                    <p className="font-medium text-stone-700">Falta para aprovar:</p>
                    <ul className="list-disc pl-5 text-stone-700">
                      {it.missing.map((m) => (
                        <li key={m.code}>{m.message}</li>
                      ))}
                    </ul>
                  </div>
                ) : (
                  <p className="text-sm font-medium text-emerald-900">Conteúdo completo: pronto para um Revisor aprovar.</p>
                ))}
              {it.reason === "second_review" && (
                <p className="text-sm text-stone-700">Aprovada sem quatro olhos: falta outra pessoa confirmar a revisão.</p>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export default function QueuePage() {
  return (
    <RequireTeam>
      {() => (
        <Suspense fallback={<p role="status" className="text-stone-600">Carregando…</p>}>
          <Queue />
        </Suspense>
      )}
    </RequireTeam>
  );
}
