import assert from "node:assert/strict";
import test from "node:test";
import { excludeCamShowsFromSceneLibrary } from "./camShowSeparation.js";

test("ordinary Scene library queries exclude classified Cam Shows", () => {
  const input = { organized: true };
  const result = excludeCamShowsFromSceneLibrary(input);

  assert.deepEqual(input, { organized: true });
  assert.deepEqual(result, {
    organized: true,
    exclude_cam_shows: true,
  });
});

test("the ordinary Scene library cannot accidentally disable separation", () => {
  assert.equal(
    excludeCamShowsFromSceneLibrary({ exclude_cam_shows: false })
      .exclude_cam_shows,
    true
  );
});
