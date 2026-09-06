import { describe, expect, it } from "vitest";
import { parsePublicRuntimeConfig } from "./runtimeConfig";

describe("parsePublicRuntimeConfig", () => {
  it("accepts a public API URL without preserving a trailing slash", () => {
    expect(parsePublicRuntimeConfig({ apiBaseURL: "https://api.example.test/", authAuthority: "" })).toEqual({
      apiBaseURL: "https://api.example.test",
      authAuthority: "",
    });
  });

  it("rejects credentials in browser configuration", () => {
    expect(() => parsePublicRuntimeConfig({ apiBaseURL: "https://user:secret@example.test", authAuthority: "" })).toThrow(
      "public_runtime_config_invalid_apiBaseURL",
    );
  });
});
