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

async function requireApprovedDeviceWithKey(db: any, stackUserId: string, deviceId: string) {
  const device = await findDeviceById(db, stackUserId, deviceId);
  if (!device || device.status !== "approved") {
    throw new ConvexError("device_not_approved");
  }
  const wrapped = await findWrappedKeyByDevice(db, stackUserId, deviceId);
  if (!wrapped || wrapped.revokedAt) {
    throw new ConvexError("wrapped_vault_key_missing");
  }
  return { device, wrapped };
}

export const getWrappedKeyForCurrentDevice = queryGeneric({
  args: {
    deviceId: v.string(),
  },
  handler: async (ctx, args) => {
    const { stackUserId } = await requireStackUser(ctx);
    const { device, wrapped } = await requireApprovedDeviceWithKey(ctx.db, stackUserId, args.deviceId);

    return {
      deviceId: device.deviceId,
      wrappedVaultKeyB64: wrapped.wrappedVaultKeyB64,
      wrapperAlgorithm: wrapped.wrapperAlgorithm,
      keyVersion: wrapped.keyVersion,
      updatedAt: wrapped.updatedAt,
    };
  },
});

export const upsertWrappedKeyForDevice = mutationGeneric({
  args: {
    actorDeviceId: v.string(),
    targetDeviceId: v.string(),
    wrappedVaultKeyB64: v.string(),
    wrapperAlgorithm: v.string(),
    keyVersion: v.optional(v.number()),
  },
  handler: async (ctx, args) => {
    const { stackUserId } = await requireStackUser(ctx);
    const now = Date.now();

    const actor = await findDeviceById(ctx.db, stackUserId, args.actorDeviceId);
    if (!actor || actor.status !== "approved") {
      throw new ConvexError("actor_device_not_approved");
    }

    const target = await findDeviceById(ctx.db, stackUserId, args.targetDeviceId);
    if (!target) {
      throw new ConvexError("target_device_not_found");
    }

    const existing = await findWrappedKeyByDevice(ctx.db, stackUserId, args.targetDeviceId);
    if (!existing) {
      await ctx.db.insert("wrapped_vault_keys", {
        stackUserId,
        deviceId: args.targetDeviceId,
        wrappedVaultKeyB64: args.wrappedVaultKeyB64,
        wrapperAlgorithm: args.wrapperAlgorithm,
        keyVersion: args.keyVersion ?? 1,
        createdAt: now,
        updatedAt: now,
      });
    } else {
      await ctx.db.patch(existing._id, {
        wrappedVaultKeyB64: args.wrappedVaultKeyB64,
        wrapperAlgorithm: args.wrapperAlgorithm,
        keyVersion: args.keyVersion ?? existing.keyVersion,
        updatedAt: now,
        revokedAt: undefined,
      });
    }

    await writeAuditEvent(
      ctx,
      stackUserId,
      "wrapped_key_upserted",
      {
        targetDeviceId: args.targetDeviceId,
        keyVersion: args.keyVersion ?? 1,
      },
      args.actorDeviceId,
    );

    return {
      ok: true,
      targetDeviceId: args.targetDeviceId,
    };
  },
});

export const getEncryptedSnapshot = queryGeneric({
  args: {
    project: v.string(),
  },
  handler: async (ctx, args) => {
    const { stackUserId } = await requireStackUser(ctx);

    const snapshots = await ctx.db
      .query("encrypted_snapshots")
      .withIndex("by_stack_user_id_project", (q: any) =>
        q.eq("stackUserId", stackUserId).eq("project", args.project),
      )
      .collect();

    const current = snapshots[0];
    if (!current) {
      return {
        project: args.project,
        revision: 0,
        payload: { version: 1, revision: 0, projects: {} },
        saltB64: null,
        keyCheckB64: null,
      };
    }

    return {
      project: current.project,
      revision: current.revision,
      payload: current.payload,
      saltB64: current.saltB64 ?? null,
      keyCheckB64: current.keyCheckB64 ?? null,
      updatedByDeviceId: current.updatedByDeviceId ?? null,
      updatedAt: current.updatedAt,
    };
  },
});

export const putEncryptedSnapshot = mutationGeneric({
  args: {
    project: v.string(),
    expectedRevision: v.number(),
    payload: v.any(),
    saltB64: v.optional(v.string()),
    keyCheckB64: v.optional(v.string()),
    deviceId: v.string(),
  },
  handler: async (ctx, args) => {
    const { stackUserId } = await requireStackUser(ctx);
    const now = Date.now();

    const { device } = await requireApprovedDeviceWithKey(ctx.db, stackUserId, args.deviceId);
    await ctx.db.patch(device._id, { lastSeenAt: now, updatedAt: now });

    const snapshots = await ctx.db
      .query("encrypted_snapshots")
      .withIndex("by_stack_user_id_project", (q: any) =>
        q.eq("stackUserId", stackUserId).eq("project", args.project),
      )
      .collect();

    const current = snapshots[0];
    const currentRevision = current?.revision ?? 0;
    if (currentRevision !== args.expectedRevision) {
      throw new ConvexError(`revision_conflict:${currentRevision}`);
    }

    const nextRevision = currentRevision + 1;
    if (!current) {
      await ctx.db.insert("encrypted_snapshots", {
        stackUserId,
        project: args.project,
        revision: nextRevision,
        payload: args.payload,
        saltB64: args.saltB64,
        keyCheckB64: args.keyCheckB64,
        updatedByDeviceId: args.deviceId,
        updatedAt: now,
      });
    } else {
      await ctx.db.patch(current._id, {
        revision: nextRevision,
        payload: args.payload,
        saltB64: args.saltB64,
        keyCheckB64: args.keyCheckB64,
        updatedByDeviceId: args.deviceId,
        updatedAt: now,
      });
    }

    await writeAuditEvent(
      ctx,
      stackUserId,
      "snapshot_put",
      {
        project: args.project,
        expectedRevision: args.expectedRevision,
        nextRevision,
      },
      args.deviceId,
    );

    return {
      project: args.project,
      revision: nextRevision,
    };
  },
});
