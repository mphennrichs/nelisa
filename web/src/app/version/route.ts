// Same contract as the other fogolab apps: which commit is live.
export const dynamic = "force-dynamic";

export function GET() {
  return Response.json({
    commit: process.env.GIT_COMMIT ?? "dev",
    builtAt: process.env.BUILD_TIME ?? null,
  });
}
