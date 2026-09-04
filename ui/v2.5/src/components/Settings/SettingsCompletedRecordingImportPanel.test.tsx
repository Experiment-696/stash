import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import React from "react";
import { Alert, Button, Modal } from "react-bootstrap";
import {
  act,
  create,
  ReactTestInstance,
  ReactTestRenderer,
} from "react-test-renderer";
import type * as GQL from "../../core/generated-graphql.js";
import { SettingsCompletedRecordingImportView } from "./SettingsCompletedRecordingImportPanel.js";

const text = (node: ReactTestInstance): string =>
  node.children.map((v) => (typeof v === "string" ? v : text(v))).join("");
const elements = (node: React.ReactNode): React.ReactElement[] =>
  Array.isArray(node)
    ? node.flatMap(elements)
    : React.isValidElement(node)
    ? [node, ...elements(node.props.children)]
    : [];
const modalButton = (view: ReactTestRenderer, label: string) =>
  elements(
    view.root.findByProps({
      "data-testid": "completed-recording-confirm-dialog",
    }).props.children
  ).find((v) => v.type === Button && String(v.props.children).includes(label))!;
const button = (view: ReactTestRenderer, label: string) =>
  view.root.findAllByType(Button).find((v) => text(v).includes(label))!;
const candidateInputs = (view: ReactTestRenderer) =>
  view.root
    .findAllByType("input")
    .filter((value) =>
      String(value.props["aria-label"] ?? "").startsWith("Select ")
    );
const preview: GQL.CompletedRecordingPreview = {
  previewID: "preview-1",
  createdAt: "2026-07-21T00:00:00Z",
  expiresAt: "2026-07-21T00:05:00Z",
  scannedCount: 3,
  truncated: false,
  items: [
    {
      candidateID: "exact",
      relativePath: "safe/exact.mp4",
      platform: "site",
      username: "alice",
      timezone: "UTC",
      matchState: "CURRENT",
      outcome: "EXACT_READY" as GQL.CompletedRecordingOutcome,
    },
    {
      candidateID: "review",
      relativePath: "safe/review.mp4",
      platform: "site",
      username: "old",
      timezone: "UTC",
      matchState: "HISTORICAL",
      outcome: "REVIEW_REQUIRED" as GQL.CompletedRecordingOutcome,
      reviewCode:
        "HISTORICAL_ALIAS_REUSED" as GQL.CompletedRecordingReviewReason,
    },
    {
      candidateID: "skip",
      relativePath: "safe/missing.mp4",
      platform: "site",
      username: "none",
      timezone: "UTC",
      matchState: "NONE",
      outcome: "MODEL_NOT_FOUND" as GQL.CompletedRecordingOutcome,
    },
  ],
};

const mount = (
  apply: (
    input: GQL.CompletedRecordingApplyInput
  ) => Promise<GQL.CompletedRecordingApplyResult[]> = async () => [
    {
      candidateID: "exact",
      outcome: "APPLIED" as GQL.CompletedRecordingOutcome,
    },
  ]
) => {
  const inputs: GQL.CompletedRecordingPreviewInput[] = [];
  const applies: GQL.CompletedRecordingApplyInput[] = [];
  const configurations: GQL.CompletedRecordingImportConfigInput[] = [];
  const view = create(
    <SettingsCompletedRecordingImportView
      roots={[{ path: "/private/server/root" }]}
      enabled={true}
      configuredRoot="/private/server/root"
      configure={async (input) => {
        configurations.push(input);
        return { enabled: input.enabled, root: input.root };
      }}
      preview={async (input) => {
        inputs.push(input);
        return preview;
      }}
      apply={async (input) => {
        applies.push(input);
        return apply(input);
      }}
    />
  );
  return { view, inputs, applies, configurations };
};

test("mounted preview uses configured root internally but never renders its raw path and reports exact review counts", async () => {
  const { view, inputs } = mount();
  assert.doesNotMatch(text(view.root), /private\/server\/root/);
  await act(async () => button(view, "Preview").props.onClick());
  assert.equal(inputs.length, 1);
  assert.equal("root" in inputs[0], false);
  assert.equal(inputs[0].maxFiles, 500);
  assert.match(
    text(
      view.root.findByProps({ "data-testid": "completed-recording-counts" })
    ),
    /1 exact current; 1 review required; 1 skipped/
  );
  assert.match(text(view.root), /HISTORICAL ALIAS REUSED/);
  assert.equal(candidateInputs(view)[1].props.disabled, true);
});

test("selection, Cancel zero mutation, accessible confirmation, and exactly-once Apply produce a visible summary", async () => {
  const { view, applies } = mount();
  await act(async () => button(view, "Preview").props.onClick());
  act(() =>
    candidateInputs(view)[0].props.onChange({
      currentTarget: { checked: true },
    })
  );
  act(() => button(view, "Apply 1 selected").props.onClick());
  const modal = view.root.findByProps({
    "data-testid": "completed-recording-confirm-dialog",
  });
  assert.equal(modal.type, Modal);
  assert.equal(modal.props.role, "dialog");
  assert.equal(modal.props["aria-modal"], true);
  assert.equal(modal.props.backdrop, "static");
  assert.equal(modal.props.keyboard, true);
  assert.equal(modal.props.autoFocus, true);
  act(() => modalButton(view, "Cancel").props.onClick());
  assert.equal(applies.length, 0);
  act(() => button(view, "Apply 1 selected").props.onClick());
  await act(async () => modalButton(view, "Apply 1").props.onClick());
  assert.equal(applies.length, 1);
  assert.deepEqual(applies[0].selectedCandidateIDs, ["exact"]);
  assert.match(
    text(
      view.root.findByProps({ "data-testid": "completed-recording-summary" })
    ),
    /1 applied, 0 no-op, 0 failed/
  );
});

