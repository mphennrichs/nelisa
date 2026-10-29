import { changePassword } from "@/app/actions";
import { requireUser } from "@/lib/session";
import { ErrorBanner } from "@/components/ui";

export default async function PasswordPage({ searchParams }: { searchParams: Promise<{ erro?: string }> }) {
  const u = await requireUser();
  const { erro } = await searchParams;
  return (
    <div className="mx-auto max-w-sm space-y-4">
      <h1 className="text-xl font-bold">Troque sua senha</h1>
      {u.mustChangePassword && (
        <p className="text-sm text-stone-600">Sua senha foi definida pelo Admin. Escolha uma nova antes de continuar.</p>
      )}
      <ErrorBanner message={erro} />
      <form action={changePassword} className="card space-y-3">
        <input name="password" type="password" minLength={8} className="input" placeholder="Nova senha (mín. 8)" required />
        <button className="btn">Salvar</button>
      </form>
    </div>
  );
}
