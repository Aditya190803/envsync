import { NextRequest, NextResponse } from "next/server";

import { convexFns } from "@/lib/convex/functions";
import { getAuthedConvexClient } from "@/lib/server/convex";

function errorMessage(err: unknown) {
  if (err instanceof Error && err.message) {
    return err.message;
  }
  return "internal_error";
}

export async function GET(req: NextRequest) {
  try {
    const client = await getAuthedConvexClient(req);
    if (!client) {
      return NextResponse.json({ error: "unauthorized" }, { status: 401 });
    }

    const project = req.nextUrl.searchParams.get("project")?.trim() || "default";
    const result = await client.query(convexFns.vault.getEncryptedSnapshot, { project });
    return NextResponse.json(result);
  } catch (err) {
    return NextResponse.json({ error: errorMessage(err) }, { status: 500 });
  }
}

export async function PUT(req: NextRequest) {
  try {
    const client = await getAuthedConvexClient(req);
    if (!client) {
      return NextResponse.json({ error: "unauthorized" }, { status: 401 });
    }

    const body = (await req.json()) as {
      project?: string;
      expectedRevision?: number;
      payload?: unknown;
      saltB64?: string;
      keyCheckB64?: string;
      deviceId?: string;
    };

    if (
      typeof body.expectedRevision !== "number" ||
      !body.payload ||
      !body.deviceId
    ) {
      return NextResponse.json({ error: "invalid_payload" }, { status: 400 });
    }

    const result = await client.mutation(convexFns.vault.putEncryptedSnapshot, {
      project: body.project?.trim() || "default",
      expectedRevision: body.expectedRevision,
      payload: body.payload,
      saltB64: body.saltB64,
      keyCheckB64: body.keyCheckB64,
      deviceId: body.deviceId,
    });

    return NextResponse.json(result);
  } catch (err) {
    const message = errorMessage(err);
    if (message.startsWith("revision_conflict:")) {
      return NextResponse.json({ error: message }, { status: 409 });
    }
    return NextResponse.json({ error: message }, { status: 500 });
  }
}
