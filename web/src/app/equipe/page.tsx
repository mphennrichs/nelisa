import { redirect } from "next/navigation";

// A página inicial da equipe é a Fila de revisão.
export default function TeamHome() {
  redirect("/equipe/fila");
}
