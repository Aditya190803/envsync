type AuditCtx = {
  db: {
    insert: (table: string, value: Record<string, unknown>) => Promise<unknown>;
  };
};

export async function writeAuditEvent(
  ctx: AuditCtx,
  stackUserId: string,
  action: string,
  metadata: Record<string, unknown>,
  actorDeviceId?: string,
) {
  await ctx.db.insert("audit_events", {
    stackUserId,
    action,
    metadata,
    actorDeviceId,
    createdAt: Date.now(),
  });
}
