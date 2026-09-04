import assert from "node:assert/strict";
import test from "node:test";
import React, { useState } from "react";
import { Alert, Button } from "react-bootstrap";
import {
  act,
  create,
  ReactTestInstance,
  ReactTestRenderer,
} from "react-test-renderer";
import { CamModelConfirmedAction } from "./CamModelConfirmedAction.js";
import { CamGirlFinderCandidateSelection } from "./CamGirlFinderCandidateSelection.js";
import type { CamGirlFinderCandidate } from "../../core/generated-graphql.js";

const textOfNode = (node: React.ReactNode): string => {
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(textOfNode).join("");
  return React.isValidElement(node) ? textOfNode(node.props.children) : "";
};
const textOf = (node: ReactTestInstance): string =>
  node.children
    .map((child) => (typeof child === "string" ? child : textOf(child)))
    .join("");
const button = (view: ReactTestRenderer, label: string) =>
  view.root
    .findAllByType("button")
    .find((item) => textOf(item).includes(label))!;
const dialog = (view: ReactTestRenderer, testID: string) =>
  view.root.findByProps({ "data-testid": `${testID}-dialog` });
const elements = (node: React.ReactNode): React.ReactElement[] => {
  if (Array.isArray(node)) return node.flatMap(elements);
  if (!React.isValidElement(node)) return [];
  return [node, ...elements(node.props.children)];
};
const modalButton = (view: ReactTestRenderer, testID: string, label: string) =>
  elements(dialog(view, testID).props.children).find(
    (item) =>
      item.type === Button && textOfNode(item.props.children).includes(label)
  )!;

const cases = [
  {
    testID: "cam-girl-finder-apply",
    title: "Add selected CamGirlFinder evidence?",
    description:
      "Add 2 selected candidates as pending review evidence. This creates pending evidence only and never merges identity.",
    trigger: "Add 2 selected as pending evidence",
    confirm: "Add 2 as pending evidence",
  },
  {
    testID: "cam-model-archive-social-7",
    title: "Move social/media profile to history?",
    description:
      "Archive the social/media profile while preserving provenance and history.",
    trigger: "Archive social",
    confirm: "Move profile to history",
  },
  {
    testID: "cam-model-retire-account-9",
    title: "Move username to history?",
    description:
      "Mark this username historical. No identity is merged or deleted.",
    trigger: "Retire account",
    confirm: "Move username to history",
  },
];

for (const item of cases) {
  test(`${item.testID} requires its visible accessible modal; Cancel is zero mutation and Confirm is exactly once`, async () => {
    let calls = 0;
    const view = create(
      <CamModelConfirmedAction
        testID={item.testID}
        title={item.title}
        description={item.description}
        triggerLabel={item.trigger}
        confirmLabel={item.confirm}
        onConfirm={async () => {
          calls += 1;
        }}
      />
    );
    assert.equal(dialog(view, item.testID).props.show, false);
    act(() => button(view, item.trigger).props.onClick());
    assert.equal(
      calls,
      0,
      "opening confirmation must never invoke the mutation"
    );
    const mountedDialog = dialog(view, item.testID);
    assert.equal(mountedDialog.props.role, "dialog");
    assert.equal(mountedDialog.props["aria-label"], item.title);
    assert.match(
      textOfNode(mountedDialog.props.children),
      new RegExp(item.description.split(".")[0])
    );
    act(() => modalButton(view, item.testID, "Cancel").props.onClick());
    assert.equal(calls, 0);
    assert.equal(dialog(view, item.testID).props.show, false);
    act(() => button(view, item.trigger).props.onClick());
    await act(async () =>
      modalButton(view, item.testID, item.confirm).props.onClick()
    );
    assert.equal(calls, 1);
    assert.equal(dialog(view, item.testID).props.show, false);
  });
}

