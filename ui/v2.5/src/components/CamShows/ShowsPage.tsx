import React, { useState } from "react";
import { Alert, Badge, Button, Card, Form, Nav, Tab } from "react-bootstrap";
import { Link, useHistory, useLocation, useParams } from "react-router-dom";
import { ExternalLink } from "src/components/Shared/ExternalLink";
import ScenePlayer from "src/components/ScenePlayer/ScenePlayer";
import { SceneCard } from "src/components/Scenes/SceneCard";
import {
  useCardWidth,
  useContainerDimensions,
} from "src/components/Shared/GridCard/GridCard";
import { RatingSystem } from "src/components/Shared/Rating/RatingSystem";
import * as GQL from "src/core/generated-graphql";
import { canManageCamModels } from "src/components/CamModels/camModelUi";
import {
  CamShowSortControl,
  camShowSortFromSearch,
  camShowSortSearch,
  uniqueCamShows,
} from "./camShowSortUi";

type Show = NonNullable<GQL.CamShowsQuery["camShows"]>[number];
type ModelAssignment = { modelID: string; role: string };

// Owner-facing labels for the frozen six-value cam show_type taxonomy.
const SHOW_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: "LIVE_PUBLIC", label: "Public" },
  {
    value: "LIVE_GROUP_TICKET_MULTIUSER",
    label: "Group / ticketed (multi-user)",
  },
  { value: "LIVE_PRIVATE", label: "Private" },
  { value: "LIVE_EXCLUSIVE_PRIVATE", label: "Exclusive private" },
  { value: "CUSTOM_VIDEO", label: "Offsite / custom video" },
  { value: "PRIVATE_CALL", label: "Private call" },
];
const showTypeLabel = (v: string) =>
  SHOW_TYPE_OPTIONS.find((o) => o.value === v)?.label ?? v.replaceAll("_", " ");

