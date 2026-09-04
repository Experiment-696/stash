import React, { useMemo, useRef, useState } from "react";
import { Alert, Button, Form, Modal, Spinner, Table } from "react-bootstrap";
import type * as GQL from "../../core/generated-graphql";

const OUTCOME = {
  exact: "EXACT_READY",
  review: "REVIEW_REQUIRED",
  applied: "APPLIED",
  noop: "ALREADY_APPLIED",
} as const;

const PATTERN =
  "^(?<site>[A-Za-z0-9_]+)-(?<model>[A-Za-z0-9_]+)-(?<timestamp>[0-9]{8}-[0-9]{6}).*[.](mp4|mkv|webm)$";
export interface ICompletedRecordingRoot {
  path: string;
}
export interface ICompletedRecordingImportViewProps {
  roots: ICompletedRecordingRoot[];
  enabled: boolean;
  configuredRoot: string;
  configure: (
    input: GQL.CompletedRecordingImportConfigInput
  ) => Promise<GQL.CompletedRecordingImportConfig>;
  preview: (
    input: GQL.CompletedRecordingPreviewInput
  ) => Promise<GQL.CompletedRecordingPreview>;
  apply: (
    input: GQL.CompletedRecordingApplyInput
  ) => Promise<GQL.CompletedRecordingApplyResult[]>;
}
const messageOf = (error: unknown) =>
  error instanceof Error ? error.message : "The operation failed. Try again.";
const stale = (message: string) =>
  /preview.*(stale|expired|missing)|stale.*preview/i.test(message);

export const SettingsCompletedRecordingImportView: React.FC<
  ICompletedRecordingImportViewProps
