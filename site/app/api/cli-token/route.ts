import { NextRequest, NextResponse } from "next/server";

import { getStackServerApp } from "@/lib/auth/stack";
import { formatUpstreamError, resolveCloudURLs } from "@/lib/server/cloudUrl";

type CreateTokenResponse = {
  token?: string;
  id?: string;
  expires_at?: string | null;
  error?: string;
};

function errorMessage(err: unknown) {
  if (err instanceof Error && err.message) {
    return err.message;
  }
  return "internal_error";
}

export async function POST(req: NextRequest) {
  try {
    const stackApp = getStackServerApp();
    if (!stackApp) {
      return NextResponse.json({ error: "stack_auth_not_configured" }, { status: 500 });
    }

    const user = await stackApp.getUser({ tokenStore: req, or: "return-null" });
    if (!user) {
      return NextResponse.json({ error: "unauthorized" }, { status: 401 });
    }

    const accessToken = await stackApp.getAccessToken({ tokenStore: req });
    if (!accessToken) {
      return NextResponse.json({ error: "missing_access_token" }, { status: 401 });
    }

    const devToken = process.env.NODE_ENV !== "production" ? process.env.ENVSYNC_CLOUD_DEV_TOKEN?.trim() : "";
    const upstreamAccessToken = devToken || accessToken;

    const cloudURLs = resolveCloudURLs({
      configuredCloudURL: process.env.ENVSYNC_CLOUD_URL,
      hostname: req.nextUrl.hostname,
      origin: req.nextUrl.origin,
    });

    let response: Response | null = null;
    let responseBody = "";
    let networkError = "";
    for (let i = 0; i < cloudURLs.length; i += 1) {
      const cloudURL = cloudURLs[i];
      try {
        response = await fetch(`${cloudURL}/v1/tokens`, {
          method: "POST",
          headers: {
            Authorization: `Bearer ${upstreamAccessToken}`,
            "Content-Type": "application/json",
          },
          body: JSON.stringify({
            scopes: ["profile:read", "store:read", "store:write"],
          }),
          cache: "no-store",
        });
      } catch (err) {
        networkError = errorMessage(err);
        const hasFallback = i < cloudURLs.length - 1;
        if (hasFallback) {
          continue;
        }
        return NextResponse.json(
          {
            error: `cloud_token_issue_failed: 502 ${networkError}`,
          },
          { status: 502 },
        );
      }

      if (response.ok) {
        break;
      }

      responseBody = await response.text();
      const hasFallback = i < cloudURLs.length - 1;
      if (response.status === 404 && hasFallback) {
        continue;
      }

      const upstreamError = formatUpstreamError(responseBody);
      return NextResponse.json(
        {
          error: `cloud_token_issue_failed: ${response.status}${upstreamError ? ` ${upstreamError}` : ""}`,
        },
        { status: response.status },
      );
    }

    if (!response || !response.ok) {
      const upstreamError = formatUpstreamError(responseBody || networkError);
      return NextResponse.json(
        {
          error: `cloud_token_issue_failed: ${response?.status ?? 502}${upstreamError ? ` ${upstreamError}` : ""}`,
        },
        { status: response?.status ?? 502 },
      );
    }

    const payload = (await response.json()) as CreateTokenResponse;
    if (!payload.token) {
      return NextResponse.json({ error: "cloud_token_issue_missing_token" }, { status: 502 });
    }

    return NextResponse.json({
      token: payload.token,
      id: payload.id ?? null,
      expiresAt: payload.expires_at ?? null,
    });
  } catch (err) {
    return NextResponse.json({ error: errorMessage(err) }, { status: 500 });
  }
}
