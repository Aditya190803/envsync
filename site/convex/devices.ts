/* eslint-disable @typescript-eslint/no-explicit-any */

import { mutationGeneric, queryGeneric } from "convex/server";
import { ConvexError, v } from "convex/values";

import { requireStackUser } from "./lib/auth";
import { writeAuditEvent } from "./lib/audit";

async function findDeviceById(db: any, stackUserId: string, deviceId: string) {
  const rows = await db
    .query("devices")
    .withIndex("by_stack_user_id_device_id", (q: any) =>
      q.eq("stackUserId", stackUserId).eq("deviceId", deviceId),
    )
    .collect();
  return rows[0] || null;
}

async function findWrappedKeyByDevice(db: any, stackUserId: string, deviceId: string) {
  const rows = await db
    .query("wrapped_vault_keys")
    .withIndex("by_stack_user_id_device_id", (q: any) =>
      q.eq("stackUserId", stackUserId).eq("deviceId", deviceId),
    )
    .collect();
  return rows[0] || null;
}

async function ensureUserProfile(db: any, stackUserId: string, email?: string) {
  const rows = await db
    .query("user_profiles")
    .withIndex("by_stack_user_id", (q: any) => q.eq("stackUserId", stackUserId))
    .collect();
  const now = Date.now();
  const existing = rows[0];
  if (!existing) {
    await db.insert("user_profiles", {
      stackUserId,
      primaryEmail: email,
      createdAt: now,
      updatedAt: now,
    });
    return;
  }
  if (email && existing.primaryEmail !== email) {
    await db.patch(existing._id, { primaryEmail: email, updatedAt: now });
  }
}

export const listForCurrentUser = queryGeneric({
  args: {},
  handler: async (ctx) => {
    const { stackUserId } = await requireStackUser(ctx);
    const devices = await ctx.db
      .query("devices")
      .withIndex("by_stack_user_id", (q: any) => q.eq("stackUserId", stackUserId))
      .collect();
    const pendingEnrollments = await ctx.db
      .query("enrollment_requests")
      .withIndex("by_stack_user_id_status", (q: any) =>
        q.eq("stackUserId", stackUserId).eq("status", "pending"),
      )
      .collect();

    return {
      devices: devices
        .slice()
        .sort((a, b) => b.updatedAt - a.updatedAt)
        .map((device) => ({
          id: String(device._id),
          deviceId: device.deviceId,
          displayName: device.displayName,
          status: device.status,
          keyAlgorithm: device.keyAlgorithm,
          approvedAt: device.approvedAt ?? null,
          revokedAt: device.revokedAt ?? null,
          lastSeenAt: device.lastSeenAt ?? null,
          updatedAt: device.updatedAt,
        })),
      pendingEnrollments: pendingEnrollments
        .slice()
        .sort((a, b) => b.createdAt - a.createdAt)
        .map((request) => ({
          id: String(request._id),
          targetDeviceId: request.targetDeviceId,
          targetDeviceName: request.targetDeviceName,
          targetPublicKey: request.targetPublicKey,
          targetKeyAlgorithm: request.targetKeyAlgorithm,
          requesterDeviceId: request.requesterDeviceId ?? null,
          createdAt: request.createdAt,
        })),
    };
  },
});

export const registerEnrollmentRequest = mutationGeneric({
  args: {
    deviceId: v.string(),
    displayName: v.string(),
    publicKey: v.string(),
    keyAlgorithm: v.string(),
    requesterDeviceId: v.optional(v.string()),
  },
  handler: async (ctx, args) => {
    const { stackUserId, email } = await requireStackUser(ctx);
    await ensureUserProfile(ctx.db, stackUserId, email);

    const deviceId = args.deviceId.trim();
    if (!deviceId) {
      throw new ConvexError("device_id_required");
    }

    const now = Date.now();
    const existingDevice = await findDeviceById(ctx.db, stackUserId, deviceId);
    if (!existingDevice) {
      await ctx.db.insert("devices", {
        stackUserId,
        deviceId,
        displayName: args.displayName.trim() || "Unnamed device",
        publicKey: args.publicKey,
        keyAlgorithm: args.keyAlgorithm,
        status: "pending",
        createdAt: now,
        updatedAt: now,
      });
    } else {
      const nextStatus = existingDevice.status === "revoked" ? "pending" : existingDevice.status;
      await ctx.db.patch(existingDevice._id, {
        displayName: args.displayName.trim() || existingDevice.displayName,
        publicKey: args.publicKey,
        keyAlgorithm: args.keyAlgorithm,
        status: nextStatus,
        lastSeenAt: existingDevice.status === "approved" ? now : existingDevice.lastSeenAt,
        updatedAt: now,
      });

      if (existingDevice.status === "approved") {
        await writeAuditEvent(ctx, stackUserId, "device_seen", {
          deviceId,
          requesterDeviceId: args.requesterDeviceId ?? null,
        });
        return {
          requestId: "",
          status: "approved",
        };
      }
    }

    const pending = await ctx.db
      .query("enrollment_requests")
      .withIndex("by_stack_user_id_target_device", (q: any) =>
        q.eq("stackUserId", stackUserId).eq("targetDeviceId", deviceId),
      )
      .collect();

    const latestPending = pending
      .filter((request) => request.status === "pending")
      .sort((a, b) => b.createdAt - a.createdAt)[0];

    let requestId: string;
    if (latestPending) {
      await ctx.db.patch(latestPending._id, {
        targetDeviceName: args.displayName.trim() || latestPending.targetDeviceName,
        targetPublicKey: args.publicKey,
        targetKeyAlgorithm: args.keyAlgorithm,
        requesterDeviceId: args.requesterDeviceId,
        updatedAt: now,
      });
      requestId = String(latestPending._id);
    } else {
      const created = await ctx.db.insert("enrollment_requests", {
        stackUserId,
        targetDeviceId: deviceId,
        targetDeviceName: args.displayName.trim() || "Unnamed device",
        targetPublicKey: args.publicKey,
        targetKeyAlgorithm: args.keyAlgorithm,
        requesterDeviceId: args.requesterDeviceId,
        status: "pending",
        createdAt: now,
        updatedAt: now,
      });
      requestId = String(created);
    }

    await writeAuditEvent(ctx, stackUserId, "device_enrollment_requested", {
      deviceId,
      requesterDeviceId: args.requesterDeviceId ?? null,
    });

    return {
      requestId,
      status: "pending",
    };
  },
});

