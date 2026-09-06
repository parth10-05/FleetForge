export type PublicRuntimeConfig = Readonly<{
  apiBaseURL: string;
  authAuthority: string;
}>;

const runtimeConfigPath = "/runtime-config.json";

export async function loadRuntimeConfig(fetcher: typeof fetch = fetch): Promise<PublicRuntimeConfig> {
  const response = await fetcher(runtimeConfigPath, { cache: "no-store" });
  if (!response.ok) {
    throw new Error("public_runtime_config_unavailable");
  }

  return parsePublicRuntimeConfig(await response.json());
}

export function parsePublicRuntimeConfig(value: unknown): PublicRuntimeConfig {
  if (!isRecord(value) || typeof value.apiBaseURL !== "string" || typeof value.authAuthority !== "string") {
    throw new Error("public_runtime_config_invalid");
  }

  return {
    apiBaseURL: publicURL("apiBaseURL", value.apiBaseURL),
    authAuthority: value.authAuthority === "" ? "" : publicURL("authAuthority", value.authAuthority),
  };
}

function publicURL(name: string, raw: string): string {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    throw new Error(`public_runtime_config_invalid_${name}`);
  }
  if ((url.protocol !== "https:" && url.protocol !== "http:") || url.username || url.password) {
    throw new Error(`public_runtime_config_invalid_${name}`);
  }
  return url.toString().replace(/\/$/, "");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
