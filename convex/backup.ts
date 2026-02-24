import { mutationGeneric, queryGeneric } from "convex/server";
import { v } from "convex/values";

const STORE_KEY = "global";

function assertApiKey(apiKey?: string) {
  const expected = process.env.ENVSYNC_CONVEX_API_KEY;
  if (!expected) {
    return;
  }
  if (!apiKey || apiKey !== expected) {
    throw new Error("unauthorized");
  }
}

export const getStore = queryGeneric({
  args: {
    apiKey: v.optional(v.string()),
  },
  handler: async (ctx, args) => {
    assertApiKey(args.apiKey);
    const doc = await ctx.db
      .query("backups")
      .withIndex("by_key", (q) => q.eq("key", STORE_KEY))
      .unique();
    if (!doc) {
      return { version: 1, revision: 0, projects: {} };
    }
    return doc.store;
  },
});

export const putStore = mutationGeneric({
  args: {
    apiKey: v.optional(v.string()),
    expectedRevision: v.number(),
    store: v.any(),
  },
  handler: async (ctx, args) => {
    assertApiKey(args.apiKey);
    const existing = await ctx.db
      .query("backups")
      .withIndex("by_key", (q) => q.eq("key", STORE_KEY))
      .unique();

    const currentRevision = existing?.store?.revision ?? 0;
    if (currentRevision !== args.expectedRevision) {
      throw new Error(
        `revision conflict: expected ${args.expectedRevision}, got ${currentRevision}`,
      );
    }

    const nextStore = {
      ...args.store,
      revision: currentRevision + 1,
    };

    const next = {
      key: STORE_KEY,
      store: nextStore,
      updatedAt: Date.now(),
    };

    if (existing) {
      await ctx.db.patch(existing._id, next);
      return { ok: true, id: existing._id };
    }

    const id = await ctx.db.insert("backups", next);
    return { ok: true, id };
  },
});
