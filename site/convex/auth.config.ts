import { getConvexProvidersConfig } from "@stackframe/stack/convex-auth.config";
import type { AuthConfig } from "convex/server";

const stackProjectId =
  process.env.NEXT_PUBLIC_STACK_PROJECT_ID ?? process.env.STACK_PROJECT_ID;

if (!stackProjectId) {
  throw new Error(
    "Missing NEXT_PUBLIC_STACK_PROJECT_ID (or STACK_PROJECT_ID) for Convex auth providers.",
  );
}

const stackBaseUrl =
  process.env.NEXT_PUBLIC_STACK_BASE_URL ?? process.env.STACK_BASE_URL;

const providers = getConvexProvidersConfig({
  projectId: stackProjectId,
  ...(stackBaseUrl ? { baseUrl: stackBaseUrl } : {}),
}).map((provider) => ({
  ...provider,
  type: "customJwt" as const,
  issuer: provider.issuer.toString(),
  jwks: provider.jwks.toString(),
  algorithm: provider.algorithm as "RS256" | "ES256",
}));

export default {
  providers,
} satisfies AuthConfig;
