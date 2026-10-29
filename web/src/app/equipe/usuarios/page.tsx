import { redirect } from "next/navigation";
import * as A from "@/app/actions";
import { db } from "@/lib/store";
import { requireTeamUser } from "@/lib/session";
import { ROLE_LABEL } from "@/lib/format";
import { Badge, ErrorBanner, Hidden } from "@/components/ui";

export default async function UsersPage({ searchParams }: { searchParams: Promise<{ erro?: string }> }) {
  const u = await requireTeamUser();
  if (u.role !== "admin") redirect("/equipe");
  const { erro } = await searchParams;

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Usuários</h1>
      <ErrorBanner message={erro} />
      <form action={A.createUser} className="card grid gap-2 sm:grid-cols-5">
        <input name="name" className="input" placeholder="Nome" required />
        <input name="email" type="email" className="input" placeholder="E-mail" required />
        <input name="password" type="password" minLength={8} className="input" placeholder="Senha inicial (mín. 8)" required />
        <select name="role" className="input">
          {Object.entries(ROLE_LABEL).map(([k, l]) => <option key={k} value={k}>{l}</option>)}
        </select>
        <button className="btn justify-center">Cadastrar</button>
      </form>
      <table className="w-full rounded-lg border border-stone-200 bg-white text-sm">
        <tbody>
          {db.users.map((x) => (
            <tr key={x.id} className="border-t border-stone-100">
              <td className="p-2">{x.name}</td>
              <td className="p-2 font-mono text-xs">{x.email}</td>
              <td className="p-2"><Badge>{ROLE_LABEL[x.role]}</Badge> {!x.active && <Badge tone="amber">desativado</Badge>} {x.mustChangePassword && <Badge>troca de senha pendente</Badge>}</td>
              <td className="p-2">
                <form action={A.resetPassword} className="flex gap-1">
                  <Hidden name="userId" value={x.id} />
                  <input name="password" type="password" minLength={8} className="input w-36" placeholder="Nova senha" required />
                  <button className="btn-ghost">Resetar</button>
                </form>
              </td>
              <td className="p-2">
                {x.id !== u.id && (
                  <form action={A.setActive}>
                    <Hidden name="userId" value={x.id} />
                    <Hidden name="active" value={x.active ? "0" : "1"} />
                    <button className="btn-ghost">{x.active ? "Desativar" : "Reativar"}</button>
                  </form>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
