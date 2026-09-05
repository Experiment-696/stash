// @ts-nocheck -- node:test types are intentionally outside the browser tsconfig.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
  camModelAccountPeriod,
  canManageCamModels,
  camModelAccountValidation,
  camModelProfileValidation,
} from "./camModelUi.js";

test("management capability fails closed across the role matrix", () => {
  assert.equal(
    canManageCamModels(["library.read", "data.admin"]),
    true,
    "full Admin capability"
  );
  assert.equal(
    canManageCamModels(["library.read"]),
    false,
    "reduced-scope Admin"
  );
  assert.equal(
    canManageCamModels(["library.read", "metadata.write"]),
    false,
    "Moderator"
  );
  assert.equal(canManageCamModels(["library.read"]), false, "User");
  assert.equal(canManageCamModels(undefined), false, "missing capability data");
});

test("profile and account drafts fail closed", () => {
  assert.equal(camModelProfileValidation("  "), "Display name is required.");
  assert.equal(camModelProfileValidation("Model"), undefined);
  assert.equal(camModelAccountValidation("", "name"), "Choose a site.");
  assert.equal(camModelAccountValidation("1", " "), "Username is required.");
});

test("account periods distinguish current and historical usernames", () => {
  assert.match(camModelAccountPeriod("2026-01-01T00:00:00Z", null), /Current$/);
  assert.doesNotMatch(
    camModelAccountPeriod("2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"),
    /Current/
  );
});

test("trusted UI exposes direct identity metadata without an evidence queue", () => {
  const page = readFileSync(
    "src/components/CamModels/CamModelsPage.tsx",
    "utf8"
  );
  const trusted = readFileSync("src/trustedExtensions.tsx", "utf8");
  const app = readFileSync("src/App.tsx", "utf8");
  for (const text of [
    "Loading Cam Models",
    "No Cam Model profiles yet",
    "Unable to load Cam Models",
    "No site accounts yet",
    "HISTORICAL",
    "CURRENT",
  ])
    assert.match(page, new RegExp(text));
  assert.doesNotMatch(page, /Evidence \(/);
  assert.doesNotMatch(page, /Approve evidence|Reject evidence|pending review/);
  assert.match(page, /import \{ ExternalLink \}/);
  assert.equal((page.match(/<ExternalLink href=/g) ?? []).length, 2);
  assert.doesNotMatch(page, /<a[^>]+target="_blank"/);
  assert.match(page, /canManage &&/);
  assert.doesNotMatch(page, /role\s*===\s*["'`]ADMIN/);
  for (const control of [
    "New profile",
    "Save",
    "Add site alias",
    "Move to history",
  ]) {
    assert.match(page, new RegExp(control));
  }
  assert.match(page, /Sign in with library access/);
  assert.ok(trusted.includes(`path: "/cam-models"`));
  assert.ok(trusted.includes("export { CamModelsPage }"));
  assert.ok(app.includes(`path="/cam-models/:id?"`));
  assert.doesNotMatch(page, /PollPresence|startRecording|stopRecording/);
});

test("CamGirlFinder is Admin-capability gated, cancellable, and applies metadata", () => {
  const page = readFileSync(
    "src/components/CamModels/CamModelsPage.tsx",
    "utf8"
  );
  const candidate = readFileSync(
    "src/components/CamModels/CamGirlFinderCandidateSelection.tsx",
    "utf8"
  );
  const search = readFileSync(
    "src/components/CamModels/CamGirlFinderSearchCard.tsx",
    "utf8"
  );
  assert.match(page.replace(/\s+/g, " "), /canManage && \( <Tab eventKey="finder"/);
  assert.match(search, /new AbortController/);
  assert.ok(search.includes("controller.current?.abort()"));
  const compactSearch = search.replace(/\s+/g, " ");
  for (const text of [
    "Search for this model",
    "No results loaded",
    "request cancelled",
    "Search",
    "adds the site, alias, and profile link",
    "profile picture is added",
  ])
    assert.match(compactSearch, new RegExp(text, "i"));
  assert.doesNotMatch(search, /window.confirm/);
  assert.doesNotMatch(page, /window.confirm/);
  assert.match(page, /async function archiveSocial/);
  assert.match(page, /async function retireAccount/);
  assert.match(page, /cam-model-archive-social-/);
  assert.match(page, /cam-model-retire-account-/);
  assert.match(search, /CamModelConfirmedAction/);
  assert.match(search, /Attach selected profiles/);
  assert.match(search, /evidenceKeys: selected/);
  assert.match(candidate, /selected.includes\(item.evidenceKey\)/);
  assert.match(search, /disabled={ingesting \|\| selected.length === 0}/);
  assert.match(search, /rejected with an explicit reason/);
  assert.match(candidate, /ExternalLink/);
  assert.match(candidate, /item\.imageURL/);
  assert.doesNotMatch(search, /payloadJSON/);
});

test("per-user favorites are available on list/detail without exposing Admin controls", () => {
  const page = readFileSync(
    "src/components/CamModels/CamModelsPage.tsx",
    "utf8"
  );
  const operations = readFileSync("graphql/cam-model-profiles.graphql", "utf8");
  for (const text of [
    "All Models",
    "Favorite Models",
    "No favorite Cam Models yet",
    "Favorites are private to your account",
    "FavoriteIcon",
    "useCamModelSetUserStateMutation",
  ])
    assert.match(page, new RegExp(text));
  assert.equal(
    (page.match(/<FavoriteIcon/g) ?? []).length,
    2,
    "list and detail toggles"
  );
  assert.match(operations, /camModelProfiles\(favoritesOnly:/);
  assert.match(operations, /mutation CamModelSetUserState/);
  assert.doesNotMatch(page, /data\.admin.*FavoriteIcon/);
});

test("social/media profiles are purpose-typed and capability gated", () => {
  const page = readFileSync(
      "src/components/CamModels/CamModelsPage.tsx",
      "utf8"
    ),
    ops = readFileSync("graphql/cam-model-profiles.graphql", "utf8");
  for (const text of [
    "Social and media profiles",
    "separate from cam-site accounts",
    "useCamModelSocialProfileCreateMutation",
    "useCamModelSocialProfileRetireMutation",
    "ExternalLink",
  ])
    assert.match(page, new RegExp(text));
  assert.ok(page.includes("onSubmit={(e) => void addSocial(e)}"));
  assert.match(ops, /CamModelSocialProfileCreate/);
  assert.match(ops, /CamModelSocialProfileRetire/);
});
