import { db } from "@/lib/store";
import { CONTRIBUTION_LABEL, CONTRIBUTION_STATUS_LABEL, fmtDate } from "@/lib/format";
import { Badge } from "@/components/ui";

export default async function TrackPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  const sp = await searchParams;
  const code = sp.codigo?.trim().toUpperCase();
  const c = code ? db.contributions.find((x) => x.trackingCode === code) : undefined;

  return (
    <div className="max-w-xl space-y-4">
      <h1 className="text-2xl font-bold">Acompanhar contribuição</h1>
      {sp.novo && c && (
        <div className="card border-emerald-300 bg-emerald-50">
          Recebemos sua contribuição. Guarde o código <strong className="font-mono">{c.trackingCode}</strong>: é a única forma
          de acompanhar, já que não pedimos e-mail.
        </div>
      )}
      <form className="flex gap-2">
        <input name="codigo" defaultValue={code} placeholder="NL-XXXX" className="input font-mono" />
        <button className="btn">Consultar</button>
      </form>
      {code && !c && <p className="text-stone-500">Código não encontrado.</p>}
      {c && (
        <div className="card space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-mono">{c.trackingCode}</span>
            <Badge tone={c.status === "accepted" ? "green" : c.status === "rejected" ? "neutral" : "amber"}>
              {CONTRIBUTION_STATUS_LABEL[c.status]}
            </Badge>
          </div>
          <div className="text-sm text-stone-600">{CONTRIBUTION_LABEL[c.kind]} · enviada em {fmtDate(c.createdAt)}</div>
        </div>
      )}
    </div>
  );
}