test("Apply Confirm invokes exactly once while pending", async () => {
  let calls = 0;
  let resolve!: () => void;
  const pending = new Promise<void>((done) => {
    resolve = done;
  });
  const view = create(
    <CamModelConfirmedAction
      testID="cam-girl-finder-apply"
      title="Add selected CamGirlFinder evidence?"
      description="Pending evidence only; no identity merge."
      triggerLabel="Add 2 selected as pending evidence"
      confirmLabel="Add 2 as pending evidence"
      onConfirm={() => {
        calls += 1;
        return pending;
      }}
    />
  );
  act(() => button(view, "Add 2 selected").props.onClick());
  await act(async () => {
    modalButton(
      view,
      "cam-girl-finder-apply",
      "Add 2 as pending evidence"
    ).props.onClick();
    modalButton(
      view,
      "cam-girl-finder-apply",
      "Add 2 as pending evidence"
    ).props.onClick();
  });
  assert.equal(calls, 1);
  const modalButtons = elements(
    dialog(view, "cam-girl-finder-apply").props.children
  ).filter((item) => item.type === Button);
  assert.ok(modalButtons.every((item) => item.props.disabled));
  await act(async () => resolve());
  assert.equal(dialog(view, "cam-girl-finder-apply").props.show, false);
});

const candidates: CamGirlFinderCandidate[] = [
  {
    evidenceKey: "first",
    platform: "mfc",
    username: "Alice",
    sourceURL: "",
    observedAt: "2026-07-21T00:00:00Z",
    payloadJSON: "{}",
  },
  {
    evidenceKey: "second",
    platform: "lj",
    username: "AliceK",
    sourceURL: "",
    observedAt: "2026-07-21T00:00:00Z",
    payloadJSON: "{}",
  },
];

const ErrorHarness: React.FC = () => {
  const [selected, setSelected] = useState<string[]>([]);
  return (
    <>
      <CamGirlFinderCandidateSelection
        items={candidates}
        selected={selected}
        setSelected={setSelected}
      />
      <CamModelConfirmedAction
        testID="cam-girl-finder-apply-error"
        title="Add selected CamGirlFinder evidence?"
        description={`${selected.length} selected; pending evidence only; no identity merge.`}
        triggerLabel={`Add ${selected.length} selected`}
        confirmLabel="Confirm selected evidence"
        disabled={selected.length === 0}
        onConfirm={async () => {
          throw new Error("network failed");
        }}
      />
    </>
  );
};

test("Apply error keeps the real first and second checkbox selections and modal visible", async () => {
  const view = create(<ErrorHarness />);
  let inputs = view.root.findAllByType("input");
  act(() => inputs[0].props.onChange({ currentTarget: { checked: true } }));
  inputs = view.root.findAllByType("input");
  act(() => inputs[1].props.onChange({ currentTarget: { checked: true } }));
  act(() => button(view, "Add 2 selected").props.onClick());
  await act(async () =>
    modalButton(
      view,
      "cam-girl-finder-apply-error",
      "Confirm selected evidence"
    ).props.onClick()
  );
  assert.deepEqual(
    view.root.findAllByType("input").map((item) => item.props.checked),
    [true, true]
  );
  const openDialog = dialog(view, "cam-girl-finder-apply-error");
  assert.equal(
    openDialog.props.show,
    true,
    "rejected Confirm must leave the modal visibly open"
  );
  assert.equal(openDialog.props.role, "dialog");
  assert.equal(
    openDialog.props["aria-label"],
    "Add selected CamGirlFinder evidence?"
  );
  assert.equal(openDialog.props["aria-modal"], true);
  assert.equal(openDialog.props.keyboard, true);
  assert.equal(openDialog.props.backdrop, "static");
  assert.equal(openDialog.props.autoFocus, true);
  assert.equal(openDialog.props.enforceFocus, true);
  assert.equal(openDialog.props.restoreFocus, true);
  const errorAlert = elements(openDialog.props.children).find(
    (item) => item.type === Alert
  )!;
  assert.equal(errorAlert.props.role, "alert");
  assert.equal(errorAlert.props["aria-live"], "assertive");
  assert.match(textOfNode(errorAlert.props.children), /network failed/);
  assert.equal(
    modalButton(
      view,
      "cam-girl-finder-apply-error",
      "Confirm selected evidence"
    ).props.disabled,
    false
  );
});
