import Link from "next/link";

// Moldura das páginas públicas reais (fora da /demo). Neutra: nada de cor
// partidária nem destaque que sugira opinião.
export default function PublicShell({ children }: { children: React.ReactNode }) {
  return (
    <>
      <header className="border-b border-stone-200 bg-white">
        <nav aria-label="Principal" className="mx-auto flex max-w-3xl flex-wrap items-center gap-x-5 gap-y-2 px-4 py-3 text-sm">
          <Link href="/" className="text-lg font-bold">Nelisa</Link>
          <Link href="/proposicoes" className="link">Proposições</Link>
          <span className="flex-1" />
          <Link href="/demo" className="text-stone-600 hover:text-stone-900">Demonstração</Link>
        </nav>
      </header>
      <main id="conteudo" className="mx-auto max-w-3xl px-4 py-8">{children}</main>
      <footer className="mx-auto max-w-3xl px-4 pb-10 text-sm text-stone-600">
        Dados oficiais da Câmara dos Deputados e do Senado Federal. Versão em construção.
      </footer>
    </>
  );
}

export function UnderReviewBadge() {
  return (
    <span className="inline-block rounded bg-sky-100 px-2 py-0.5 text-sm font-medium text-sky-900">
      Em análise
    </span>
  );
}
