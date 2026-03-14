import { defineSchema, defineTable } from "convex/server";
import { v } from "convex/values";

export default defineSchema({
  user_profiles: defineTable({
    stackUserId: v.string(),
    primaryEmail: v.optional(v.string()),
    createdAt: v.number(),
    updatedAt: v.number(),
  }).index("by_stack_user_id", ["stackUserId"]),

  devices: defineTable({
    stackUserId: v.string(),
    deviceId: v.string(),
    displayName: v.string(),
    publicKey: v.string(),
    keyAlgorithm: v.string(),
    status: v.union(v.literal("pending"), v.literal("approved"), v.literal("revoked")),
    createdAt: v.number(),
    updatedAt: v.number(),
    approvedAt: v.optional(v.number()),
    revokedAt: v.optional(v.number()),
    lastSeenAt: v.optional(v.number()),
  })
    .index("by_stack_user_id", ["stackUserId"])
    .index("by_stack_user_id_device_id", ["stackUserId", "deviceId"])
    .index("by_stack_user_id_status", ["stackUserId", "status"]),

  enrollment_requests: defineTable({
    stackUserId: v.string(),
    targetDeviceId: v.string(),
    targetDeviceName: v.string(),
    targetPublicKey: v.string(),
    targetKeyAlgorithm: v.string(),
    requesterDeviceId: v.optional(v.string()),
    status: v.union(v.literal("pending"), v.literal("approved"), v.literal("rejected")),
    recoveryUsed: v.optional(v.boolean()),
    approvedByDeviceId: v.optional(v.string()),
    wrappedVaultKeyB64: v.optional(v.string()),
    wrapperAlgorithm: v.optional(v.string()),
    createdAt: v.number(),
    updatedAt: v.number(),
    approvedAt: v.optional(v.number()),
  })
    .index("by_stack_user_id_status", ["stackUserId", "status"])
    .index("by_stack_user_id_target_device", ["stackUserId", "targetDeviceId"]),

  wrapped_vault_keys: defineTable({
    stackUserId: v.string(),
    deviceId: v.string(),
    wrappedVaultKeyB64: v.string(),
    wrapperAlgorithm: v.string(),
    keyVersion: v.number(),
    createdAt: v.number(),
    updatedAt: v.number(),
    revokedAt: v.optional(v.number()),
  })
    .index("by_stack_user_id_device_id", ["stackUserId", "deviceId"])
    .index("by_stack_user_id", ["stackUserId"]),

  encrypted_snapshots: defineTable({
    stackUserId: v.string(),
    project: v.string(),
    revision: v.number(),
    payload: v.any(),
    saltB64: v.optional(v.string()),
    keyCheckB64: v.optional(v.string()),
    updatedByDeviceId: v.optional(v.string()),
    updatedAt: v.number(),
  }).index("by_stack_user_id_project", ["stackUserId", "project"]),

  audit_events: defineTable({
    stackUserId: v.string(),
    actorDeviceId: v.optional(v.string()),
    action: v.string(),
    metadata: v.any(),
    createdAt: v.number(),
  }).index("by_stack_user_id_created_at", ["stackUserId", "createdAt"]),
});
