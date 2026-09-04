import React, { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Alert, Badge, Button, Card, Form, Table } from "react-bootstrap";
import * as GQL from "src/core/generated-graphql";
import { TagIDSelect, Tag } from "src/components/Tags/TagSelect";
import { canManageCamModels } from "src/components/CamModels/camModelUi";
import {
  classificationApplyConfirmation,
  classificationExamples,
  classificationCountsLabel,
  classificationDraftError,
} from "./camClassificationUi";

type Target = "BASENAME" | "RELATIVE_PATH";
interface IClassificationRuleDraft {
  id?: string;
  name: string;
  pattern: string;
  target: Target;
  category: string;
  enabled: boolean;
  tagIDs: string[];
}
const emptyDraft: IClassificationRuleDraft = {
  name: "",
  pattern: "",
  target: "BASENAME",
  category: "RECORDED",
  enabled: true,
  tagIDs: [],
};
const errorMessage = (error: unknown) =>
  error instanceof Error
    ? error.message
    : "The operation could not be completed.";

export const SettingsCamClassificationPanel: React.FC = () => {
  const me = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const canManage = canManageCamModels(me.data?.me.capabilities);
  const { data, loading, error, refetch } = GQL.useCamClassificationRulesQuery({
    fetchPolicy: "no-cache",
    skip: !canManage,
  });
  const [createRule] = GQL.useCamClassificationRuleCreateMutation();
  const [updateRule] = GQL.useCamClassificationRuleUpdateMutation();
  const [setEnabled] = GQL.useCamClassificationRuleSetEnabledMutation();
  const [loadPreview, previewState] = GQL.useCamClassificationPreviewLazyQuery({
    fetchPolicy: "no-cache",
  });
  const [applyRules, applyState] = GQL.useCamClassificationApplyMutation();
  const [loadShows] = GQL.useCamShowsLazyQuery({ fetchPolicy: "network-only" });
  const [draft, setDraft] = useState<IClassificationRuleDraft>(() => {
    try {
      return (
        JSON.parse(
          localStorage.getItem("cam-classification-draft") || "null"
        ) || emptyDraft
      );
    } catch {
      return emptyDraft;
    }
  });
  const [completion, setCompletion] = useState<{
    matched: number;
    applied: number;
    skipped: number;
    conflicted: number;
    total: number;
  }>();
  const [failure, setFailure] = useState<string>();
  const [notice, setNotice] = useState<string>();
  const validation = useMemo(() => classificationDraftError(draft), [draft]);
  const preview = previewState.data?.camClassificationPreview;
  const items = (preview?.items ?? []).filter(
    (item) => item.matched || item.conflict
  );
  const tagNames = useMemo(
    () =>
      new Map(
        (data?.camClassificationRules ?? []).flatMap((rule) =>
          rule.tags.map((tag) => [tag.id, tag.name] as const)
        )
      ),
    [data]
  );
  useEffect(() => {
    localStorage.setItem("cam-classification-draft", JSON.stringify(draft));
  }, [draft]);

  async function run(action: () => Promise<unknown>) {
    setFailure(undefined);
    try {
      await action();
    } catch (caught) {
      setFailure(errorMessage(caught));
    }
  }
  async function save(event: React.FormEvent) {
    event.preventDefault();
    if (validation) {
      setFailure(validation);
      return;
    }
    await run(async () => {
      const common = {
        name: draft.name.trim(),
        pattern: draft.pattern.trim(),
        target: draft.target,
        category: draft.category.trim(),
        enabled: draft.enabled,
        tagIDs: draft.tagIDs,
      };
      if (draft.id)
        await updateRule({ variables: { input: { id: draft.id, ...common } } });
      else await createRule({ variables: { input: common } });
      await refetch();
      setNotice(draft.id ? "Rule updated." : "Rule created.");
      setDraft(emptyDraft);
      localStorage.removeItem("cam-classification-draft");
    });
  }

  if (!canManage) return null;
  if (loading) return <p>Loading Cam Show classification rules…</p>;
  if (error)
    return <Alert variant="danger">Unable to load classification rules.</Alert>;
  return (
    <div data-testid="cam-classification-settings">
      <h2>Cam Show classification</h2>
      <p className="text-muted">
        Rules classify matching library scenes as Cam Shows. They inspect names
        and paths only; they do not inspect or change media files.
      </p>
      <ul className="text-muted">
        <li>
          <strong>Filename (basename)</strong> checks only the filename. Example
          timestamp expression: <code>{classificationExamples.basename}</code>
        </li>
        <li>
          <strong>Normalized relative path</strong> checks the path below a
          configured library root, using forward slashes. Example:{" "}
          <code>{classificationExamples.relativePath}</code>
        </li>
        <li>
          <strong>Category</strong> records the kind of Cam Show, such as{" "}
          <code>RECORDED</code> or <code>LIVE</code>. Selected tags are added to
          each matched scene&apos;s database metadata.
        </li>
        <li>
          Only enabled rules participate. Disabled rules stay saved but are
          ignored by Preview and Apply.
        </li>
        <li>
          Preview is a non-mutating dry run. It shows counts and items; category
          conflicts are reported and skipped.
        </li>
        <li>
          Apply changes database metadata only. It never renames, rewrites, or
          modifies media, and applying the same rules again is idempotent.
        </li>
      </ul>
      {failure && (
        <Alert variant="danger" role="alert">
          {failure}
        </Alert>
      )}
      {notice && <Alert variant="success">{notice}</Alert>}
      <Card body className="mb-4">
        <h3>{draft.id ? "Edit rule" : "Create rule"}</h3>
        <Form onSubmit={(event) => void save(event)}>
          <Form.Group controlId="classification-rule-name">
            <Form.Label>Name</Form.Label>
            <Form.Control
              value={draft.name}
              onChange={(e) =>
                setDraft({ ...draft, name: e.currentTarget.value })
              }
            />
          </Form.Group>
          <Form.Group controlId="classification-rule-pattern">
            <Form.Label>Regular expression</Form.Label>
            <Form.Control
              value={draft.pattern}
              onChange={(e) =>
                setDraft({ ...draft, pattern: e.currentTarget.value })
              }
            />
            <Form.Text muted>
              Invalid expressions are rejected before saving and by the server.
            </Form.Text>
          </Form.Group>
          <Form.Group controlId="classification-rule-target">
            <Form.Label>Match target</Form.Label>
            <Form.Control
              as="select"
              value={draft.target}
              onChange={(e) =>
                setDraft({ ...draft, target: e.currentTarget.value as Target })
              }
            >
              <option value="BASENAME">Filename (basename)</option>
              <option value="RELATIVE_PATH">Normalized relative path</option>
            </Form.Control>
          </Form.Group>
          <Form.Group controlId="classification-rule-category">
            <Form.Label>Cam Show category</Form.Label>
            <Form.Control
              value={draft.category}
              onChange={(e) =>
                setDraft({ ...draft, category: e.currentTarget.value })
              }
            />
          </Form.Group>
          <Form.Group>
            <Form.Label>Tags</Form.Label>
            <TagIDSelect
              isMulti
              isClearable
              creatable
              ids={draft.tagIDs}
              onSelect={(tags: Tag[]) =>
                setDraft({ ...draft, tagIDs: tags.map((tag) => tag.id) })
              }
            />
          </Form.Group>
          <Form.Check
            id="classification-rule-enabled"
            type="switch"
            label="Enabled"
            checked={draft.enabled}
            onChange={(e) =>
              setDraft({ ...draft, enabled: e.currentTarget.checked })
            }
          />
          {validation && draft.pattern && (
            <Form.Text className="text-danger d-block mt-2">
              {validation}
            </Form.Text>
          )}
          <Button className="mt-3" type="submit" disabled={Boolean(validation)}>
            {draft.id ? "Save rule" : "Create rule"}
          </Button>
          {draft.id && (
            <Button
              className="ml-2 mt-3"
              variant="secondary"
              onClick={() => setDraft(emptyDraft)}
            >
              Cancel
            </Button>
          )}
        </Form>
      </Card>
      <h3>Rules</h3>
      <Table responsive striped>
        <thead>
          <tr>
            <th>Name / expression</th>
            <th>Target</th>
            <th>Category</th>
            <th>Tags</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {(data?.camClassificationRules ?? []).map((rule) => (
            <tr key={rule.id}>
              <td>
                <strong>{rule.name}</strong>
                <br />
                <code>{rule.pattern}</code>
              </td>
              <td>{rule.target}</td>
              <td>{rule.category}</td>
              <td>{rule.tags.map((tag) => tag.name).join(", ") || "None"}</td>
              <td>
                <Badge variant={rule.enabled ? "success" : "secondary"}>
                  {rule.enabled ? "Enabled" : "Disabled"}
                </Badge>
              </td>
              <td>
                <Button
                  size="sm"
                  className="mr-2"
                  onClick={() =>
                    setDraft({
                      id: rule.id,
                      name: rule.name,
                      pattern: rule.pattern,
                      target: rule.target as Target,
                      category: rule.category,
                      enabled: rule.enabled,
                      tagIDs: rule.tagIDs,
                    })
                  }
                >
                  Edit
                </Button>
                <Button
                  size="sm"
                  variant="outline-secondary"
                  onClick={() =>
                    void run(async () => {
                      await setEnabled({
                        variables: { id: rule.id, enabled: !rule.enabled },
                      });
                      await refetch();
                    })
                  }
                >
                  {rule.enabled ? "Disable" : "Enable"}
                </Button>
              </td>
            </tr>
          ))}
          {!data?.camClassificationRules.length && (
            <tr>
              <td colSpan={6}>No rules configured.</td>
            </tr>
          )}
        </tbody>
      </Table>
      <Card body className="mt-4">
        <h3>Preview and apply</h3>
        <p>Preview is a dry run. Category conflicts are shown and skipped.</p>
        <Button
          className="mr-2"
          disabled={previewState.loading}
          onClick={() => void run(() => loadPreview())}
        >
          Preview enabled rules
        </Button>
        <Button
          variant="danger"
          disabled={!preview || applyState.loading}
          onClick={() => {
            if (!window.confirm(classificationApplyConfirmation)) return;
            void run(async () => {
              const result = await applyRules();
              const shows = await loadShows();
              if (result.data)
                setCompletion({
                  ...result.data.camClassificationApply,
                  total: shows.data?.camShows.length ?? 0,
                });
              await refetch();
            });
          }}
        >
          Apply metadata changes…
        </Button>
        {preview && (
          <div className="mt-3">
            <Alert variant={preview.conflicted ? "warning" : "info"}>
              {classificationCountsLabel(preview)}
            </Alert>
            <Table responsive size="sm">
              <thead>
                <tr>
                  <th>Scene</th>
                  <th>Category</th>
                  <th>Tags</th>
                  <th>Result</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item) => (
                  <tr key={item.sceneID}>
                    <td>{item.sceneID}</td>
                    <td>{item.category || "—"}</td>
                    <td>
                      {item.tagIDs
                        .map((id) => tagNames.get(id) || id)
                        .join(", ") || "None"}
                    </td>
                    <td>{item.conflict || "Will apply"}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        )}
        {completion && (
          <Alert
            className="mt-3"
            variant="success"
            role="status"
            data-testid="classification-apply-complete"
          >
            <strong>Apply complete.</strong>{" "}
            {classificationCountsLabel(completion)} | {completion.total} total
            Shows. <Link to="/shows">View Shows</Link>
          </Alert>
        )}
      </Card>
    </div>
  );
};
