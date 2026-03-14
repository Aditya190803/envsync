const DEVICE_STORAGE_KEY = "envsync.device.identity.v1";
const VAULT_KEY_STORAGE_KEY = "envsync.vault.key.v1";

export type LocalDeviceIdentity = {
  deviceId: string;
  displayName: string;
  keyAlgorithm: "RSA-OAEP-SHA256";
  publicKeySpkiB64: string;
  privateKeyPkcs8B64: string;
  createdAt: number;
};

function assertBrowser() {
  if (typeof window === "undefined" || !window.localStorage || !window.crypto?.subtle) {
    throw new Error("browser_crypto_unavailable");
  }
}

function toBase64(bytes: Uint8Array) {
  let bin = "";
  for (let i = 0; i < bytes.length; i++) {
    bin += String.fromCharCode(bytes[i]);
  }
  return btoa(bin);
}

function fromBase64(value: string) {
  const bin = atob(value);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) {
    out[i] = bin.charCodeAt(i);
  }
  return out;
}

function deriveDefaultDeviceName() {
  const ua = navigator.userAgent || "Unknown device";
  if (ua.includes("Mac")) return "Mac device";
  if (ua.includes("Linux")) return "Linux device";
  if (ua.includes("Windows")) return "Windows device";
  return "Browser device";
}

async function generateDeviceIdentity(): Promise<LocalDeviceIdentity> {
  const keyPair = await window.crypto.subtle.generateKey(
    {
      name: "RSA-OAEP",
      modulusLength: 2048,
      publicExponent: new Uint8Array([1, 0, 1]),
      hash: "SHA-256",
    },
    true,
    ["encrypt", "decrypt"],
  );

  const publicKey = new Uint8Array(await window.crypto.subtle.exportKey("spki", keyPair.publicKey));
  const privateKey = new Uint8Array(await window.crypto.subtle.exportKey("pkcs8", keyPair.privateKey));

  return {
    deviceId: window.crypto.randomUUID(),
    displayName: deriveDefaultDeviceName(),
    keyAlgorithm: "RSA-OAEP-SHA256",
    publicKeySpkiB64: toBase64(publicKey),
    privateKeyPkcs8B64: toBase64(privateKey),
    createdAt: Date.now(),
  };
}

export async function getOrCreateLocalDeviceIdentity(): Promise<LocalDeviceIdentity> {
  assertBrowser();
  const raw = window.localStorage.getItem(DEVICE_STORAGE_KEY);
  if (raw) {
    const parsed = JSON.parse(raw) as LocalDeviceIdentity;
    if (parsed?.deviceId && parsed.publicKeySpkiB64 && parsed.privateKeyPkcs8B64) {
      return parsed;
    }
  }

  const identity = await generateDeviceIdentity();
  window.localStorage.setItem(DEVICE_STORAGE_KEY, JSON.stringify(identity));
  return identity;
}

export async function getOrCreateVaultKeyB64() {
  assertBrowser();
  const existing = window.localStorage.getItem(VAULT_KEY_STORAGE_KEY);
  if (existing) {
    return existing;
  }
  const key = new Uint8Array(32);
  window.crypto.getRandomValues(key);
  const encoded = toBase64(key);
  window.localStorage.setItem(VAULT_KEY_STORAGE_KEY, encoded);
  return encoded;
}

export function getStoredVaultKeyB64() {
  assertBrowser();
  return window.localStorage.getItem(VAULT_KEY_STORAGE_KEY);
}

export function setStoredVaultKeyB64(value: string) {
  assertBrowser();
  window.localStorage.setItem(VAULT_KEY_STORAGE_KEY, value);
}

export async function wrapVaultKeyForDevicePublicKey(
  targetPublicKeySpkiB64: string,
  options?: { allowGenerate?: boolean },
) {
  assertBrowser();
  const allowGenerate = options?.allowGenerate ?? true;
  const existing = getStoredVaultKeyB64();
  if (!existing && !allowGenerate) {
    throw new Error("vault_key_missing");
  }
  const vaultKeyB64 = existing ?? (await getOrCreateVaultKeyB64());
  const publicKey = await window.crypto.subtle.importKey(
    "spki",
    fromBase64(targetPublicKeySpkiB64),
    {
      name: "RSA-OAEP",
      hash: "SHA-256",
    },
    false,
    ["encrypt"],
  );

  const wrapped = await window.crypto.subtle.encrypt(
    {
      name: "RSA-OAEP",
    },
    publicKey,
    fromBase64(vaultKeyB64),
  );

  return {
    wrappedVaultKeyB64: toBase64(new Uint8Array(wrapped)),
    wrapperAlgorithm: "RSA-OAEP-SHA256",
    keyVersion: 1,
  };
}

export async function unwrapVaultKeyForLocalDevice(identity: LocalDeviceIdentity, wrappedVaultKeyB64: string) {
  assertBrowser();
  const privateKey = await window.crypto.subtle.importKey(
    "pkcs8",
    fromBase64(identity.privateKeyPkcs8B64),
    {
      name: "RSA-OAEP",
      hash: "SHA-256",
    },
    false,
    ["decrypt"],
  );

  const unwrapped = await window.crypto.subtle.decrypt(
    {
      name: "RSA-OAEP",
    },
    privateKey,
    fromBase64(wrappedVaultKeyB64),
  );

  const value = toBase64(new Uint8Array(unwrapped));
  setStoredVaultKeyB64(value);
  return value;
}

export function formatRelativeTime(timestamp: number | null | undefined) {
  if (!timestamp) {
    return "-";
  }
  const delta = Math.max(0, Date.now() - timestamp);
  const mins = Math.floor(delta / (60 * 1000));
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}
