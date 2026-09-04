import React, { useState } from "react";
import { Alert, Badge, Button, Card, Form } from "react-bootstrap";
import { Link, useHistory, useLocation } from "react-router-dom";
import { ExternalLink } from "src/components/Shared/ExternalLink";
import * as GQL from "src/core/generated-graphql";
import { canManageCamModels } from "src/components/CamModels/camModelUi";
import {
  CamShowSortControl,
  camShowSortFromSearch,
  camShowSortSearch,
  uniqueCamShows,
} from "./camShowSortUi";

type Show = NonNullable<GQL.CamShowsQuery["camShows"]>[number];

const ShowCard: React.FC<{
  show: Show;
  canManage: boolean;
  saved: () => Promise<unknown>;
}> = ({ show, canManage, saved }) => {
  const [editing, setEditing] = useState(false),
    [title, setTitle] = useState(show.title),
    [showType, setShowType] = useState(show.showType),
    [precision, setPrecision] = useState(show.capturedPrecision ?? ""),
    [details, setDetails] = useState(show.details ?? ""),
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
            capturedAt: show.capturedAt,
            capturedTimezone: show.capturedTimezone,
            capturedPrecision: precision || null,
            durationOverrideSeconds: show.durationOverridden
              ? show.durationSeconds
              : null,
            durationOverrideReason: show.durationOverrideReason,
            details: details.trim() || null,
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
              {[
                "LIVE_PUBLIC",
                "LIVE_GROUP_TICKET_MULTIUSER",
                "LIVE_PRIVATE",
                "LIVE_EXCLUSIVE_PRIVATE",
                "CUSTOM_VIDEO",
                "PRIVATE_CALL",
              ].map((v) => (
                <option key={v}>{v}</option>
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
            <Form.Label>Details / notes</Form.Label>
            <Form.Control
              as="textarea"
              value={details}
              onChange={(e) => setDetails(e.currentTarget.value)}
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
            <Link to={"/scenes/" + show.sceneID}>{show.title}</Link>
          </Card.Title>
          <div>
            <Badge variant="secondary">
              {show.showType.replaceAll("_", " ")}
            </Badge>
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
          {show.details && <p className="mt-2 mb-1">{show.details}</p>}
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
          <Link className="btn btn-primary mt-3" to={"/scenes/" + show.sceneID}>
            Open Scene player
          </Link>
          {canManage && (
            <Button
              className="mt-3 ml-2"
              variant="outline-primary"
              onClick={() => setEditing(true)}
            >
              Edit Show
            </Button>
          )}
        </>
      )}
    </Card>
  );
};

export const ShowsPage: React.FC = () => {
  const history = useHistory();
  const location = useLocation();
  const sort = camShowSortFromSearch(location.search);
  const { data: me } = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const canManage = canManageCamModels(me?.me.capabilities);
  const { data, loading, error, refetch } = GQL.useCamShowsQuery({
    variables: { sort },
    fetchPolicy: "network-only",
  });
  const shows = uniqueCamShows(data?.camShows ?? []);
  const changeSort = (next: GQL.CamShowSortMode) => {
    history.replace({
      pathname: location.pathname,
      search: camShowSortSearch(location.search, next),
    });
  };
  const loadError = error
    ? "Unable to load Shows. Your session may have expired; sign in again and return to this page."
    : undefined;

  return (
    <div className="container-fluid" data-testid="cam-shows-library">
      <h1>Shows</h1>
      <CamShowSortControl
        sort={sort}
        onChange={changeSort}
        loading={loading}
        error={loadError}
      />
      {!loading && !error && (
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
            <div className="row">
              {shows.map((show) => (
                <div className="col-12 col-md-6 col-xl-4 mb-3" key={show.id}>
                  <ShowCard show={show} canManage={canManage} saved={refetch} />
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
};
