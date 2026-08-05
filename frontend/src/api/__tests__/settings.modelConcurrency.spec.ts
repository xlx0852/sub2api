import { describe, expect, it } from "vitest";

import {
  normalizeModelConcurrencyLimits,
  parseModelConcurrencyLimits,
  serializeModelConcurrencyLimits,
} from "@/api/admin/settings";

describe("admin settings model concurrency limits helpers", () => {
  it("normalizes to positive integers only", () => {
    expect(
      normalizeModelConcurrencyLimits({
        "gpt-5.6-luna": 8,
        "gpt-5.6-sol": 0, // 非正数丢弃 = 不限制
        "gpt-5.6-terra": -3, // 负数丢弃
        "": 5, // 空键丢弃
        "  gpt-5.4  ": 4, // 首尾空格裁剪
      }),
    ).toEqual({
      "gpt-5.6-luna": 8,
      "gpt-5.4": 4,
    });
  });

  it("floors non-integer limits", () => {
    expect(normalizeModelConcurrencyLimits({ "gpt-5.6-luna": 8.7 })).toEqual({
      "gpt-5.6-luna": 8,
    });
  });

  it("returns empty map for null / non-object input", () => {
    expect(normalizeModelConcurrencyLimits(undefined)).toEqual({});
    expect(normalizeModelConcurrencyLimits(null)).toEqual({});
  });

  it("serializes to pretty JSON and round-trips", () => {
    const input = { "gpt-5.6-luna": 8 };
    const raw = serializeModelConcurrencyLimits(input);
    expect(JSON.parse(raw)).toEqual({ "gpt-5.6-luna": 8 });
    expect(parseModelConcurrencyLimits(raw)).toEqual({ "gpt-5.6-luna": 8 });
  });

  it("parse handles invalid JSON gracefully", () => {
    expect(parseModelConcurrencyLimits("{not-json")).toEqual({});
    expect(parseModelConcurrencyLimits("")).toEqual({});
    expect(parseModelConcurrencyLimits("[1,2]")).toEqual({});
  });
});
