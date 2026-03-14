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
      targetDeviceId?: string;
      actorDeviceId?: string;
    };

    if (!body.targetDeviceId) {
      return NextResponse.json({ error: "invalid_payload" }, { status: 400 });
    }

    const result = await client.mutation(convexFns.devices.revokeDevice, {
      targetDeviceId: body.targetDeviceId,
      actorDeviceId: body.actorDeviceId,
    });

    return NextResponse.json(result);
  } catch (err) {
    return NextResponse.json({ error: errorMessage(err) }, { status: 500 });
  }
}
