import Link from "next/link";
import { propositionLabel, chamberOf, claimOf, reviewQueue, userById } from "@/lib/store";
import { requireTeamUser } from "@/lib/session";
import { CONTRIBUTION_LABEL, SPHERE_LABEL, TEAM_STATE_LABEL, daysUntil, fmtDate } from "@/lib/format";
import { Badge } from "@/components/ui";

export default async function QueuePage({ searchParams }: { searchParams: Promise<{ esfera?: string }> }) {
  await requireTeamUser();
  const { esfera } = await searchParams;
  const items = reviewQueue(esfera);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="mr-4 text-2xl font-bold">Fila de revisão</h1>
        {[undefined, "federal", "state", "municipal"].map((s) => (
          <Link key={s ?? "all"} href={s ? `/equipe?esfera=${s}` : "/equipe"} className={s === esfera ? "btn" : "btn-ghost"}>
            {s ? SPHERE_LABEL[s as keyof typeof SPHERE_LABEL] : "Todas"}
          </Link>
        ))}
      </div>
      <p className="text-sm text-stone-500">Ordem: esfera (federal → municipal), depois votação mais próxima, depois o mais antigo.</p>
      <table className="w-full overflow-hidden rounded-lg border border-stone-200 bg-white text-sm">
        <thead className="bg-stone-100 text-left text-xs uppercase text-stone-500">
          <tr>
            <th className="p-2">Esfera</th>
            <th className="p-2">Item</th>
            <th className="p-2">Votação</th>
            <th className="p-2">Posse</th>
          </tr>
        </thead>
        <tbody>
          {items.map((i, idx) => {
            const claim = i.proposition && claimOf(i.proposition);
            return (
              <tr key={idx} className="border-t border-stone-100">
                <td className="p-2"><Badge>{SPHERE_LABEL[i.sphere]}</Badge></td>
                <td className="p-2">
                  {i.kind === "contribution" ? (
                    <Link href="/equipe/contribuicoes" className="link">
                      {CONTRIBUTION_LABEL[i.contribution!.kind]} {i.proposition && `· ${propositionLabel(i.proposition)}`} ({i.contribution!.trackingCode})
                    </Link>
                  ) : (
                    <Link href={`/equipe/proposicao/${i.proposition!.id}`} className="link">
                      {propositionLabel(i.proposition!)} · {chamberOf(i.proposition!).name}
                    </Link>
                  )}{" "}
                  {i.kind === "proposition" && <Badge tone="amber">{TEAM_STATE_LABEL[i.proposition!.state]}</Badge>}
                  {i.kind === "second_review" && <Badge tone="blue">Segunda revisão pendente</Badge>}
                </td>
                <td className="p-2">{i.vote ? `${fmtDate(i.vote)} (${daysUntil(i.vote)}d)` : "—"}</td>
                <td className="p-2 text-stone-500">{claim ? userById(claim.userId)?.name : "livre"}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
