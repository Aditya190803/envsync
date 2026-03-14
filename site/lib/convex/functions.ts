import { makeFunctionReference } from "convex/server";

export const convexFns = {
  devices: {
    listForCurrentUser: makeFunctionReference<"query">("devices:listForCurrentUser"),
    registerEnrollmentRequest: makeFunctionReference<"mutation">("devices:registerEnrollmentRequest"),
    approveEnrollmentRequest: makeFunctionReference<"mutation">("devices:approveEnrollmentRequest"),
    revokeDevice: makeFunctionReference<"mutation">("devices:revokeDevice"),
    markCurrentDeviceSeen: makeFunctionReference<"mutation">("devices:markCurrentDeviceSeen"),
  },
  vault: {
    getWrappedKeyForCurrentDevice: makeFunctionReference<"query">("vault:getWrappedKeyForCurrentDevice"),
    upsertWrappedKeyForDevice: makeFunctionReference<"mutation">("vault:upsertWrappedKeyForDevice"),
    getEncryptedSnapshot: makeFunctionReference<"query">("vault:getEncryptedSnapshot"),
    putEncryptedSnapshot: makeFunctionReference<"mutation">("vault:putEncryptedSnapshot"),
  },
};
