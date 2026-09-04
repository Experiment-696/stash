import assert from "node:assert/strict";
import test from "node:test";

import {
  canApplyVttLoad,
  isSuccessfulVttResponse,
} from "./vtt-thumbnails-lifecycle.js";

test("accepts only successful HTTP VTT responses", () => {
  assert.equal(isSuccessfulVttResponse(200), true);
  assert.equal(isSuccessfulVttResponse(206), true);
  assert.equal(isSuccessfulVttResponse(0), false);
  assert.equal(isSuccessfulVttResponse(404), false);
  assert.equal(isSuccessfulVttResponse(500), false);
});

test("ignores stale or disposed-player VTT completions", () => {
  assert.equal(canApplyVttLoad(3, 3, false), true);
  assert.equal(canApplyVttLoad(2, 3, false), false);
  assert.equal(canApplyVttLoad(3, 3, true), false);
});
