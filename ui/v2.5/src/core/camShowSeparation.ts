import type { SceneFilterType } from "./generated-graphql.js";

export const excludeCamShowsFromSceneLibrary = (
  sceneFilter: SceneFilterType = {}
): SceneFilterType => ({
  ...sceneFilter,
  exclude_cam_shows: true,
});
