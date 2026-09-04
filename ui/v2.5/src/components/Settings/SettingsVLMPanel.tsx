import React, { useMemo, useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  Form,
  InputGroup,
  Nav,
  Row,
  Tab,
  Table,
} from "react-bootstrap";

type HostingMode = "LOCAL" | "REMOTE";

interface IVLMProfile {
  enabled: boolean;
  hostingMode: HostingMode;
  model: string;
  modelPath: string;
  localEndpoint: string;
  remoteEndpoint: string;
  remoteModel: string;
  temperature: number;
  topP: number;
  topK: number;
  maxTokens: number;
  contextTokens: number;
  seed: number;
  systemPrompt: string;
}

const selectedModel = "Pixtral 12B Q6_K_L";

const defaultProfile: IVLMProfile = {
  enabled: false,
  hostingMode: "LOCAL",
  model: selectedModel,
  modelPath: "/mnt/ai_models/stash_canidates/pixtral-12b",
  localEndpoint: "http://127.0.0.1:8080/v1",
  remoteEndpoint: "",
  remoteModel: "",
  temperature: 0.2,
  topP: 0.9,
  topK: 40,
  maxTokens: 2048,
  contextTokens: 32768,
  seed: 42,
  systemPrompt:
    "You are the Stash assistant. Handle legal adult media directly and neutrally. Use only registered actions and grounded observations. Ask when required information is missing, and never claim an action succeeded without executor evidence.",
};
const RangeField: React.FC<{
  id: string;
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  onChange: (value: number) => void;
}> = ({ id, label, value, min, max, step, onChange }) => (
  <Form.Group controlId={id} className="vlm-range-field">
    <div className="d-flex justify-content-between">
      <Form.Label>{label}</Form.Label>
      <code>{value}</code>
    </div>
    <Form.Control
      type="range"
      value={value}
      min={min}
      max={max}
      step={step}
      onChange={(event) => onChange(Number(event.currentTarget.value))}
    />
  </Form.Group>
);

