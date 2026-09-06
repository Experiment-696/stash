import assert from "node:assert/strict";
import test from "node:test";
import { homepageRoutes, safeHomepageDestination } from "./homepagePreference.js";

test("accepts only exact stable library routes", () => {
  for (const route of homepageRoutes) {
    assert.equal(safeHomepageDestination(route), route);
  }
  for (const route of [
    "https://example.com",
    "//example.com",
    "/settings",
    "/setup",
    "/scenes?sort=date",
    "/scenes/1",
    "scenes",
    "",
    null,
  ]) {
    assert.equal(safeHomepageDestination(route), "/");
  }
});

test("root preference remains root instead of redirecting", () => {
  assert.equal(safeHomepageDestination("/"), "/");
});
