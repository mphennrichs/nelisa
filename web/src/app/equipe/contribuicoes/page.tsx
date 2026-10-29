import { resolveContribution } from "@/app/actions";
import { propositionLabel, db, domainOf, isOfficialDomain } from "@/lib/store";
import { requireTeamUser } from "@/lib/session";
import { CONTRIBUTION_LABEL, CONTRIBUTION_STATUS_LABEL, PROGRESS_LABEL, fmtDate } from "@/lib/format";
import { Badge, ErrorBanner, Hidden } from "@/components/ui";

export default async function ContributionsPage({ searchParams }: { searchParams: Promise<{ erro?: string }> }) {
  await requireTeamUser();
  const { erro } = await searchParams;
  const list = [...db.contributions].sort((a, b) => b.createdAt.localeCompare(a.createdAt));

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Contribuições</h1>
      <ErrorBanner message={erro} />
      {list.map((c) => {
        const proposition = c.propositionId ? db.propositions.find((b) => b.id === c.propositionId) : undefined;
        const srcDomain = domainOf(c.source);
        return (
          <div key={c.trackingCode} className="card space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-sm">{c.trackingCode}</span>
              <Badge>{CONTRIBUTION_LABEL[c.kind]}</Badge>
              <Badge tone={c.status === "pending" ? "amber" : c.status === "accepted" ? "green" : "neutral"}>
                {CONTRIBUTION_STATUS_LABEL[c.status]}
              </Badge>
              <span className="text-xs text-stone-500">{fmtDate(c.createdAt)}</span>
            </div>
            <div className="text-sm">
              {c.kind === "referral" && <>Endereço indicado: <a href={c.url} className="link" target="_blank" rel="noreferrer">{c.url}</a> <span className="text-stone-500">(confira à mão: o Nelisa não baixa o link na v1)</span></>}
              {c.kind === "progress_update" && proposition && (
                <>{propositionLabel(proposition)}: <strong>{PROGRESS_LABEL[c.progressKind!]}</strong> em {fmtDate(c.date)}{c.note && ` — “${c.note}”`}</>
              )}
              {c.kind === "source_request" && <>Domínio <strong>{c.domain}</strong>{c.note && ` — diz ser: ${c.note}`}</>}
            </div>
            <div className="text-sm">
              Fonte: <a href={c.source} className="link" target="_blank" rel="noreferrer">{c.source}</a>{" "}
              {srcDomain && (isOfficialDomain(srcDomain) ? <Badge tone="green">fonte oficial</Badge> : <Badge>fonte não oficial</Badge>)}
            </div>
            {c.status === "pending" && (
              <div className="flex flex-wrap gap-2">
                <form action={resolveContribution} className="flex gap-2">
                  <Hidden name="code" value={c.trackingCode} />
                  <Hidden name="decision" value="accept" />
                  {c.kind === "source_request" && (
                    <select name="chamberId" className="input w-64" required>
                      <option value="">Casa dona do domínio…</option>
                      {db.chambers.map((ch) => <option key={ch.id} value={ch.id}>{ch.name}</option>)}
                    </select>
                  )}
                  <button className="btn">Aceitar</button>
                </form>
                <form action={resolveContribution}>
                  <Hidden name="code" value={c.trackingCode} />
                  <Hidden name="decision" value="reject" />
                  <button className="btn-ghost">Rejeitar</button>
                </form>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
