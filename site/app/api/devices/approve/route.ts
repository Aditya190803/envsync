import { NextRequest, NextResponse } from "next/server";

import { convexFns } from "@/lib/convex/functions";
import { getAuthedConvexClient } from "@/lib/server/convex";

function errorMessage(err: unknown) {
  if (err instanceof Error && err.message) {
    return err.message;
  }
  return "internal_error";
}

export async function POST(req: NextRequest) {
  try {
    const client = await getAuthedConvexClient(req);
    if (!client) {
      return NextResponse.json({ error: "unauthorized" }, { status: 401 });
    }

    const body = (await req.json()) as {
      requestId?: string;
      approverDeviceId?: string;
      recoveryUsed?: boolean;
      wrappedVaultKeyB64?: string;
      wrapperAlgorithm?: string;
      keyVersion?: number;
    };

    if (!body.requestId || !body.wrappedVaultKeyB64 || !body.wrapperAlgorithm) {
      return NextResponse.json({ error: "invalid_payload" }, { status: 400 });
    }

    const result = await client.mutation(convexFns.devices.approveEnrollmentRequest, {
      requestId: body.requestId,
      approverDeviceId: body.approverDeviceId,
      recoveryUsed: Boolean(body.recoveryUsed),
      wrappedVaultKeyB64: body.wrappedVaultKeyB64,
      wrapperAlgorithm: body.wrapperAlgorithm,
      keyVersion: body.keyVersion,
    });

    return NextResponse.json(result);
  } catch (err) {
    const message = errorMessage(err);
    if (message.includes("not_found") || message.includes("not_pending")) {
      return NextResponse.json({ error: message }, { status: 409 });
    }
    return NextResponse.json({ error: message }, { status: 500 });
  }
}
