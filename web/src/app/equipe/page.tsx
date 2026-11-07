"use client";

import Link from "next/link";
import { ROLE_LABEL } from "@/lib/team/session";
import { RequireTeam } from "@/lib/team/ui";

export default function TeamHome() {
  return (
    <RequireTeam>
      {(user) => (
        <div className="space-y-6">
          <h1 className="text-2xl font-bold">Olá, {user.name}</h1>
          <p className="text-stone-700">Você entrou como {ROLE_LABEL[user.role]}.</p>
          <section className="card space-y-2" aria-labelledby="em-construcao">
            <h2 id="em-construcao" className="text-lg font-semibold">Em construção</h2>
            <p className="text-stone-700">
              A Importação de Proposições chega na fase 1 e a Fila de revisão na fase 2. O andamento está no{" "}
              <a href="https://github.com/users/mphennrichs/projects/1" className="link">quadro do projeto</a>.
            </p>
          </section>
          {user.role === "admin" && (
            <p>
              <Link href="/equipe/usuarios" className="link">Gerenciar usuários da equipe</Link>
            </p>
          )}
        </div>
      )}
    </RequireTeam>
  );
}