const ShowCard: React.FC<{
  show: Show;
  canManage: boolean;
  saved: () => Promise<unknown>;
  initiallyEditing?: boolean;
}> = ({ show, canManage, saved, initiallyEditing = false }) => {
  const [editing, setEditing] = useState(initiallyEditing),
    [title, setTitle] = useState(show.title),
    [showType, setShowType] = useState(show.showType),
    [precision, setPrecision] = useState(show.capturedPrecision ?? ""),
    [capturedAt, setCapturedAt] = useState(
      show.capturedAt ? show.capturedAt.slice(0, 16) : ""
    ),
    [rate, setRate] = useState(show.rate == null ? "" : String(show.rate)),
    [extras, setExtras] = useState(show.extras ?? ""),
    [request, setRequest] = useState(show.request ?? ""),
    [failure, setFailure] = useState<string>();
  const [update, state] = GQL.useCamShowUpdateMutation();
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setFailure(undefined);
    try {
      await update({
        variables: {
          input: {
            id: show.id,
            title: title.trim(),
            showType,
            showDate: show.showDate,
            capturedAt: capturedAt
              ? new Date(capturedAt + "Z").toISOString()
              : null,
            capturedTimezone: show.capturedTimezone,
            capturedPrecision: precision || null,
            durationOverrideSeconds: show.durationOverridden
              ? show.durationSeconds
              : null,
            durationOverrideReason: show.durationOverridden
              ? show.durationOverrideReason
              : null,
            rate: rate ? Number(rate) : null,
            extras: extras.trim() || null,
            request: request.trim() || null,
          },
        },
      });
      await saved();
      setEditing(false);
    } catch (error) {
      setFailure(
        error instanceof Error ? error.message : "Unable to update Show."
      );
    }
  }
  return (
    <Card body className="h-100">
      {failure && <Alert variant="danger">{failure}</Alert>}
      {editing ? (
        <Form onSubmit={(e) => void submit(e)} data-testid="cam-show-editor">
          <Form.Group>
            <Form.Label>Show title</Form.Label>
            <Form.Control
              value={title}
              onChange={(e) => setTitle(e.currentTarget.value)}
              required
            />
          </Form.Group>
          <Form.Group>
            <Form.Label>Show type</Form.Label>
            <Form.Control
              as="select"
              value={showType}
              onChange={(e) => setShowType(e.currentTarget.value)}
            >
              {SHOW_TYPE_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </Form.Control>
          </Form.Group>
          <Form.Group>
            <Form.Label>Recording time precision</Form.Label>
            <Form.Control
              as="select"
              value={precision}
              onChange={(e) => setPrecision(e.currentTarget.value)}
            >
              <option value="">Unknown</option>
              {["DATE", "HOUR", "MINUTE", "SECOND"].map((v) => (
                <option key={v}>{v}</option>
              ))}
            </Form.Control>
            <Form.Text muted>
              Use HOUR when the recording hour is known but minutes are not.
            </Form.Text>
          </Form.Group>
          <Form.Group>
            <Form.Label>Date / time</Form.Label>
            <Form.Control
              type="datetime-local"
              value={capturedAt}
              onChange={(e) => setCapturedAt(e.currentTarget.value)}
            />
            <Form.Text muted>Stored and shown in UTC.</Form.Text>
          </Form.Group>
          <Form.Group>
            <Form.Label>Rate</Form.Label>
            <Form.Control
              type="number"
              min="0"
              step="any"
              value={rate}
              onChange={(e) => setRate(e.currentTarget.value)}
            />
          </Form.Group>
          <Form.Group>
            <Form.Label>Request</Form.Label>
            <Form.Control
              as="textarea"
              value={request}
              onChange={(e) => setRequest(e.currentTarget.value)}
            />
          </Form.Group>
          <Form.Group>
            <Form.Label>Extras</Form.Label>
            <Form.Control
              as="textarea"
              value={extras}
              onChange={(e) => setExtras(e.currentTarget.value)}
            />
          </Form.Group>
          <Button type="submit" disabled={state.loading}>
            Save Show metadata
          </Button>
          <Button
            className="ml-2"
            variant="secondary"
            onClick={() => setEditing(false)}
          >
            Cancel
          </Button>
          <Form.Text muted>
            Only database metadata changes. The linked Scene and media file are
            never renamed or modified.
          </Form.Text>
        </Form>
      ) : (
        <>
          <Card.Title>
            <Link to={"/shows/" + show.id}>{show.title}</Link>
          </Card.Title>
          <div>
            <Badge variant="secondary">{showTypeLabel(show.showType)}</Badge>
          </div>
          <div className="mt-2">
            {show.showDate
              ? new Date(show.showDate).toLocaleDateString()
              : "Date unknown"}
            {show.capturedAt
              ? " � " + new Date(show.capturedAt).toLocaleString()
              : ""}
            {show.capturedTimezone ? " " + show.capturedTimezone : ""}
            {show.capturedPrecision
              ? " · " + show.capturedPrecision.toLowerCase() + " precision"
              : ""}
          </div>
          {show.durationSeconds != null && (
            <div>
              {Math.round(show.durationSeconds)} seconds
              {show.durationOverridden
                ? " (justified override: " + show.durationOverrideReason + ")"
                : " (from linked Scene file)"}
            </div>
          )}
          {show.rate != null && <div>Rate: {show.rate}</div>}
          {show.request && <p className="mt-2 mb-1">Request: {show.request}</p>}
          {show.extras && <p className="mt-2 mb-1">Extras: {show.extras}</p>}
          <div className="mt-2">
            {show.sites.length ? (
              show.sites.map((v) => (
                <Badge className="mr-1" variant="light" key={v.id}>
                  {v.icon && (
                    <img
                      alt=""
                      src={v.icon}
                      width={16}
                      height={16}
                      className="mr-1"
                    />
                  )}
                  {v.name}
                </Badge>
              ))
            ) : (
              <span className="text-muted">No Sites</span>
            )}
          </div>
          <div className="mt-2">
            {show.models.map((v) => (
              <span className="mr-2" key={v.modelID}>
                <Link to={"/cam-models/" + v.modelID}>{v.displayName}</Link> (
                {v.role})
              </span>
            ))}
          </div>
          <div className="mt-2">
            {show.links.map((v) => (
              <ExternalLink key={v.id} href={v.url}>
                {v.label || v.linkType}
              </ExternalLink>
            ))}
          </div>
          <div className="mt-2">
            {show.tags.length ? (
              show.tags.map((v) => (
                <Badge className="mr-1" variant="info" key={v.id}>
                  {v.name}
                </Badge>
              ))
            ) : (
              <span className="text-muted">No tags</span>
            )}
          </div>
        </>
      )}
    </Card>
  );
};

