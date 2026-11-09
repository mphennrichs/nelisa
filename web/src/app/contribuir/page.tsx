import type { Metadata } from "next";
import Link from "next/link";
import ContributionForm from "./ContributionForm";
import PublicShell from "@/components/PublicShell";
import { getContributionAvailability, getPublicProposition } from "@/lib/public/api";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Contribuir · Nelisa",
  description: "Indique uma Proposição, informe um evento de Tramitação ou peça o cadastro de uma Fonte oficial.",
};

type Props = { searchParams: Promise<{ proposicao?: string | string[] }> };

export default async function ContributePage({ searchParams }: Props) {
  const availability = await getContributionAvailability();
  const raw = (await searchParams).proposicao;
  const slug = typeof raw === "string" ? raw : undefined;
  const p = availability.open && slug ? await getPublicProposition(slug).catch(() => null) : null;

  return (
    <PublicShell>
      <div className="space-y-6">
        <div className="space-y-2">
          <h1 className="text-3xl font-bold">Contribuir</h1>
          <p className="text-lg text-stone-700">
            Ajude a equipe a manter o Nelisa em dia. Toda Contribuição precisa de uma fonte e é avaliada pela equipe antes de
            mudar qualquer coisa no site. Você não precisa se identificar.
          </p>
        </div>
        {availability.open ? (
          <ContributionForm proposition={p ? { slug: p.slug, name: p.name } : undefined} />
        ) : (
          <div className="card space-y-2">
            <h2 className="text-lg font-semibold">
              {availability.reason === "paused" ? "Contribuições pausadas" : "Contribuições ainda não estão abertas"}
            </h2>
            <p className="text-stone-700">
              {availability.reason === "paused"
                ? "A equipe está avaliando as Contribuições pendentes e reabre o formulário logo."
                : "O formulário abre em breve."}{" "}
              Se você já mandou uma, pode <Link href="/acompanhar" className="link">acompanhar pelo código</Link>.
            </p>
          </div>
        )}
      </div>
    </PublicShell>
  );
}
