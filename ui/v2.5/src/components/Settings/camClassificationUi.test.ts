// @ts-nocheck -- node:test types are intentionally outside the browser tsconfig.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
  classificationApplyConfirmation,
  classificationExamples,
  classificationCountsLabel,
  classificationDraftError,
} from "./camClassificationUi.js";
test("classification draft reports invalid regular expressions", () => {
  assert.match(
    classificationDraftError({
      name: "capture",
      pattern: "[",
      category: "RECORDED",
    }) ?? "",
    /Invalid regular expression/
  );
  assert.equal(
    classificationDraftError({
      name: "capture",
      pattern: "^.+\\.mp4$",
      category: "RECORDED",
    }),
    undefined
  );
});
test("preview and confirmation communicate conflicts and metadata-only idempotency", () => {
  assert.equal(
    classificationCountsLabel({
      matched: 4,
      applied: 0,
      skipped: 2,
      conflicted: 2,
    }),
    "4 matched | 0 applied | 2 skipped | 2 conflicts"
  );
  assert.match(classificationApplyConfirmation, /metadata only/);
  assert.match(
    classificationApplyConfirmation,
    /never renamed, rewritten, or deleted/
  );
  assert.match(classificationApplyConfirmation, /idempotent/);
  assert.equal(
    classificationExamples.basename,
    String.raw`^\d{4}-\d{2}-\d{2} \d{2}-\d{2}-\d{2}\.mp4$`
  );
  assert.equal(
    classificationExamples.relativePath,
    String.raw`^captures/2026/.*\.mp4$`
  );
});

test("classification settings live under Library and keep the legacy deep link", () => {
  const settings = readFileSync("src/components/Settings/Settings.tsx", "utf8");
  const library = readFileSync(
    "src/components/Settings/SettingsLibraryPanel.tsx",
    "utf8"
  );
  assert.doesNotMatch(settings, /eventKey="cam-shows"/);
  assert.match(settings, /tab === "cam-shows" && isAdmin/);
  assert.match(settings, /search: "tab=library"/);
  assert.match(library, /id="cam-show-classification"/);
  assert.match(library, /<SettingsCamClassificationPanel \/>/);
});

test("owner repair exposes real Shows, creatable tags, persistent completion, and draft recovery", () => {
  const panel = readFileSync("src/components/Settings/SettingsCamClassificationPanel.tsx", "utf8");
  const shows = readFileSync("src/components/CamShows/ShowsPage.tsx", "utf8");
  const client = readFileSync("src/core/createClient.ts", "utf8");
  assert.match(panel, /creatable/);
  assert.match(panel, /rule\.tags\.map/);
  assert.match(panel, /classification-apply-complete/);
  assert.match(panel, /total\s+Shows/);
  assert.match(panel, /localStorage\.setItem\("cam-classification-draft"/);
  assert.match(shows, /useCamShowsQuery/);
  assert.match(shows, /Open Scene player/);
  assert.match(client, /sessionExpired/);
});

test("Shows UI uses the first-class domain and Scene only as player bridge", () => {
  const page = readFileSync("src/components/CamShows/ShowsPage.tsx", "utf8");
  const schema = readFileSync("../../graphql/schema/types/cam-show-classification.graphql", "utf8");
  for (const text of ["showType", "showDate", "capturedAt", "capturedTimezone", "durationSeconds", "durationOverridden", "details", "sites", "links", "models", "Open Scene player"]) assert.match(page, new RegExp(text));
  for (const type of ["LIVE_PUBLIC", "LIVE_GROUP_TICKET_MULTIUSER", "LIVE_PRIVATE", "LIVE_EXCLUSIVE_PRIVATE", "CUSTOM_VIDEO", "PRIVATE_CALL"]) assert.match(readFileSync("../../pkg/sqlite/migrations/92_cam_show_domain_correction.up.sql", "utf8"), new RegExp(type));
  assert.doesNotMatch(page, /studio|director|galleries|groups|stash.?ids/i);
  assert.match(schema, /CamShowDomainSite/);
  assert.match(schema, /CamShowDomainModel/);
});

test("Cam Show editor is capability gated and metadata-only",()=>{const page=readFileSync("src/components/CamShows/ShowsPage.tsx","utf8"),ops=readFileSync("graphql/cam-classification.graphql","utf8");for(const text of ["canManageCamModels","cam-show-editor","Edit Show","Save Show metadata","database metadata changes","never renamed or modified", "Recording time precision", "HOUR", "recording hour is known"])assert.match(page,new RegExp(text));assert.match(ops,/mutation CamShowUpdate/);});