const ShowDetailsPage: React.FC<{
  show: Show;
  canManage: boolean;
  saved: () => Promise<unknown>;
}> = ({ show, canManage, saved }) => {
  const { data, loading, error } = GQL.useFindSceneQuery({
    variables: { id: show.sceneID },
  });
  const { data: options } = GQL.useCamShowAssociationOptionsQuery({
    skip: !canManage,
  });
  const [siteIDs, setSiteIDs] = useState(show.sites.map((site) => site.id));
  const [models, setModels] = useState<ModelAssignment[]>(
    show.models.map((model) => ({
      modelID: model.modelID,
      role: model.role,
    }))
  );
  const [associationFailure, setAssociationFailure] = useState<string>();
  const [setAssociations, associationState] =
    GQL.useCamShowSetAssociationsMutation();
  const [setShowRating] = GQL.useCamShowSetRatingMutation();

  const toggleModel = (modelID: string, selected: boolean) => {
    setModels((current) =>
      selected
        ? [...current, { modelID, role: "PARTICIPANT" }]
        : current.filter((model) => model.modelID !== modelID)
    );
  };
  const setModelRole = (modelID: string, role: string) => {
    setModels((current) =>
      current.map((model) =>
        model.modelID === modelID ? { ...model, role } : model
      )
    );
  };
  const saveAssociations = async (event: React.FormEvent) => {
    event.preventDefault();
    setAssociationFailure(undefined);
    try {
      await setAssociations({
        variables: { input: { id: show.id, siteIDs, models } },
      });
      await saved();
    } catch (saveError) {
      setAssociationFailure(
        saveError instanceof Error
          ? saveError.message
          : "Unable to save Show associations."
      );
    }
  };

  return (
    <div className="row" data-testid="cam-show-details">
      <div className="scene-tabs order-xl-first order-last">
        <div className="scene-header-container">
          <h3 className="scene-header no-studio">{show.title}</h3>
        </div>
        <div className="scene-subheader">
          {show.showDate
            ? new Date(show.showDate).toLocaleDateString()
            : "Date unknown"}
        </div>
        <div className="scene-toolbar">
          <span className="scene-toolbar-group">
            <RatingSystem
              value={show.rating100}
              onSetRating={(rating100) =>
                void setShowRating({
                  variables: { id: show.id, rating100 },
                }).then(() => saved())
              }
              clickToRate
              withoutContext
            />
            <span className="scene-rating-average">
              Global {show.rating100Average.toFixed(1)}/100 (
              {show.rating100Count})
            </span>
          </span>
        </div>
        <Tab.Container defaultActiveKey="show-details-panel">
          <Nav variant="tabs" className="mr-auto">
            <Nav.Item>
              <Nav.Link eventKey="show-details-panel">Details</Nav.Link>
            </Nav.Item>
            {canManage && (
              <Nav.Item>
                <Nav.Link eventKey="show-edit-panel">Edit</Nav.Link>
              </Nav.Item>
            )}
          </Nav>
          <Tab.Content>
            <Tab.Pane eventKey="show-details-panel">
              <ShowCard show={show} canManage={false} saved={saved} />
            </Tab.Pane>
            {canManage && (
              <Tab.Pane eventKey="show-edit-panel">
                <ShowCard
                  show={show}
                  canManage
                  saved={saved}
                  initiallyEditing
                />
                {canManage && (
                  <Card body className="mt-3">
                    <Card.Title>Site and Cam Models</Card.Title>
                    {associationFailure && (
                      <Alert variant="danger">{associationFailure}</Alert>
                    )}
                    <Form onSubmit={(event) => void saveAssociations(event)}>
                      <Form.Group>
                        <Form.Label>Site</Form.Label>
                        <Form.Control
                          as="select"
                          multiple
                          value={siteIDs}
                          onChange={(event) =>
                            setSiteIDs(
                              Array.from(
                                event.currentTarget.selectedOptions
                              ).map((option) => option.value)
                            )
                          }
                        >
                          {options?.camModelSites
                            .filter((site) => site.enabled)
                            .map((site) => (
                              <option key={site.id} value={site.id}>
                                {site.name}
                              </option>
                            ))}
                        </Form.Control>
                      </Form.Group>
                      <Form.Group>
                        <Form.Label>Cam Models</Form.Label>
                        {options?.camModelProfiles
                          .filter((model) => model.status === "ACTIVE")
                          .map((model) => {
                            const assignment = models.find(
                              (value) => value.modelID === model.id
                            );
                            return (
                              <div
                                className="d-flex align-items-center mb-2"
                                key={model.id}
                              >
                                <Form.Check
                                  className="mr-2"
                                  id={`show-model-${model.id}`}
                                  label={model.displayName}
                                  checked={Boolean(assignment)}
                                  onChange={(event) =>
                                    toggleModel(
                                      model.id,
                                      event.currentTarget.checked
                                    )
                                  }
                                />
                                {assignment && (
                                  <Form.Control
                                    aria-label={`${model.displayName} participation role`}
                                    as="select"
                                    className="ml-auto w-auto"
                                    value={assignment.role}
                                    onChange={(event) =>
                                      setModelRole(
                                        model.id,
                                        event.currentTarget.value
                                      )
                                    }
                                  >
                                    {[
                                      "SOLO",
                                      "PRIMARY",
                                      "GUEST",
                                      "PARTICIPANT",
                                    ].map((role) => (
                                      <option key={role}>{role}</option>
                                    ))}
                                  </Form.Control>
                                )}
                              </div>
                            );
                          })}
                      </Form.Group>
                      <Button type="submit" disabled={associationState.loading}>
                        Save Site and Cam Models
                      </Button>
                    </Form>
                  </Card>
                )}
              </Tab.Pane>
            )}
          </Tab.Content>
        </Tab.Container>
      </div>
      <div className="scene-divider d-none d-xl-block" />
      <div className="scene-player-container">
        {loading && <p>Loading Show video…</p>}
        {error && (
          <Alert variant="danger">Unable to load the Show video.</Alert>
        )}
        {data?.findScene && (
          <div className="scene-player-container">
            <ScenePlayer
              scene={data.findScene}
              hideScrubberOverride={false}
              autoplay={false}
              permitLoop
              initialTimestamp={0}
              sendSetTimestamp={() => undefined}
              onComplete={() => undefined}
              onNext={() => undefined}
              onPrevious={() => undefined}
            />
          </div>
        )}
      </div>
    </div>
  );
};

