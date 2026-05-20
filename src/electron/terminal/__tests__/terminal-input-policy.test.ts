import { describe, expect, it } from "vitest";

import { normalizeTerminalAttachInput } from "../terminal-input-policy";

describe("terminal input policy", () => {
  it("allows empty input to attach terminal output listeners", () => {
    expect(normalizeTerminalAttachInput("")).toBe("");
    expect(normalizeTerminalAttachInput(undefined)).toBe("");
  });

  it("rejects raw command input so terminal execution uses approvals", () => {
    expect(() => normalizeTerminalAttachInput("ls")).toThrow(/Raw terminal input is disabled/);
    expect(() => normalizeTerminalAttachInput("\r")).toThrow(/Raw terminal input is disabled/);
    expect(() => normalizeTerminalAttachInput("echo unsafe\n")).toThrow(
      /Raw terminal input is disabled/,
    );
  });
});
