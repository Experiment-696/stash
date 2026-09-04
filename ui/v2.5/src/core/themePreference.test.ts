import assert from "node:assert/strict";
import test from "node:test";
import {
  selectedThemeID,
  selectedThemeStylesheetPath,
} from "./themePreference.js";

const themes = [
  { id: "dark-theme", name: "Dark Theme" },
  { id: "light theme", name: "Light Theme" },
];

test("only derives an internal stylesheet for a server-catalogued theme", () => {
  assert.equal(selectedThemeID("dark-theme", themes), "dark-theme");
  assert.equal(
    selectedThemeStylesheetPath("dark-theme", themes),
    "theme.css"
  );
  assert.equal(
    selectedThemeStylesheetPath("light theme", themes),
    "theme.css"
  );
  for (const invalid of [undefined, null, "", "../admin", "https://evil.test"]) {
    assert.equal(selectedThemeStylesheetPath(invalid, themes), undefined);
  }
});

test("removed themes resolve to the global theme", () => {
  assert.equal(selectedThemeID("dark-theme", []), undefined);
  assert.equal(selectedThemeStylesheetPath("dark-theme", []), undefined);
});
