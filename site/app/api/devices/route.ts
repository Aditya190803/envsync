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

    const result = await client.query(convexFns.devices.listForCurrentUser, {});
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
      deviceId?: string;
      displayName?: string;
      publicKey?: string;
      keyAlgorithm?: string;
      requesterDeviceId?: string;
    };

    if (!body.deviceId || !body.displayName || !body.publicKey || !body.keyAlgorithm) {
      return NextResponse.json({ error: "invalid_payload" }, { status: 400 });
    }

    const result = await client.mutation(convexFns.devices.registerEnrollmentRequest, {
      deviceId: body.deviceId,
      displayName: body.displayName,
      publicKey: body.publicKey,
      keyAlgorithm: body.keyAlgorithm,
      requesterDeviceId: body.requesterDeviceId,
    });

    return NextResponse.json(result);
  } catch (err) {
    return NextResponse.json({ error: errorMessage(err) }, { status: 500 });
  }
}
