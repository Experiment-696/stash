// @ts-nocheck
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
test("CamGirlFinder settings use Metadata Providers and fail closed", () => {
  const settings = readFileSync(
    "src/components/Settings/SettingsScrapingPanel.tsx",
    "utf8"
  );
  const panel = readFileSync(
    "src/components/Settings/SettingsCamGirlFinderPanel.tsx",
    "utf8"
  );
  assert.ok(settings.includes("<SettingsCamGirlFinderPanel />"));
  assert.ok(panel.includes("if (!canManage) return null"));
  assert.match(panel, /skip: !canManage/);
  const compact = panel.replace(/\s+/g, " ");
  for (const text of [
    "Enable CamGirlFinder discovery",
    "100–60000 ms",
    "1–120",
    "1–100",
    "fixed official API",
    "never creates accounts, merges identity, infers history or online state, or records media",
  ])
    assert.match(compact, new RegExp(text));
  assert.doesNotMatch(panel, /api key|password|credential|base url/i);
});
