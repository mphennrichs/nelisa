import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: { default: "Nelisa — demonstração", template: "%s · Nelisa (demo)" },
  description: "Demonstração do Nelisa com dados de teste.",
};

// Protótipo em memória, público e aberto, usado para apresentar o projeto.
// Nada aqui fala com a API real.
export default function DemoLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <div className="bg-amber-100 px-4 py-1.5 text-center text-sm text-amber-900">
        Demonstração com dados em memória. Só as PECs 221/2019 e 8/2025 têm dados reais; resumos, comentários e o resto
        são de teste.
      </div>
      <header className="border-b border-stone-200 bg-white">
        <nav aria-label="Demonstração" className="mx-auto flex max-w-5xl flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 text-sm">
          <Link href="/demo" className="text-lg font-bold">Nelisa <span className="font-normal text-stone-600">demo</span></Link>
          <Link href="/demo" className="link">Proposições</Link>
          <Link href="/demo/contribuir" className="link">Contribuir</Link>
          <Link href="/demo/contribuicao" className="link">Acompanhar contribuição</Link>
          <span className="flex-1" />
          <Link href="/demo/equipe" className="text-stone-600 hover:text-stone-900">Área da equipe (demo)</Link>
        </nav>
      </header>
      <main id="conteudo" className="mx-auto max-w-5xl px-4 py-6">{children}</main>
    </>
  );
}