export const approveEnrollmentRequest = mutationGeneric({
  args: {
    requestId: v.id("enrollment_requests"),
    approverDeviceId: v.optional(v.string()),
    recoveryUsed: v.boolean(),
    wrappedVaultKeyB64: v.string(),
    wrapperAlgorithm: v.string(),
    keyVersion: v.optional(v.number()),
  },
  handler: async (ctx, args) => {
    const { stackUserId } = await requireStackUser(ctx);
    const now = Date.now();

    const request = await ctx.db.get(args.requestId);
    if (!request || request.stackUserId !== stackUserId) {
      throw new ConvexError("enrollment_request_not_found");
    }
    if (request.status !== "pending") {
      throw new ConvexError("enrollment_request_not_pending");
    }

    if (!args.recoveryUsed) {
      const approverId = args.approverDeviceId?.trim() || "";
      if (!approverId) {
        throw new ConvexError("approver_device_required");
      }
      const approverDevice = await findDeviceById(ctx.db, stackUserId, approverId);
      if (!approverDevice || approverDevice.status !== "approved") {
        throw new ConvexError("approver_device_not_approved");
      }
      await ctx.db.patch(approverDevice._id, { lastSeenAt: now, updatedAt: now });
    }

    const targetDevice = await findDeviceById(ctx.db, stackUserId, request.targetDeviceId);
    if (!targetDevice) {
      throw new ConvexError("target_device_missing");
    }
    await ctx.db.patch(targetDevice._id, {
      status: "approved",
      approvedAt: now,
      revokedAt: undefined,
      updatedAt: now,
      lastSeenAt: now,
    });

    const wrapped = await findWrappedKeyByDevice(ctx.db, stackUserId, request.targetDeviceId);
    const wrappedPayload = {
      wrappedVaultKeyB64: args.wrappedVaultKeyB64,
      wrapperAlgorithm: args.wrapperAlgorithm,
      keyVersion: args.keyVersion ?? 1,
      updatedAt: now,
      revokedAt: undefined,
    };
    if (!wrapped) {
      await ctx.db.insert("wrapped_vault_keys", {
        stackUserId,
        deviceId: request.targetDeviceId,
        createdAt: now,
        ...wrappedPayload,
      });
    } else {
      await ctx.db.patch(wrapped._id, wrappedPayload);
    }

    await ctx.db.patch(request._id, {
      status: "approved",
      approvedAt: now,
      recoveryUsed: args.recoveryUsed,
      approvedByDeviceId: args.approverDeviceId,
      wrappedVaultKeyB64: args.wrappedVaultKeyB64,
      wrapperAlgorithm: args.wrapperAlgorithm,
      updatedAt: now,
    });

    await writeAuditEvent(
      ctx,
      stackUserId,
      "device_enrollment_approved",
      {
        requestId: String(request._id),
        targetDeviceId: request.targetDeviceId,
        recoveryUsed: args.recoveryUsed,
      },
      args.approverDeviceId,
    );

    return {
      approved: true,
      targetDeviceId: request.targetDeviceId,
    };
  },
});

export const revokeDevice = mutationGeneric({
  args: {
    targetDeviceId: v.string(),
    actorDeviceId: v.optional(v.string()),
  },
  handler: async (ctx, args) => {
    const { stackUserId } = await requireStackUser(ctx);
    const now = Date.now();

    const target = await findDeviceById(ctx.db, stackUserId, args.targetDeviceId);
    if (!target) {
      throw new ConvexError("device_not_found");
    }

    if (args.actorDeviceId) {
      const actor = await findDeviceById(ctx.db, stackUserId, args.actorDeviceId);
      if (!actor || actor.status !== "approved") {
        throw new ConvexError("actor_device_not_approved");
      }
      await ctx.db.patch(actor._id, { lastSeenAt: now, updatedAt: now });
    }

    if (target.status !== "revoked") {
      await ctx.db.patch(target._id, {
        status: "revoked",
        revokedAt: now,
        updatedAt: now,
      });
    }

    const wrapped = await findWrappedKeyByDevice(ctx.db, stackUserId, args.targetDeviceId);
    if (wrapped && !wrapped.revokedAt) {
      await ctx.db.patch(wrapped._id, { revokedAt: now, updatedAt: now });
    }

    await writeAuditEvent(
      ctx,
      stackUserId,
      "device_revoked",
      {
        targetDeviceId: args.targetDeviceId,
      },
      args.actorDeviceId,
    );

    return {
      revoked: true,
      targetDeviceId: args.targetDeviceId,
    };
  },
});

export const markCurrentDeviceSeen = mutationGeneric({
  args: {
    deviceId: v.string(),
  },
  handler: async (ctx, args) => {
    const { stackUserId } = await requireStackUser(ctx);
    const device = await findDeviceById(ctx.db, stackUserId, args.deviceId);
    if (!device) {
      return { updated: false };
    }
    await ctx.db.patch(device._id, {
      lastSeenAt: Date.now(),
      updatedAt: Date.now(),
    });
    return { updated: true };
  },
});
