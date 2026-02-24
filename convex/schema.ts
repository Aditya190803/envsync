import { defineSchema, defineTable } from "convex/server";
import { v } from "convex/values";

export default defineSchema({
  backups: defineTable({
    key: v.string(),
    store: v.any(),
    updatedAt: v.number(),
  }).index("by_key", ["key"]),
});