> = ({ roots, enabled, configuredRoot, configure, preview, apply }) => {
  const configuredIndex = Math.max(
    0,
    roots.findIndex((value) => value.path === configuredRoot)
  );
  const [root, setRoot] = useState(String(configuredIndex));
  const [draftEnabled, setDraftEnabled] = useState(enabled);
  const [activeEnabled, setActiveEnabled] = useState(enabled);
  const [activeRoot, setActiveRoot] = useState(configuredRoot);
  const [saving, setSaving] = useState(false);
  const [result, setResult] = useState<GQL.CompletedRecordingPreview>();
  const [selected, setSelected] = useState<string[]>([]);
  const [previewing, setPreviewing] = useState(false);
  const [applying, setApplying] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string>();
  const [summary, setSummary] = useState<string>();
  const lock = useRef(false);
  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const counts = useMemo(
    () => ({
      exact:
        result?.items.filter((v) => v.outcome === OUTCOME.exact).length ?? 0,
      review:
        result?.items.filter((v) => v.outcome === OUTCOME.review).length ?? 0,
      other:
        result?.items.filter(
          (v) => v.outcome !== OUTCOME.exact && v.outcome !== OUTCOME.review
        ).length ?? 0,
    }),
    [result]
  );
  const selectedRoot = roots[Number(root)];
  const configuredReady =
    activeEnabled && !!selectedRoot && activeRoot === selectedRoot.path;

  const saveConfiguration = async () => {
    setSaving(true);
    setError(undefined);
    setSummary(undefined);
    try {
      const saved = await configure({
        enabled: draftEnabled,
        root: selectedRoot?.path ?? "",
      });
      setActiveEnabled(saved.enabled);
      setActiveRoot(saved.root);
      setResult(undefined);
      setSelected([]);
      setSummary(
        saved.enabled
          ? "Completed recording import enabled for the selected Library root."
          : "Completed recording import disabled."
      );
    } catch (cause) {
      setError(messageOf(cause));
    } finally {
      setSaving(false);
    }
  };

  const runPreview = async () => {
    if (!configuredReady) return;
    setPreviewing(true);
    setError(undefined);
    setSummary(undefined);
    try {
      setResult(
        await preview({
          maxFiles: 500,
          maxDepth: 8,
          timeoutMS: 10000,
          extensions: [".mp4", ".mkv", ".webm"],
          filenamePattern: PATTERN,
          timestampLayout: "20060102-150405",
          timezone: "UTC",
          precision: "SECOND" as GQL.CompletedRecordingPrecision,
          parserVersion: "completed-import-ui-v1",
        })
      );
      setSelected([]);
    } catch (cause) {
      setError(messageOf(cause));
    } finally {
      setPreviewing(false);
    }
  };
  const runApply = async () => {
    if (!result || selected.length === 0 || lock.current) return;
    lock.current = true;
    setApplying(true);
    setError(undefined);
    setSummary(undefined);
    try {
      const items = await apply({
        previewID: result.previewID,
        selectedCandidateIDs: selected,
      });
      const applied = items.filter((v) => v.outcome === OUTCOME.applied).length;
      const noops = items.filter((v) => v.outcome === OUTCOME.noop).length;
      setSummary(
        `Apply finished: ${applied} applied, ${noops} no-op, ${
          items.length - applied - noops
        } failed or skipped.`
      );
      setSelected([]);
      setConfirming(false);
    } catch (cause) {
      const next = messageOf(cause);
      setError(next);
      if (stale(next)) {
        setResult(undefined);
        setSelected([]);
        setConfirming(false);
      }
    } finally {
      lock.current = false;
      setApplying(false);
    }
  };

  return (
    <section
      className="completed-recording-import"
      aria-labelledby="completed-recording-import-heading"
    >
      <h3 id="completed-recording-import-heading">
        Completed recording import
      </h3>
      <p>
        Preview existing library files from one configured root, then link only
        exact current Site and model matches to Cam Shows. This reads file
        metadata and changes database metadata only; it never records, scans,
        renames, moves, deletes, or writes media.
      </p>
      <Form>
        <Form.Check
          id="completed-recording-enabled"
          label="Enable local completed recording import"
          checked={draftEnabled}
          disabled={saving || roots.length === 0}
          onChange={(event) => setDraftEnabled(event.currentTarget.checked)}
        />
        <Form.Group controlId="completed-recording-root">
          <Form.Label>Approved Library root</Form.Label>
          <Form.Control
            as="select"
            value={root}
            disabled={saving || roots.length === 0}
            onChange={(event) => {
              setRoot(event.currentTarget.value);
              setResult(undefined);
              setSelected([]);
            }}
          >
            {roots.map((_item, index) => (
              <option key={index} value={index}>
                Library root {index + 1}
              </option>
            ))}
          </Form.Control>
          <Form.Text>
            Server paths are hidden here. Exactly one existing Library root is
            approved; no root is scanned implicitly.
          </Form.Text>
        </Form.Group>
        <Button
          type="button"
          variant="secondary"
          disabled={saving || roots.length === 0}
          onClick={() => void saveConfiguration()}
          data-testid="completed-recording-save-config"
        >
          {saving ? "Saving…" : "Save import configuration"}
        </Button>
      </Form>
      {roots.length === 0 ? (
        <Alert variant="info" role="status">
          Configure a Library root above before using completed recording
          import.
        </Alert>
      ) : !configuredReady ? (
        <Alert variant="info" role="status">
          Import is disabled or the selected root has not been explicitly saved.
          No filesystem discovery will run.
        </Alert>
      ) : (
        <Form className="mt-3">
          <p>
            Preview is bounded to 500 files, 8 directory levels, and 10 seconds.
          </p>
          <Button
            type="button"
            disabled={previewing || applying}
            onClick={() => void runPreview()}
            data-testid="completed-recording-preview"
          >
            {previewing ? (
              <>
                <Spinner animation="border" size="sm" /> Previewing…
              </>
            ) : (
              "Preview"
            )}
          </Button>
        </Form>
      )}
      {error && (
        <Alert variant="danger" role="alert" aria-live="assertive">
          {error} {stale(error) && "Run Preview again to continue."}
        </Alert>
      )}
      {summary && (
        <Alert
          variant="success"
          role="status"
          aria-live="polite"
          data-testid="completed-recording-summary"
        >
          {summary}
        </Alert>
      )}
      {result && (
        <div className="completed-recording-results" aria-live="polite">
          <h4>Preview results</h4>
          <p data-testid="completed-recording-counts">
            {result.scannedCount} scanned; {counts.exact} exact current;{" "}
            {counts.review} review required; {counts.other} skipped.
          </p>
          {result.truncated && (
            <Alert variant="warning" role="status">
              Preview stopped at the{" "}
              {result.boundReason?.toLowerCase().replaceAll("_", " ")} bound.
              Completed items are shown.
            </Alert>
          )}
          {result.items.length === 0 ? (
            <Alert variant="info" role="status">
              No completed recording candidates were found.
            </Alert>
          ) : (
            <div className="completed-recording-table-wrap">
              <Table responsive striped size="sm">
                <thead>
                  <tr>
                    <th scope="col">Select</th>
                    <th scope="col">File</th>
                    <th scope="col">Site / model</th>
                    <th scope="col">Outcome</th>
                    <th scope="col">Reason</th>
                  </tr>
                </thead>
                <tbody>
                  {result.items.map((item) => {
                    const eligible = item.outcome === OUTCOME.exact;
                    return (
                      <tr key={item.candidateID}>
                        <td>
                          <Form.Check
                            aria-label={`Select ${item.relativePath}`}
                            disabled={!eligible || applying}
                            checked={selectedSet.has(item.candidateID)}
                            onChange={(event) => {
                              const { checked } = event.currentTarget;
                              setSelected((current) =>
                                checked
                                  ? [...current, item.candidateID]
                                  : current.filter(
                                      (id) => id !== item.candidateID
                                    )
                              );
                            }}
                          />
                        </td>
                        <td>{item.relativePath}</td>
                        <td>
                          {item.platform} / {item.username}
                        </td>
                        <td>{item.outcome.replaceAll("_", " ")}</td>
                        <td>
                          {item.reviewCode?.replaceAll("_", " ") ??
                            item.reviewReason ??
                            "—"}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </Table>
            </div>
          )}
          <Button
            type="button"
            variant="warning"
            disabled={selected.length === 0 || applying}
            onClick={() => setConfirming(true)}
            data-testid="completed-recording-apply-trigger"
          >
            Apply {selected.length} selected
          </Button>
        </div>
      )}
      <Modal
        animation={false}
        show={confirming}
        onHide={() => !applying && setConfirming(false)}
        backdrop="static"
        keyboard={!applying}
        aria-label="Apply completed recording metadata?"
        aria-modal
        role="dialog"
        data-testid="completed-recording-confirm-dialog"
        autoFocus
        enforceFocus
        restoreFocus
      >
        <Modal.Header>
          <Modal.Title>Apply completed recording metadata?</Modal.Title>
        </Modal.Header>
        <Modal.Body>
          Apply {selected.length} exact-current candidate
          {selected.length === 1 ? "" : "s"}. This is metadata-only and
          idempotent. It will not modify media or create or merge model
          identity. Cancel makes no changes.
          {error && (
            <Alert
              className="mt-3 mb-0"
              variant="danger"
              role="alert"
              aria-live="assertive"
            >
              {error}
            </Alert>
          )}
        </Modal.Body>
        <Modal.Footer>
          <Button
            type="button"
            variant="secondary"
            disabled={applying}
            onClick={() => setConfirming(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="danger"
            disabled={applying}
            onClick={() => void runApply()}
            data-testid="completed-recording-confirm-apply"
          >
            {applying ? (
              <>
                <Spinner animation="border" size="sm" /> Applying…
              </>
            ) : (
              `Apply ${selected.length}`
            )}
          </Button>
        </Modal.Footer>
      </Modal>
    </section>
  );
};
