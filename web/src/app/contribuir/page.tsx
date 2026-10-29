import Link from "next/link";
import { submitContribution } from "@/app/actions";
import { db, publicBills } from "@/lib/store";
import { CONTRIBUTION_LABEL, PROGRESS_LABEL } from "@/lib/format";
import { ErrorBanner, Hidden } from "@/components/ui";
import type { ContributionKind } from "@/lib/types";

const KINDS: ContributionKind[] = ["referral", "progress_update", "source_request"];

export default async function ContributePage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  const sp = await searchParams;
  const kind = (KINDS.includes(sp.tipo as ContributionKind) ? sp.tipo : "referral") as ContributionKind;
  const bills = publicBills({});

  return (
    <div className="max-w-2xl space-y-4">
      <h1 className="text-2xl font-bold">Contribuir</h1>
      <p className="text-stone-600">
        Sem cadastro. Você recebe um código para acompanhar. Toda contribuição precisa de uma fonte e passa pela equipe antes
        de mudar qualquer coisa.
      </p>
      <nav className="flex gap-2">
        {KINDS.map((k) => (
          <Link key={k} href={`/contribuir?tipo=${k}`} className={k === kind ? "btn" : "btn-ghost"}>{CONTRIBUTION_LABEL[k]}</Link>
        ))}
      </nav>
      <ErrorBanner message={sp.erro} />
      <form action={submitContribution} className="card space-y-3">
        <Hidden name="kind" value={kind} />
        {kind === "referral" && (
          <label className="block">
            <span className="label">Endereço do PL na fonte oficial</span>
            <input name="url" className="input" placeholder="https://www.camara-da-sua-cidade.leg.br/..." required />
            <span className="text-xs text-stone-500">Aceitamos domínios .leg.br, .gov.br e fontes oficiais cadastradas.</span>
          </label>
        )}
        {kind === "progress_update" && (
          <>
            <label className="block">
              <span className="label">Projeto</span>
              <select name="billId" defaultValue={sp.pl} className="input" required>
                <option value="">Escolha</option>
                {bills.map((b) => (
                  <option key={b.id} value={b.id}>
                    {b.kind} {b.number}/{b.year} — {db.chambers.find((c) => c.id === b.chamberId)?.name}
                  </option>
                ))}
              </select>
            </label>
            <div className="grid grid-cols-2 gap-3">
              <label>
                <span className="label">Tipo de evento</span>
                <select name="progressKind" className="input" required>
                  {Object.entries(PROGRESS_LABEL).map(([k, l]) => <option key={k} value={k}>{l}</option>)}
                </select>
              </label>
              <label>
                <span className="label">Data</span>
                <input type="date" name="date" className="input" required />
              </label>
            </div>
            <label className="block">
              <span className="label">Descrição curta (opcional)</span>
              <input name="note" className="input" placeholder="Ex.: votação adiada" />
            </label>
          </>
        )}
        {kind === "source_request" && (
          <>
            <label className="block">
              <span className="label">Domínio</span>
              <input name="domain" className="input" placeholder="camaradexyz.com.br" required />
            </label>
            <label className="block">
              <span className="label">De qual Casa legislativa?</span>
              <input name="note" className="input" placeholder="Câmara Municipal de XYZ" />
            </label>
          </>
        )}
        <label className="block">
          <span className="label">Fonte (link que comprova)</span>
          <input name="source" className="input" placeholder="https://..." required />
        </label>
        <div className="rounded-md border border-dashed border-stone-300 p-2 text-xs text-stone-500">
          [captcha aqui no sistema real]
        </div>
        <button className="btn">Enviar</button>
      </form>
    </div>
  );
}
