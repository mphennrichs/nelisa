import Link from "next/link";
import { chambersOf, db, publicPropositions, TOPICS } from "@/lib/store";
import { PUBLIC_STATE_LABEL, SPHERE_LABEL, fmtDate } from "@/lib/format";
import { Badge } from "@/components/ui";

type Search = Promise<Record<string, string | undefined>>;

export default async function Home({ searchParams }: { searchParams: Search }) {
  const f = await searchParams;
  const propositions = publicPropositions(f);
  const ufs = [...new Set(db.chambers.map((c) => c.uf).filter(Boolean))] as string[];

  return (
    <div className="space-y-6">
      <section>
        <h1 className="text-2xl font-bold">Proposições legislativas, explicadas</h1>
        <p className="text-stone-600">Federais, estaduais e municipais, com resumo e comentários revisados pela equipe.</p>
      </section>

      <form className="card grid gap-3 sm:grid-cols-6">
        <div className="sm:col-span-6">
          <input name="q" defaultValue={f.q} placeholder="Buscar por número, ementa ou resumo" className="input" />
        </div>
        <Select name="sphere" value={f.sphere} label="Esfera" options={Object.entries(SPHERE_LABEL)} />
        <Select name="uf" value={f.uf} label="UF" options={ufs.map((u) => [u, u])} />
        <Select name="chamberId" value={f.chamberId} label="Casa" options={db.chambers.map((c) => [c.id, c.name])} />
        <Select name="topic" value={f.topic} label="Tema" options={TOPICS.map((t) => [t, t])} />
        <Select name="state" value={f.state} label="Situação" options={[["ready", "Revisado"], ["in_review", "Em análise"]]} />
        <div className="flex items-end">
          <button className="btn w-full justify-center">Filtrar</button>
        </div>
      </form>

      <ul className="space-y-3">
        {propositions.length === 0 && <li className="text-stone-500">Nenhuma proposição encontrada.</li>}
        {propositions.map((b) => {
          const chambers = chambersOf(b);
          return (
            <li key={b.id} className="card">
              <div className="flex flex-wrap items-center gap-2 text-xs text-stone-500">
                <Badge>{SPHERE_LABEL[chambers[0].sphere]}</Badge>
                <span>{chambers.map((c) => c.name).join(" → ")}</span>
                <span>· apresentado em {fmtDate(b.progress[0]?.date)}</span>
                <Badge tone={b.state === "ready" ? "green" : "amber"}>{PUBLIC_STATE_LABEL[b.state]}</Badge>
                {b.topics.map((t) => <Badge key={t} tone="blue">{t}</Badge>)}
              </div>
              <Link href={`/proposicao/${b.id}`} className="mt-1 block text-lg font-semibold hover:underline">
                {b.kind} {b.number}/{b.year}
              </Link>
              <p className="text-stone-700">{b.shortSummary ?? b.ementa}</p>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function Select({ name, value, label, options }: { name: string; value?: string; label: string; options: (readonly [string, string])[] | string[][] }) {
  return (
    <label>
      <span className="label">{label}</span>
      <select name={name} defaultValue={value ?? ""} className="input">
        <option value="">Todas</option>
        {options.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
      </select>
    </label>
  );
}
