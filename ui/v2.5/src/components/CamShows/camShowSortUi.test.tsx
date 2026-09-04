import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { act, create, ReactTestRenderer } from "react-test-renderer";
import {
  CamShowSortControl,
  camShowSortFromSearch,
  camShowSortModes,
  camShowSortSearch,
  uniqueCamShows,
} from "./camShowSortUi.js";

test("sort control defaults, selects enum, resets, and reports loading/error", () => {
  const selected: string[] = [];
  let view: ReactTestRenderer;
  act(() => {
    view = create(
      <CamShowSortControl
        sort={camShowSortModes.Default}
        onChange={(value) => {
          selected.push(value);
        }}
        loading={false}
      />
    );
  });
  const select = view!.root.findByProps({ "aria-label": "Sort Shows" });
  assert.equal(select.props.value, camShowSortModes.Default);
  act(() =>
    select.props.onChange({
      currentTarget: { value: camShowSortModes.FavoriteModelsFirst },
    })
  );
  assert.deepEqual(selected, [camShowSortModes.FavoriteModelsFirst]);

  act(() => {
    view!.update(
      <CamShowSortControl
        sort={camShowSortModes.FavoriteModelsFirst}
        onChange={(value) => {
          selected.push(value);
        }}
        loading={true}
        error="Unable to load sorted Shows."
      />
    );
  });
  assert.equal(
    view!.root.findByProps({ "aria-label": "Sort Shows" }).props.disabled,
    true
  );
  assert.equal(
    view!.root.findAll(
      (node) => typeof node.type === "string" && node.props.role === "status"
    ).length,
    1
  );
  assert.equal(
    view!.root.findAll(
      (node) => typeof node.type === "string" && node.props.role === "alert"
    ).length,
    1
  );

  act(() => {
    view!.update(
      <CamShowSortControl
        sort={camShowSortModes.FavoriteModelsFirst}
        onChange={(value) => {
          selected.push(value);
        }}
        loading={false}
      />
    );
  });
  const reset = view!.root.findAllByType("button")[0];
  act(() => reset.props.onClick());
  assert.equal(selected.at(-1), camShowSortModes.Default);
});

test("Shows query sends only the sort enum and has no client user identity", () => {
  const page = readFileSync(
    new URL("./ShowsPage.tsx", import.meta.url),
    "utf8"
  );
  const operation = readFileSync(
    new URL("../../../graphql/cam-classification.graphql", import.meta.url),
    "utf8"
  );
  const generatedClient = readFileSync(
    new URL("../../core/generated-graphql.ts", import.meta.url),
    "utf8"
  );

  assert.match(page, /variables:\s*\{ sort \}/);
  assert.doesNotMatch(page, /variables:\s*\{[^}]*user(?:ID|Id|_id)/i);
  assert.match(operation, /\$sort:\s*CamShowSortMode!\s*=\s*DEFAULT/);
  assert.match(operation, /camShows\(sort:\s*\$sort\)/);
  assert.doesNotMatch(operation, /user(?:ID|Id|_id)/i);

  const generatedSortMode = generatedClient.match(
    /export enum CamShowSortMode \{\s*Default = '([^']+)',\s*FavoriteModelsFirst = '([^']+)'\s*\}/
  );
  assert.ok(generatedSortMode, "generated CamShowSortMode must exist");
  assert.equal(camShowSortModes.Default, generatedSortMode[1]);
  assert.equal(camShowSortModes.FavoriteModelsFirst, generatedSortMode[2]);
});

test("URL state preserves normal query parameters and reset restores default", () => {
  assert.equal(camShowSortFromSearch(""), camShowSortModes.Default);
  const favoriteSearch = camShowSortSearch(
    "?tab=shows",
    camShowSortModes.FavoriteModelsFirst
  );
  assert.equal(
    camShowSortFromSearch(favoriteSearch),
    camShowSortModes.FavoriteModelsFirst
  );
  assert.match(favoriteSearch, /tab=shows/);
  assert.equal(
    camShowSortSearch(favoriteSearch, camShowSortModes.Default),
    "?tab=shows"
  );
  assert.equal(
    camShowSortFromSearch("?sort=client-user-id"),
    camShowSortModes.Default
  );
});

test("duplicate server rows render through one unique card input", () => {
  const shows = [{ id: "1" }, { id: "2" }, { id: "1" }];
  assert.deepEqual(
    uniqueCamShows(shows).map((show) => show.id),
    ["1", "2"]
  );
});
