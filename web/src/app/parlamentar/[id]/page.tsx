import Link from "next/link";
import { notFound } from "next/navigation";
import { db, toPublic } from "@/lib/store";
import { PUBLIC_STATE_LABEL, fmtDate } from "@/lib/format";
import { Badge } from "@/components/ui";

export default async function LegislatorPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const l = db.legislators.find((x) => x.id === id);
  if (!l) notFound();
  const propositions = db.propositions
    .filter((b) => b.authors.some((a) => a.kind === "legislator" && a.legislatorId === l.id))
    .map(toPublic)
    .filter((b) => b !== null);

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{l.name}</h1>
      <section className="card">
        <h2 className="mb-2 font-semibold">Mandatos</h2>
        <ul className="text-sm">
          {l.terms.map((t, i) => (
            <li key={i}>
              {db.chambers.find((c) => c.id === t.chamberId)?.name} · {t.party}/{t.uf} · {fmtDate(t.start)} – {t.end ? fmtDate(t.end) : "atual"}
            </li>
          ))}
        </ul>
        {l.officialContact && <p className="mt-2 text-sm">Contato oficial: {l.officialContact}</p>}
      </section>
      <section>
        <h2 className="mb-2 font-semibold">Proposições de autoria</h2>
        <ul className="space-y-2">
          {propositions.map((b) => (
            <li key={b.id} className="card">
              <Link href={`/proposicao/${b.id}`} className="font-semibold hover:underline">{b.kind} {b.number}/{b.year}</Link>{" "}
              <Badge tone={b.state === "ready" ? "green" : "amber"}>{PUBLIC_STATE_LABEL[b.state]}</Badge>
              <p className="text-sm text-stone-700">{b.ementa}</p>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