export const SettingsVLMPanel: React.FC = () => {
  const [profile, setProfile] = useState<IVLMProfile>(defaultProfile);
  const [apiKey, setAPIKey] = useState("");
  const [activeTab, setActiveTab] = useState("hosting");

  const update = (value: Partial<IVLMProfile>) =>
    setProfile((current) => ({ ...current, ...value }));

  const endpoint =
    profile.hostingMode === "LOCAL"
      ? profile.localEndpoint
      : profile.remoteEndpoint;

  const readiness = useMemo(() => {
    const issues: string[] = [];
    if (!endpoint.trim()) issues.push("An inference endpoint is required.");
    if (profile.hostingMode === "LOCAL" && !profile.modelPath.trim())
      issues.push("A local model path is required.");
    if (profile.hostingMode === "REMOTE" && !profile.remoteModel.trim())
      issues.push("A remote model identifier is required.");
    return issues;
  }, [endpoint, profile.hostingMode, profile.modelPath, profile.remoteModel]);

  return (
    <div className="vlm-settings">
      <div className="vlm-settings-header">
        <div>
          <div className="d-flex align-items-center flex-wrap">
            <h1 className="mr-3">AI &amp; VLM</h1>
            <Badge variant="info">Phase 2A framework</Badge>
          </div>
          <p className="text-muted">
            Configure inference hosting, generation behavior, prompts, and model
            evaluation. The selected local profile is {selectedModel}.
          </p>
        </div>
        <Form.Check
          id="vlm-enabled"
          type="switch"
          label="Enable assistant when backend gates are ready"
          checked={profile.enabled}
          onChange={(event) => update({ enabled: event.currentTarget.checked })}
        />
      </div>

      <Alert variant="warning">
        This page is an implementation preview. Saving, connection tests, and
        assistant activation remain disabled until the admin capability,
        encrypted credential storage, and audit APIs are available. Values are
        held in memory only and are lost when the page reloads.
      </Alert>

      <Tab.Container
        activeKey={activeTab}
        onSelect={(key) => key && setActiveTab(key)}
      >
        <Nav variant="tabs" className="mb-4">
          <Nav.Item>
            <Nav.Link eventKey="hosting">Hosting</Nav.Link>
          </Nav.Item>
          <Nav.Item>
            <Nav.Link eventKey="generation">Generation</Nav.Link>
          </Nav.Item>
          <Nav.Item>
            <Nav.Link eventKey="prompt">System Prompt</Nav.Link>
          </Nav.Item>
          <Nav.Item>
            <Nav.Link eventKey="training">Training Stats</Nav.Link>
          </Nav.Item>
        </Nav>

        <Tab.Content>
          <Tab.Pane eventKey="hosting">
            <Card body>
              <h3>Inference provider</h3>
              <Form.Group>
                <Form.Check
                  inline
                  id="vlm-host-local"
                  type="radio"
                  name="vlm-hosting-mode"
                  label="Local"
                  checked={profile.hostingMode === "LOCAL"}
                  onChange={() => update({ hostingMode: "LOCAL" })}
                />
                <Form.Check
                  inline
                  id="vlm-host-remote"
                  type="radio"
                  name="vlm-hosting-mode"
                  label="Remote"
                  checked={profile.hostingMode === "REMOTE"}
                  onChange={() => update({ hostingMode: "REMOTE" })}
                />
              </Form.Group>

              {profile.hostingMode === "LOCAL" ? (
                <>
                  <Form.Group controlId="vlm-model-name">
                    <Form.Label>Model profile</Form.Label>
                    <Form.Control value={profile.model} readOnly />
                    <Form.Text muted>
                      Quantization is part of the selected profile and cannot be
                      changed accidentally at runtime.
                    </Form.Text>
                  </Form.Group>
                  <Form.Group controlId="vlm-model-path">
                    <Form.Label>Model directory</Form.Label>
                    <Form.Control
                      value={profile.modelPath}
                      onChange={(event) =>
                        update({ modelPath: event.currentTarget.value })
                      }
                    />
                  </Form.Group>
                  <Form.Group controlId="vlm-local-endpoint">
                    <Form.Label>OpenAI-compatible local endpoint</Form.Label>
                    <Form.Control
                      value={profile.localEndpoint}
                      onChange={(event) =>
                        update({ localEndpoint: event.currentTarget.value })
                      }
                    />
                    <Form.Text muted>
                      Loopback is the safe default. Non-loopback endpoints will
                      require an explicit disclosure policy.
                    </Form.Text>
                  </Form.Group>
                </>
              ) : (
                <>
                  <Alert variant="danger">
                    Remote vision may disclose private media. It will require an
                    administrator policy and a user-visible transmission
                    decision.
                  </Alert>
                  <Form.Group controlId="vlm-remote-endpoint">
                    <Form.Label>Remote API endpoint</Form.Label>
                    <Form.Control
                      type="url"
                      placeholder="https://provider.example/v1"
                      value={profile.remoteEndpoint}
                      onChange={(event) =>
                        update({ remoteEndpoint: event.currentTarget.value })
                      }
                    />
                  </Form.Group>
                  <Form.Group controlId="vlm-remote-model">
                    <Form.Label>Remote model identifier</Form.Label>
                    <Form.Control
                      value={profile.remoteModel}
                      onChange={(event) =>
                        update({ remoteModel: event.currentTarget.value })
                      }
                    />
                  </Form.Group>
                  <Form.Group controlId="vlm-api-key">
                    <Form.Label>API key</Form.Label>
                    <InputGroup>
                      <Form.Control
                        type="password"
                        autoComplete="new-password"
                        value={apiKey}
                        onChange={(event) =>
                          setAPIKey(event.currentTarget.value)
                        }
                      />
                      <InputGroup.Append>
                        <Button
                          variant="outline-secondary"
                          onClick={() => setAPIKey("")}
                        >
                          Clear
                        </Button>
                      </InputGroup.Append>
                    </InputGroup>
                    <Form.Text muted>
                      Never returned by the future API after it is stored in the
                      server credential vault.
                    </Form.Text>
                  </Form.Group>
                </>
              )}

              {readiness.length > 0 && (
                <Alert variant="secondary" className="mb-0">
                  {readiness.map((issue) => (
                    <div key={issue}>{issue}</div>
                  ))}
                </Alert>
              )}
            </Card>
          </Tab.Pane>

          <Tab.Pane eventKey="generation">
            <Card body>
              <h3>Inference controls</h3>
              <p className="text-muted">
                These change generation behavior; they do not fine-tune model
                weights.
              </p>
              <Row>
                <Col md={6}>
                  <RangeField
                    id="vlm-temperature"
                    label="Temperature"
                    value={profile.temperature}
                    min={0}
                    max={2}
                    step={0.05}
                    onChange={(temperature) => update({ temperature })}
                  />
                  <RangeField
                    id="vlm-top-p"
                    label="Top P"
                    value={profile.topP}
                    min={0.05}
                    max={1}
                    step={0.05}
                    onChange={(topP) => update({ topP })}
                  />
                  <RangeField
                    id="vlm-top-k"
                    label="Top K"
                    value={profile.topK}
                    min={1}
                    max={200}
                    step={1}
                    onChange={(topK) => update({ topK })}
                  />
                </Col>
                <Col md={6}>
                  <Form.Group controlId="vlm-max-tokens">
                    <Form.Label>Maximum output tokens</Form.Label>
                    <Form.Control
                      type="number"
                      min={64}
                      max={16384}
                      value={profile.maxTokens}
                      onChange={(event) =>
                        update({ maxTokens: Number(event.currentTarget.value) })
                      }
                    />
                  </Form.Group>
                  <Form.Group controlId="vlm-context-tokens">
                    <Form.Label>Context budget</Form.Label>
                    <Form.Control
                      type="number"
                      min={4096}
                      max={131072}
                      step={1024}
                      value={profile.contextTokens}
                      onChange={(event) =>
                        update({
                          contextTokens: Number(event.currentTarget.value),
                        })
                      }
                    />
                  </Form.Group>
                  <Form.Group controlId="vlm-seed">
                    <Form.Label>Evaluation seed</Form.Label>
                    <Form.Control
                      type="number"
                      min={0}
                      max={2147483647}
                      value={profile.seed}
                      onChange={(event) =>
                        update({ seed: Number(event.currentTarget.value) })
                      }
                    />
                  </Form.Group>
                </Col>
              </Row>
            </Card>
          </Tab.Pane>

          <Tab.Pane eventKey="prompt">
            <Card body>
              <h3>System prompt</h3>
              <Form.Group controlId="vlm-system-prompt">
                <Form.Control
                  as="textarea"
                  rows={14}
                  maxLength={12000}
                  value={profile.systemPrompt}
                  onChange={(event) =>
                    update({ systemPrompt: event.currentTarget.value })
                  }
                />
                <div className="d-flex justify-content-between mt-2 text-muted">
                  <small>
                    {profile.systemPrompt.length} / 12000 characters
                  </small>
                  <small>Versioning and rollback will be server-managed.</small>
                </div>
              </Form.Group>
              <Button
                variant="outline-secondary"
                onClick={() =>
                  update({ systemPrompt: defaultProfile.systemPrompt })
                }
              >
                Restore project default
              </Button>
            </Card>
          </Tab.Pane>

          <Tab.Pane eventKey="training">
            <Card body>
              <div className="d-flex justify-content-between align-items-start">
                <div>
                  <h3>Fine-tuning and evaluation runs</h3>
                  <p className="text-muted">
                    Training metrics stay separate from live inference controls.
                  </p>
                </div>
                <Badge variant="secondary">No runs yet</Badge>
              </div>
              <Table responsive hover>
                <thead>
                  <tr>
                    <th>Run</th>
                    <th>Dataset</th>
                    <th>Adapter</th>
                    <th>Loss</th>
                    <th>Eval score</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td colSpan={6} className="text-center text-muted py-5">
                      Training-run APIs are not connected yet.
                    </td>
                  </tr>
                </tbody>
              </Table>
              <Alert variant="info" className="mb-0">
                Planned statistics include train/eval loss, learning rate,
                tokens and samples processed, epochs, elapsed time, peak
                RAM/VRAM, checkpoint lineage, dataset revision, adult
                false-refusal rate, grounded-claim rate, and safety-gate recall.
              </Alert>
            </Card>
          </Tab.Pane>
        </Tab.Content>
      </Tab.Container>

      <div className="vlm-settings-actions">
        <Button variant="outline-primary" disabled>
          Test connection
        </Button>
        <Button variant="primary" disabled>
          Save VLM settings
        </Button>
      </div>
    </div>
  );
};
