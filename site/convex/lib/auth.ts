import { ConvexError } from "convex/values";

type AuthCtx = {
  auth: {
    getUserIdentity: () => Promise<{
      subject?: string;
      email?: string;
    } | null>;
  };
};

export async function requireStackUser(ctx: AuthCtx) {
  const identity = await ctx.auth.getUserIdentity();
  const subject = identity?.subject?.trim();
  if (!subject) {
    throw new ConvexError("unauthorized");
  }
  return {
    stackUserId: subject,
    email: identity?.email?.trim() || undefined,
  };
}
