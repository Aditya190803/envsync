import { NextRequest, NextResponse } from "next/server";

import { getStackServerApp } from "@/lib/auth/stack";

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

function resolveCloudURL(req: NextRequest) {
  const configured = process.env.ENVSYNC_CLOUD_URL?.trim();
  if (configured) {
    return configured.replace(/\/$/, "");
  }
  return req.nextUrl.origin;
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

    const cloudURL = resolveCloudURL(req);
    const response = await fetch(`${cloudURL}/v1/tokens`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${accessToken}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        scopes: ["profile:read", "store:read", "store:write"],
      }),
      cache: "no-store",
    });

    if (!response.ok) {
      const raw = await response.text();
      const trimmed = raw.trim();
      return NextResponse.json(
        {
          error: `cloud_token_issue_failed: ${response.status}${trimmed ? ` ${trimmed}` : ""}`,
        },
        { status: response.status },
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
