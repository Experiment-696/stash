import React, { useRef, useState } from "react";
import { Alert, Button, Card, Form, Spinner } from "react-bootstrap";
import * as GQL from "src/core/generated-graphql";
import { camModelError } from "./camModelUi";
import { CamGirlFinderCandidateSelection } from "./CamGirlFinderCandidateSelection";
import { CamModelConfirmedAction } from "./CamModelConfirmedAction";

export const CamGirlFinderSearchCard: React.FC<{
  modelID: string;
  onIngest: () => Promise<unknown>;
}> = ({ modelID, onIngest }) => {
  const [query, setQuery] = useState(""),
    [items, setItems] = useState<GQL.CamGirlFinderCandidate[]>([]),
    [failure, setFailure] = useState<string>(),
    [notice, setNotice] = useState<string>(),
    [cancelled, setCancelled] = useState(false),
    [selected, setSelected] = useState<string[]>([]),
    [outcomes, setOutcomes] = useState<GQL.CamGirlFinderIngestResult[]>([]);
  const controller = useRef<AbortController>();
  const [search, { loading }] = GQL.useCamGirlFinderSearchMutation();
  const [ingest, { loading: ingesting }] =
    GQL.useCamGirlFinderIngestPendingMutation();
  const runSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!query.trim()) {
      setFailure("Enter a Cam Model name or username.");
      return;
    }
    setFailure(undefined);
    setNotice(undefined);
    setCancelled(false);
    const next = new AbortController();
    controller.current = next;
    try {
      const result = await search({
        variables: { query: query.trim() },
        context: { fetchOptions: { signal: next.signal } },
      });
      setItems(result.data?.camGirlFinderSearch ?? []);
      setSelected([]);
      setOutcomes([]);
    } catch (err) {
      if (next.signal.aborted) {
        setCancelled(true);
        setItems([]);
      } else setFailure(camModelError(err));
    } finally {
      controller.current = undefined;
    }
  };
  const addPending = async () => {
    if (selected.length === 0) {
      setFailure("Select at least one preview candidate.");
      return;
    }
    setFailure(undefined);
    setNotice(undefined);
    const next = new AbortController();
    controller.current = next;
    try {
      const result = await ingest({
        variables: { modelID, query: query.trim(), evidenceKeys: selected },
        context: { fetchOptions: { signal: next.signal } },
      });
      const ingestOutcomes = result.data?.camGirlFinderIngestPending ?? [];
      setOutcomes(ingestOutcomes);
      const count = ingestOutcomes.filter((item) => item.evidenceID).length;
      const rejected = ingestOutcomes.length - count;
      setNotice(
        count +
          " evidence item" +
          (count === 1 ? "" : "s") +
          " added or already observed" +
          (rejected
            ? "; " + rejected + " rejected with an explicit reason"
            : "") +
          "; identity metadata was not changed."
      );
      await onIngest();
    } catch (err) {
      if (next.signal.aborted) setCancelled(true);
      else setFailure(camModelError(err));
      throw err;
    } finally {
      controller.current = undefined;
    }
  };
  return (
    <Card body className="mb-4" data-testid="cam-girl-finder-search">
      <h2>CamGirlFinder discovery</h2>
      <p className="text-muted">
        Dry-run search previews normalized platform and username evidence. It
        does not change the database. Adding results creates pending review
        evidence only—never profiles, accounts, identity merges, history, online
        state, or recordings.
      </p>
      {failure && <Alert variant="danger">{failure}</Alert>}
      {cancelled && (
        <Alert variant="info">CamGirlFinder request cancelled.</Alert>
      )}
      {notice && <Alert variant="success">{notice}</Alert>}
      {outcomes.some((item) => item.reason) && (
        <Alert variant="warning">
          <strong>Rejected selections</strong>
          <ul className="mb-0">
            {outcomes
              .filter((item) => item.reason)
              .map((item) => (
                <li key={item.evidenceKey}>{item.reason}</li>
              ))}
          </ul>
        </Alert>
      )}
      <Form inline onSubmit={(e) => void runSearch(e)}>
        <Form.Control
          aria-label="CamGirlFinder search"
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          placeholder="Name or username"
        />
        <Button className="ml-2" type="submit" disabled={loading || ingesting}>
          {loading ? (
            <>
              <Spinner size="sm" animation="border" /> Searching…
            </>
          ) : (
            "Preview results"
          )}
        </Button>
        {(loading || ingesting) && (
          <Button
            className="ml-2"
            variant="outline-secondary"
            type="button"
            onClick={() => controller.current?.abort()}
          >
            Cancel
          </Button>
        )}
      </Form>
      {!loading && items.length === 0 && (
        <p className="mt-3 mb-0 text-muted">No preview results loaded.</p>
      )}
      {items.length > 0 && (
        <div className="mt-3">
          <p>
            <strong>{items.length}</strong> normalized preview result
            {items.length === 1 ? "" : "s"}
          </p>
          <CamGirlFinderCandidateSelection
            items={items}
            selected={selected}
            setSelected={setSelected}
          />
          <CamModelConfirmedAction
            testID="cam-girl-finder-apply"
            title="Add selected CamGirlFinder evidence?"
            description={
              <>
                <p>
                  Add <strong>{selected.length}</strong> selected candidate
                  {selected.length === 1 ? "" : "s"} as pending review evidence?
                </p>
                <p className="mb-0">
                  This creates pending evidence only. It never creates accounts,
                  merges identity, or changes identity metadata automatically.
                </p>
              </>
            }
            confirmLabel={"Add " + selected.length + " as pending evidence"}
            triggerLabel={
              ingesting
                ? "Adding pending evidence…"
                : "Add " + selected.length + " selected as pending evidence"
            }
            disabled={ingesting || selected.length === 0}
            onConfirm={addPending}
          />
        </div>
      )}
    </Card>
  );
};
