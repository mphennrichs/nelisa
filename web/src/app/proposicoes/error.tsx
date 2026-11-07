"use client";

export default function Error({ reset }: { reset: () => void }) {
  return (
    <main id="conteudo" className="mx-auto max-w-3xl px-4 py-16">
      <h1 className="mb-3 text-2xl font-bold">Não foi possível carregar</h1>
      <p className="mb-4 text-stone-700">O serviço está indisponível agora. Tente de novo em instantes.</p>
      <button type="button" onClick={reset} className="btn-lg">Tentar de novo</button>
    </main>
  );
}
