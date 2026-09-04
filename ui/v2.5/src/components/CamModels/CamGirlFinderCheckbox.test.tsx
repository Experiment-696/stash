import assert from "node:assert/strict";
import test from "node:test";
import React, { useState } from "react";
import { act, create, ReactTestRenderer } from "react-test-renderer";
import { CamGirlFinderCandidateSelection } from "./CamGirlFinderCandidateSelection.js";
import type { CamGirlFinderCandidate } from "../../core/generated-graphql.js";

const candidates: CamGirlFinderCandidate[] = [
  {
    evidenceKey: "first",
    platform: "cb",
    username: "Alice",
    sourceURL: "",
    observedAt: "2026-07-21T00:00:00Z",
    payloadJSON: "{}",
  },
  {
    evidenceKey: "second",
    platform: "mfc",
    username: "AliceTwo",
    sourceURL: "",
    observedAt: "2026-07-21T00:00:00Z",
    payloadJSON: "{}",
  },
];

const Harness: React.FC<{ revision: number }> = () => {
  const [selected, setSelected] = useState<string[]>([]);
  return (
    <CamGirlFinderCandidateSelection
      items={candidates}
      selected={selected}
      setSelected={setSelected}
    />
  );
};

test("rendered CamGirlFinder checkboxes select first and second and survive rerender", () => {
  let view: ReactTestRenderer;
  assert.doesNotThrow(() => {
    act(() => {
      view = create(<Harness revision={0} />);
    });
  });
  let inputs = view!.root.findAllByType("input");
  assert.equal(inputs.length, 2);
  assert.deepEqual(
    inputs.map((input) => input.props.checked),
    [false, false]
  );

  const firstEvent = { currentTarget: { checked: true } };
  assert.doesNotThrow(() => act(() => inputs[0].props.onChange(firstEvent)));
  firstEvent.currentTarget = null as never;
  inputs = view!.root.findAllByType("input");
  assert.deepEqual(
    inputs.map((input) => input.props.checked),
    [true, false]
  );

  const secondEvent = { currentTarget: { checked: true } };
  assert.doesNotThrow(() => act(() => inputs[1].props.onChange(secondEvent)));
  secondEvent.currentTarget = null as never;
  inputs = view!.root.findAllByType("input");
  assert.deepEqual(
    inputs.map((input) => input.props.checked),
    [true, true]
  );

  assert.doesNotThrow(() => act(() => view!.update(<Harness revision={1} />)));
  inputs = view!.root.findAllByType("input");
  assert.deepEqual(
    inputs.map((input) => input.props.checked),
    [true, true]
  );
});
