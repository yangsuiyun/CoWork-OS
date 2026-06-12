import { describe, expect, it, vi } from "vitest";
import { maybeApplyQualityPasses } from "../executor-llm-turn-utils";

describe("maybeApplyQualityPasses", () => {
  it("keeps the original response when the quality pass result is not accepted", async () => {
    const response = {
      stopReason: "end_turn",
      content: [{ type: "text", text: "Original draft" }],
    };

    const result = await maybeApplyQualityPasses({
      response,
      enabled: true,
      contextLabel: "follow-up 2",
      userIntent: "Review again",
      getQualityPassCount: () => 2,
      extractTextFromLLMContent: (content) =>
        (content || [])
          .filter((item: Any) => item.type === "text")
          .map((item: Any) => item.text)
          .join("\n"),
      applyQualityPassesToDraft: vi.fn(async () => ({
        text: 'to=run_command {"command":"git status --short"}',
        accepted: false,
      })),
    });

    expect(result).toBe(response);
  });

  it("replaces the response when the quality pass result is accepted", async () => {
    const response = {
      stopReason: "end_turn",
      content: [{ type: "text", text: "Original draft" }],
    };

    const result = await maybeApplyQualityPasses({
      response,
      enabled: true,
      contextLabel: "follow-up 2",
      userIntent: "Review again",
      getQualityPassCount: () => 2,
      extractTextFromLLMContent: (content) =>
        (content || [])
          .filter((item: Any) => item.type === "text")
          .map((item: Any) => item.text)
          .join("\n"),
      applyQualityPassesToDraft: vi.fn(async () => ({
        text: "Improved draft",
        accepted: true,
      })),
    });

    expect(result).not.toBe(response);
    expect(result.content).toEqual([{ type: "text", text: "Improved draft" }]);
    expect(result.stopReason).toBe("end_turn");
  });
});
