import { signIn } from "@/app/actions";
import { db } from "@/lib/store";
import { ROLE_LABEL } from "@/lib/format";
import { ErrorBanner } from "@/components/ui";

export default async function SignInPage({ searchParams }: { searchParams: Promise<{ erro?: string }> }) {
  const { erro } = await searchParams;
  return (
    <div className="mx-auto max-w-sm space-y-4">
      <h1 className="text-2xl font-bold">Entrar na equipe</h1>
      <ErrorBanner message={erro} />
      <form action={signIn} className="card space-y-3">
        <label className="block">
          <span className="label">E-mail</span>
          <input name="email" type="email" className="input" required />
        </label>
        <label className="block">
          <span className="label">Senha</span>
          <input name="password" type="password" className="input" required />
        </label>
        <div className="rounded-md border border-dashed border-stone-300 p-2 text-xs text-stone-500">
          [código 2FA aqui para Revisor e Admin no sistema real]
        </div>
        <button className="btn w-full justify-center">Entrar</button>
      </form>
      <div className="card text-sm">
        <div className="label">Contas de teste (senha: nelisa123)</div>
        <ul>
          {db.users.filter((u) => u.active).map((u) => (
            <li key={u.id} className="font-mono text-xs">{u.email} · {ROLE_LABEL[u.role]}</li>
          ))}
        </ul>
      </div>
    </div>
  );
}
