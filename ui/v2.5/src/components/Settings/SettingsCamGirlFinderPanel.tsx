import React, { useEffect, useState } from "react";
import { Alert, Button, Card, Form, Spinner } from "react-bootstrap";
import * as GQL from "src/core/generated-graphql";
import { canManageCamModels, camModelError } from "../CamModels/camModelUi";

export const SettingsCamGirlFinderPanel: React.FC = () => {
  const { data: meData } = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const canManage = canManageCamModels(meData?.me.capabilities);
  const { data, loading, error } = GQL.useCamGirlFinderConfigQuery({
    skip: !canManage,
    fetchPolicy: "no-cache",
  });
  const [save, saving] = GQL.useCamGirlFinderConfigureMutation();
  const [enabled, setEnabled] = useState(false),
    [interval, setIntervalValue] = useState(1000),
    [timeout, setTimeoutValue] = useState(15),
    [limit, setLimit] = useState(25);
  const [failure, setFailure] = useState<string>(),
    [notice, setNotice] = useState<string>();
  useEffect(() => {
    const v = data?.camGirlFinderConfig;
    if (v) {
      setEnabled(v.enabled);
      setIntervalValue(v.requestIntervalMS);
      setTimeoutValue(v.timeoutSeconds);
      setLimit(v.resultLimit);
    }
  }, [data]);
  if (!canManage) return null;
  if (loading) return <Spinner animation="border" />;
  return (
    <Card body className="mb-4" data-testid="cam-girl-finder-settings">
      <h2>CamGirlFinder</h2>
      <p className="text-muted">
        Optional Cam Model discovery from the fixed official API. Search results
        are previews only until you explicitly add them as pending evidence.
        Stash never creates accounts, merges identity, infers history or online
        state, or records media.
      </p>
      {(error || failure) && (
        <Alert variant="danger">
          {failure ?? "Unable to load CamGirlFinder settings."}
        </Alert>
      )}
      {notice && <Alert variant="success">{notice}</Alert>}
      <Form
        onSubmit={async (e) => {
          e.preventDefault();
          setFailure(undefined);
          setNotice(undefined);
          try {
            await save({
              variables: {
                input: {
                  enabled,
                  requestIntervalMS: interval,
                  timeoutSeconds: timeout,
                  resultLimit: limit,
                },
              },
            });
            setNotice("CamGirlFinder settings saved.");
          } catch (err) {
            setFailure(camModelError(err));
          }
        }}
      >
        <Form.Check
          id="cam-girl-finder-enabled"
          label="Enable CamGirlFinder discovery"
          checked={enabled}
          onChange={(e) => setEnabled(e.currentTarget.checked)}
        />
        <Form.Group controlId="cam-girl-finder-interval">
          <Form.Label>Minimum request interval (milliseconds)</Form.Label>
          <Form.Control
            type="number"
            min={100}
            max={60000}
            value={interval}
            onChange={(e) => setIntervalValue(Number(e.currentTarget.value))}
          />
          <Form.Text>
            100–60000 ms. A conservative 1000 ms default limits requests to one
            per second.
          </Form.Text>
        </Form.Group>
        <Form.Group controlId="cam-girl-finder-timeout">
          <Form.Label>Request timeout (seconds)</Form.Label>
          <Form.Control
            type="number"
            min={1}
            max={120}
            value={timeout}
            onChange={(e) => setTimeoutValue(Number(e.currentTarget.value))}
          />
          <Form.Text>1–120 seconds.</Form.Text>
        </Form.Group>
        <Form.Group controlId="cam-girl-finder-limit">
          <Form.Label>Maximum preview results</Form.Label>
          <Form.Control
            type="number"
            min={1}
            max={100}
            value={limit}
            onChange={(e) => setLimit(Number(e.currentTarget.value))}
          />
          <Form.Text>1–100 results per bounded request.</Form.Text>
        </Form.Group>
        <Button type="submit" disabled={saving.loading}>
          {saving.loading ? "Saving…" : "Save CamGirlFinder settings"}
        </Button>
      </Form>
    </Card>
  );
};
