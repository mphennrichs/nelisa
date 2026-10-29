import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: "Nelisa",
  description: "Projetos de lei explicados — protótipo",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="pt-BR">
      <body className="antialiased">
        <div className="bg-amber-100 px-4 py-1 text-center text-xs text-amber-900">
          Protótipo, tudo em memória. Dados reais só nas PECs 221/2019 e 8/2025; resumos, comentários e o resto são de teste.
        </div>
        <header className="border-b border-stone-200 bg-white">
          <nav className="mx-auto flex max-w-5xl items-center gap-4 px-4 py-3 text-sm">
            <Link href="/" className="text-lg font-bold">Nelisa</Link>
            <Link href="/" className="link">Projetos de lei</Link>
            <Link href="/contribuir" className="link">Contribuir</Link>
            <Link href="/contribuicao" className="link">Acompanhar contribuição</Link>
            <span className="flex-1" />
            <Link href="/equipe" className="text-stone-500 hover:text-stone-900">Área da equipe</Link>
          </nav>
        </header>
        <main className="mx-auto max-w-5xl px-4 py-6">{children}</main>
      </body>
    </html>
  );
}
