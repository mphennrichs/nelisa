// Cliente tipado da API Go, gerado do api/openapi.yaml (`npm run gen:api`).
// No servidor (Server Components) fala direto com a API pela rede interna;
// no navegador, pelo proxy /api do próprio Next.
import createClient from "openapi-fetch";
import type { paths } from "./schema";

export function apiClient() {
  const baseUrl =
    typeof window === "undefined" ? (process.env.API_INTERNAL_URL ?? "http://localhost:8080") : "";
  return createClient<paths>({ baseUrl });
}

export type { paths };
