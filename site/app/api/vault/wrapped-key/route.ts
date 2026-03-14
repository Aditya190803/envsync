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

    const deviceId = req.nextUrl.searchParams.get("deviceId")?.trim();
    if (!deviceId) {
      return NextResponse.json({ error: "device_id_required" }, { status: 400 });
    }

    const result = await client.query(convexFns.vault.getWrappedKeyForCurrentDevice, { deviceId });
    return NextResponse.json(result);
  } catch (err) {
    return NextResponse.json({ error: errorMessage(err) }, { status: 500 });
  }
}

export async function POST(req: NextRequest) {
  try {
    const client = await getAuthedConvexClient(req);
    if (!client) {
      return NextResponse.json({ error: "unauthorized" }, { status: 401 });
    }

    const body = (await req.json()) as {
      actorDeviceId?: string;
      targetDeviceId?: string;
      wrappedVaultKeyB64?: string;
      wrapperAlgorithm?: string;
      keyVersion?: number;
    };
    if (!body.actorDeviceId || !body.targetDeviceId || !body.wrappedVaultKeyB64 || !body.wrapperAlgorithm) {
      return NextResponse.json({ error: "invalid_payload" }, { status: 400 });
    }

    const result = await client.mutation(convexFns.vault.upsertWrappedKeyForDevice, {
      actorDeviceId: body.actorDeviceId,
      targetDeviceId: body.targetDeviceId,
      wrappedVaultKeyB64: body.wrappedVaultKeyB64,
      wrapperAlgorithm: body.wrapperAlgorithm,
      keyVersion: body.keyVersion,
    });
    return NextResponse.json(result);
  } catch (err) {
    return NextResponse.json({ error: errorMessage(err) }, { status: 500 });
  }
}