test("a failed second Apply clears the first success summary and shows only the new error", async () => {
  let attempt = 0;
  const { view } = mount(async () => {
    attempt += 1;
    if (attempt === 1)
      return [
        {
          candidateID: "exact",
          outcome: "APPLIED" as GQL.CompletedRecordingOutcome,
        },
      ];
    throw new Error("second apply failed");
  });
  await act(async () => button(view, "Preview").props.onClick());
  act(() =>
    candidateInputs(view)[0].props.onChange({
      currentTarget: { checked: true },
    })
  );
  act(() => button(view, "Apply 1 selected").props.onClick());
  await act(async () => modalButton(view, "Apply 1").props.onClick());
  assert.match(
    text(
      view.root.findByProps({ "data-testid": "completed-recording-summary" })
    ),
    /1 applied/
  );
  act(() =>
    candidateInputs(view)[0].props.onChange({
      currentTarget: { checked: true },
    })
  );
  act(() => button(view, "Apply 1 selected").props.onClick());
  await act(async () => modalButton(view, "Apply 1").props.onClick());
  assert.equal(
    view.root.findAllByProps({ "data-testid": "completed-recording-summary" })
      .length,
    0
  );
  const alerts = view.root
    .findAllByType(Alert)
    .filter((v) => v.props.role === "alert");
  assert.ok(alerts.some((v) => /second apply failed/.test(text(v))));
  assert.doesNotMatch(text(view.root), /Apply finished/);
});

test("stale Apply clears unsafe state, preserves an honest recovery message, and has no silent completion", async () => {
  const { view } = mount(async () => {
    throw new Error("preview is missing or stale");
  });
  await act(async () => button(view, "Preview").props.onClick());
  act(() =>
    candidateInputs(view)[0].props.onChange({
      currentTarget: { checked: true },
    })
  );
  act(() => button(view, "Apply 1 selected").props.onClick());
  await act(async () => modalButton(view, "Apply 1").props.onClick());
  assert.equal(
    view.root.findAllByProps({ "data-testid": "completed-recording-counts" })
      .length,
    0
  );
  const alerts = view.root
    .findAllByType(Alert)
    .filter((v) => v.props.role === "alert");
  assert.match(text(alerts[0]), /Run Preview again/);
  assert.equal(
    view.root.findAllByProps({ "data-testid": "completed-recording-summary" })
      .length,
    0
  );
});

test("empty roots and responsive stylesheet remain accessible and path-safe", () => {
  const css = readFileSync(
    new URL("./settingsCompletedRecordingImport.scss", import.meta.url),
    "utf8"
  );
  assert.match(css, /max-width: 575\.98px/);
  assert.match(css, /min-height: 44px/);
  const view = create(
    <SettingsCompletedRecordingImportView
      roots={[]}
      enabled={false}
      configuredRoot=""
      configure={async () => ({ enabled: false, root: "" })}
      preview={async () => preview}
      apply={async () => []}
    />
  );
  assert.match(text(view.root), /Configure a Library root/);
  assert.equal(
    view.root.findAllByProps({ "data-testid": "completed-recording-preview" })
      .length,
    0
  );
});

test("default-disabled UI performs no preview until one root is explicitly saved and enabled", async () => {
  const inputs: GQL.CompletedRecordingPreviewInput[] = [];
  const configurations: GQL.CompletedRecordingImportConfigInput[] = [];
  const view = create(
    <SettingsCompletedRecordingImportView
      roots={[{ path: "/private/server/root" }]}
      enabled={false}
      configuredRoot=""
      configure={async (input) => {
        configurations.push(input);
        return { enabled: input.enabled, root: input.root };
      }}
      preview={async (input) => {
        inputs.push(input);
        return preview;
      }}
      apply={async () => []}
    />
  );
  assert.equal(
    view.root.findAllByProps({ "data-testid": "completed-recording-preview" })
      .length,
    0
  );
  assert.doesNotMatch(text(view.root), /private\/server\/root/);
  act(() =>
    view.root
      .findByProps({ id: "completed-recording-enabled" })
      .props.onChange({ currentTarget: { checked: true } })
  );
  await act(async () =>
    button(view, "Save import configuration").props.onClick()
  );
  assert.deepEqual(configurations, [
    { enabled: true, root: "/private/server/root" },
  ]);
  await act(async () => button(view, "Preview").props.onClick());
  assert.equal(inputs.length, 1);
  assert.equal("root" in inputs[0], false);
});

test("generated client exposes mutation hooks and never generates a preview query hook", () => {
  const generated = readFileSync(
    new URL("../../core/generated-graphql.ts", import.meta.url),
    "utf8"
  );
  const operation = readFileSync(
    new URL(
      "../../../graphql/completed-recording-import.graphql",
      import.meta.url
    ),
    "utf8"
  );
  const container = readFileSync(
    new URL(
      "./SettingsCompletedRecordingImportPanelContainer.tsx",
      import.meta.url
    ),
    "utf8"
  );
  assert.match(generated, /useCompletedRecordingPreviewMutation/);
  assert.match(generated, /useCompletedRecordingApplyMutation/);
  assert.match(generated, /useCompletedRecordingImportConfigQuery/);
  assert.match(generated, /useCompletedRecordingImportConfigureMutation/);
  assert.doesNotMatch(generated, /useCompletedRecordingPreviewQuery/);
  assert.doesNotMatch(
    operation,
    /CompletedRecordingPreview[\s\S]*\$root|root:\s*\$root/
  );
  assert.match(container, /skip:\s*!canManage/);
  assert.match(container, /if \(!canManage\) return null/);
});
