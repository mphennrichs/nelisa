import Link from "next/link";
import PublicShell from "@/components/PublicShell";

export default function NotFound() {
  return (
    <PublicShell>
      <h1 className="mb-3 text-2xl font-bold">Proposição não encontrada</h1>
      <p className="text-stone-700">
        Ela pode ainda não ter sido publicada pela equipe. Veja as <Link href="/proposicoes" className="link">Proposições acompanhadas</Link>.
      </p>
    </PublicShell>
  );
}