export const ShowsPage: React.FC = () => {
  const history = useHistory();
  const location = useLocation();
  const { id } = useParams<{ id?: string }>();
  const sort = camShowSortFromSearch(location.search);
  const { data: me } = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const canManage = canManageCamModels(me?.me.capabilities);
  const { data, loading, error, refetch } = GQL.useCamShowsQuery({
    variables: { sort },
    fetchPolicy: "network-only",
  });
  const shows = uniqueCamShows(data?.camShows ?? []);
  const sceneIDs = shows.map((show) => Number(show.sceneID));
  const { data: sceneData, loading: scenesLoading } = GQL.useFindScenesQuery({
    variables: { scene_ids: sceneIDs },
    skip: sceneIDs.length === 0,
  });
  const [gridRef, { width: gridWidth }] = useContainerDimensions();
  const cardWidth = useCardWidth(gridWidth, 0, [280, 340, 480, 640]);
  const showBySceneID = new Map(shows.map((show) => [show.sceneID, show]));
  const changeSort = (next: GQL.CamShowSortMode) => {
    history.replace({
      pathname: location.pathname,
      search: camShowSortSearch(location.search, next),
    });
  };
  const loadError = error
    ? "Unable to load Shows. Your session may have expired; sign in again and return to this page."
    : undefined;

  if (id && !loading && !error) {
    const show = shows.find((candidate) => candidate.id === id);
    if (!show) {
      return (
        <div className="container-fluid">
          <Alert variant="warning">Show {id} was not found.</Alert>
          <Link className="btn btn-secondary" to="/shows">
            Back to Shows
          </Link>
        </div>
      );
    }
    return (
      <ShowDetailsPage show={show} canManage={canManage} saved={refetch} />
    );
  }

  return (
    <div className="container-fluid" data-testid="cam-shows-library">
      <h1>Shows</h1>
      <CamShowSortControl
        sort={sort}
        onChange={changeSort}
        loading={loading}
        error={loadError}
      />
      {!loading && !scenesLoading && !error && (
        <>
          <p className="text-muted">
            {shows.length} classified Shows. Show details are independent
            metadata; Scene is only the media/player bridge.
          </p>
          {!shows.length ? (
            <Alert variant="info">
              No cam-show records have been classified yet.
            </Alert>
          ) : (
            <div className="row justify-content-center" ref={gridRef}>
              {(sceneData?.findScenes.scenes ?? []).map((scene) => {
                const show = showBySceneID.get(scene.id);
                return show ? (
                  <SceneCard
                    key={show.id}
                    width={cardWidth}
                    scene={scene}
                    zoomIndex={0}
                    selecting={false}
                    selected={false}
                    url={`/shows/${show.id}`}
                  />
                ) : null;
              })}
            </div>
          )}
        </>
      )}
    </div>
  );
};
