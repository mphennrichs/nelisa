import Link from "next/link";

export default function Home() {
  return (
    <main id="conteudo" className="mx-auto flex min-h-dvh max-w-2xl flex-col justify-center gap-6 px-6 py-16">
      <p className="text-sm font-semibold uppercase tracking-wide text-stone-600">Em construção</p>
      <h1 className="text-4xl font-bold text-balance sm:text-5xl">Nelisa</h1>
      <p className="text-lg leading-relaxed text-stone-700">
        Proposições legislativas explicadas: resumo, texto comentado e tramitação, revisados por uma equipe com
        regra de quatro olhos. Estamos construindo a primeira versão.
      </p>
      <p className="text-base leading-relaxed text-stone-700">
        Enquanto isso, a demonstração mostra como o site vai funcionar, com dados de teste.
      </p>
      <div>
        <Link href="/demo" className="btn-lg">Ver a demonstração</Link>
      </div>
      <p className="text-sm text-stone-600">
        O código é aberto:{" "}
        <a href="https://github.com/mphennrichs/nelisa" className="link">github.com/mphennrichs/nelisa</a>
      </p>
    </main>
  );
}
