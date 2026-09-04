// @ts-nocheck -- node:test types are intentionally outside the browser tsconfig.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { canManageCamModels } from "./camModelUi.js";

test("Cam role UI derives management only from data.admin capability", () => {
  const roles = [
    ["active Admin", ["library.read", "data.admin"], true],
    ["Moderator", ["library.read", "metadata.write"], false],
    ["User", ["library.read"], false],
    ["reduced Admin token", ["library.read"], false],
    ["missing principal data", undefined, false],
  ] as const;
  for (const [name, capabilities, expected] of roles)
    assert.equal(canManageCamModels(capabilities), expected, name);
});

test("every Cam identity control and provider panel is hidden by the shared capability gate", () => {
  const models = readFileSync(
    "src/components/CamModels/CamModelsPage.tsx",
    "utf8"
  );
  const shows = readFileSync("src/components/CamShows/ShowsPage.tsx", "utf8");
  const finder = readFileSync(
    "src/components/Settings/SettingsCamGirlFinderPanel.tsx",
    "utf8"
  );
  const completed = readFileSync(
    "src/components/Settings/SettingsCompletedRecordingImportPanelContainer.tsx",
    "utf8"
  );
  const classification = readFileSync(
    "src/components/Settings/SettingsCamClassificationPanel.tsx",
    "utf8"
  );
  const settings = readFileSync("src/components/Settings/Settings.tsx", "utf8");

  for (const source of [models, shows, finder, completed, classification]) {
    assert.match(source, /canManageCamModels\([^)]*capabilities/);
    assert.doesNotMatch(source, /role\s*===\s*["'`]ADMIN/);
  }
  for (const control of [
    "New profile",
    "Save profile",
    "Add account",
    "Move username to history",
    "Add profile",
    "Move profile to history",
    "CamGirlFinderSearchCard",
    "Raw provider evidence",
    "Approve evidence",
    "Reject evidence",
  ]) {
    assert.match(models, new RegExp(`canManage[\\s\\S]{0,1800}${control}`));
  }
  assert.match(shows, /canManage\s*&&[\s\S]{0,300}Edit Show/);
  for (const panel of [finder, completed, classification]) {
    assert.match(panel, /skip:\s*!canManage/);
    assert.match(panel, /if \(!canManage\) return null/);
  }
  assert.match(settings, /!isAdmin && tab !== "tasks"/);
});
