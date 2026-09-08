import { describe, expect, it } from "vitest";
import { normalizeSupportedModelScopesForPlatform } from "../groupsSupportedModelScopes";

describe("normalizeSupportedModelScopesForPlatform", () => {
  it("normalizes legacy scopes to Gemini text for Antigravity", () => {
    expect(
      normalizeSupportedModelScopesForPlatform("antigravity", [
        "claude",
        "gemini_text",
      ]),
    ).toEqual(["gemini_text"]);
  });

  it("defaults Antigravity to Gemini text without scopes", () => {
    expect(normalizeSupportedModelScopesForPlatform("antigravity", undefined)).toEqual(["gemini_text"]);
  });

  it("drops hidden model scopes for OpenAI groups", () => {
    expect(
      normalizeSupportedModelScopesForPlatform("openai", [
        "claude",
        "gemini_text",
        "gemini_image",
      ]),
    ).toEqual([]);
  });

  it("drops hidden model scopes for other non-Antigravity groups", () => {
    expect(normalizeSupportedModelScopesForPlatform("claude", ["claude"])).toEqual([]);
  });
});
