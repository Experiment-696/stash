import React, { useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Card,
  Col,
  Form,
  Row,
  Spinner,
  Table,
} from "react-bootstrap";
import { Link, useHistory, useParams } from "react-router-dom";
import { ExternalLink } from "src/components/Shared/ExternalLink";
import { FavoriteIcon } from "src/components/Shared/FavoriteIcon";
import * as GQL from "src/core/generated-graphql";
import {
  camModelAccountPeriod,
  canManageCamModels,
  camModelAccountValidation,
  camModelError,
  camModelProfileValidation,
} from "./camModelUi";
import { CamGirlFinderSearchCard } from "./CamGirlFinderSearchCard";
import { CamModelConfirmedAction } from "./CamModelConfirmedAction";

const Loading = () => (
  <div className="p-4">
    <Spinner animation="border" /> <span>Loading Cam Models…</span>
  </div>
);

const CamModelList: React.FC = () => {
  const { data: meData } = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const canManage = canManageCamModels(meData?.me.capabilities);
  const history = useHistory();
  const [favoritesOnly, setFavoritesOnly] = useState(false);
  const { data, loading, error, refetch } = GQL.useCamModelProfilesQuery({
    variables: { favoritesOnly },
    fetchPolicy: "no-cache",
  });
  const [setUserState, favoriteState] = GQL.useCamModelSetUserStateMutation();
  const [create] = GQL.useCamModelProfileCreateMutation();
  const [name, setName] = useState("");
  const [failure, setFailure] = useState<string>();
  if (loading) return <Loading />;
  if (error)
    return (
      <Alert variant="danger">
        Unable to load Cam Models. Sign in with library access and try again.
      </Alert>
    );
  const profiles = data?.camModelProfiles ?? [];
  async function toggleFavorite(
    id: string,
    favorite: boolean,
    rating100?: number | null
  ) {
    setFailure(undefined);
    try {
      await setUserState({ variables: { id, favorite, rating100 } });
      await refetch();
    } catch (e) {
      setFailure(camModelError(e));
    }
  }
  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const validation = camModelProfileValidation(name);
    if (validation) {
      setFailure(validation);
      return;
    }
    setFailure(undefined);
    try {
      const result = await create({
        variables: {
          input: { displayName: name.trim(), status: "ACTIVE", accounts: [] },
        },
      });
      const id = result.data?.camModelProfileCreate.id;
      await refetch();
      if (id) history.push("/cam-models/" + id);
    } catch (e) {
      setFailure(camModelError(e));
    }
  }
  return (
    <div className="container-fluid" data-testid="cam-model-list">
      <h1>Cam Models</h1>
      <p className="text-muted">
        Durable profiles keep site accounts and username history together.
        Provider evidence always waits for review and never merges identity
        automatically.
      </p>
      {failure && (
        <Alert variant="danger" role="alert">
          {failure}
        </Alert>
      )}
      {canManage && (
        <Card body className="mb-4">
          <Form inline onSubmit={(e) => void submit(e)}>
            <Form.Label className="mr-2" htmlFor="cam-model-name">
              New profile
            </Form.Label>
            <Form.Control
              id="cam-model-name"
              value={name}
              onChange={(e) => setName(e.currentTarget.value)}
              placeholder="Display name"
            />
            <Button className="ml-2" type="submit">
              Create
            </Button>
          </Form>
        </Card>
      )}
      <Form.Group controlId="cam-model-view" className="mb-3">
        <Form.Label>Model view</Form.Label>
        <Form.Control
          as="select"
          value={favoritesOnly ? "favorites" : "all"}
          onChange={(e) =>
            setFavoritesOnly(e.currentTarget.value === "favorites")
          }
        >
          <option value="all">All Models</option>
          <option value="favorites">Favorite Models</option>
        </Form.Control>
        <Form.Text muted>
          Favorites are private to your account. Favorite Models are ordered by
          name, then stable profile ID.
        </Form.Text>
      </Form.Group>
      {!profiles.length ? (
        <Alert variant="info">
          {favoritesOnly
            ? "No favorite Cam Models yet."
            : "No Cam Model profiles yet. Create the first profile above."}
        </Alert>
      ) : (
        <Row>
          {profiles.map((profile) => (
            <Col md={6} xl={4} key={profile.id} className="mb-3">
              <Card body>
                <Card.Title className="d-flex align-items-center justify-content-between">
                  <Link to={"/cam-models/" + profile.id}>
                    {profile.displayName}
                  </Link>
                  <FavoriteIcon
                    favorite={profile.favorite}
                    onToggleFavorite={(value) =>
                      void toggleFavorite(profile.id, value, profile.rating100)
                    }
                    className={favoriteState.loading ? "disabled" : undefined}
                  />
                </Card.Title>
                <Badge
                  variant={
                    profile.status === "ACTIVE" ? "success" : "secondary"
                  }
                >
                  {profile.status}
                </Badge>
                <p className="mt-2 mb-0">
                  {profile.accounts.length} site account
                  {profile.accounts.length === 1 ? "" : "s"} ·{" "}
                  {
                    profile.evidence.filter((e) => e.reviewState === "PENDING")
                      .length
                  }{" "}
                  pending review
                </p>
              </Card>
            </Col>
          ))}
        </Row>
      )}
    </div>
  );
};

const CamModelDetail: React.FC<{ id: string }> = ({ id }) => {
  const { data: meData } = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const canManage = canManageCamModels(meData?.me.capabilities);
  const { data, loading, error, refetch } = GQL.useCamModelProfileQuery({
    variables: { id },
    fetchPolicy: "no-cache",
  });
  const [update] = GQL.useCamModelProfileUpdateMutation();
  const [addAccount] = GQL.useCamModelAccountAddMutation();
  const [retire] = GQL.useCamModelAccountRetireMutation();
  const [review] = GQL.useCamModelEvidenceReviewMutation();
  const [createSocial] = GQL.useCamModelSocialProfileCreateMutation();
  const [retireSocial] = GQL.useCamModelSocialProfileRetireMutation();
  const [setUserState, favoriteState] = GQL.useCamModelSetUserStateMutation();
  const [name, setName] = useState<string>();
  const [status, setStatus] = useState<string>();
  const [siteID, setSiteID] = useState("");
  const [handle, setHandle] = useState("");
  const [socialPlatform, setSocialPlatform] = useState("");
  const [socialHandle, setSocialHandle] = useState("");
  const [socialURL, setSocialURL] = useState("");
  const [failure, setFailure] = useState<string>();
  const [notice, setNotice] = useState<string>();
  if (loading) return <Loading />;
  if (error)
    return (
      <Alert variant="danger">Unable to load this Cam Model profile.</Alert>
    );
  const profile = data?.camModelProfile;
  if (!profile)
    return <Alert variant="warning">Cam Model profile not found.</Alert>;
  async function toggleFavorite(value: boolean) {
    setFailure(undefined);
    try {
      await setUserState({
        variables: { id, favorite: value, rating100: profile?.rating100 },
      });
      await refetch();
    } catch (e) {
      setFailure(camModelError(e));
    }
  }
  async function run(action: () => Promise<unknown>, message: string) {
    setFailure(undefined);
    setNotice(undefined);
    try {
      await action();
      await refetch();
      setNotice(message);
      return true;
    } catch (e) {
      setFailure(camModelError(e));
      return false;
    }
  }
  const editName = name ?? profile.displayName,
    editStatus = status ?? profile.status;
  async function save(e: React.FormEvent) {
    e.preventDefault();
    const validation = camModelProfileValidation(editName);
    if (validation) {
      setFailure(validation);
      return;
    }
    await run(
      () =>
        update({
          variables: {
            input: {
              id,
              displayName: editName.trim(),
              image: profile!.image,
              notes: profile!.notes,
              performerID: profile!.performerID,
              status: editStatus,
            },
          },
        }),
      "Profile updated."
    );
  }
  async function add(e: React.FormEvent) {
    e.preventDefault();
    const validation = camModelAccountValidation(siteID, handle);
    if (validation) {
      setFailure(validation);
      return;
    }
    await run(
      () =>
        addAccount({
          variables: {
            input: { modelID: id, account: { siteID, handle: handle.trim() } },
          },
        }),
      "Site account added."
    );
    setHandle("");
  }
  async function addSocial(e: React.FormEvent) {
    e.preventDefault();
    if (!socialPlatform.trim() || !/^https?:\/\//i.test(socialURL)) {
      setFailure("Platform and a valid http(s) profile URL are required.");
      return;
    }
    await run(
      () =>
        createSocial({
          variables: {
            input: {
              modelID: id,
              platform: socialPlatform.trim(),
              handle: socialHandle.trim() || null,
              profileURL: socialURL.trim(),
              source: "MANUAL",
            },
          },
        }),
      "Social/media profile added."
    );
    setSocialPlatform("");
    setSocialHandle("");
    setSocialURL("");
  }
  async function archiveSocial(profileID: string) {
    const ok = await run(
      () =>
        retireSocial({
          variables: { id: profileID, validTo: new Date().toISOString() },
        }),
      "Social/media profile moved to history."
    );
    if (!ok) throw new Error("archive social profile failed");
  }
  async function retireAccount(accountID: string) {
    const ok = await run(
      () =>
        retire({
          variables: { id: accountID, validTo: new Date().toISOString() },
        }),
      "Username moved to history."
    );
    if (!ok) throw new Error("retire account failed");
  }
  return (
    <div className="container-fluid" data-testid="cam-model-detail">
      <Link to="/cam-models">← All Cam Models</Link>
      <div className="d-flex align-items-center">
        <h1 className="mr-2">{profile.displayName}</h1>
        <FavoriteIcon
          favorite={profile.favorite}
          onToggleFavorite={(value) => void toggleFavorite(value)}
          className={favoriteState.loading ? "disabled" : undefined}
        />
      </div>
      <p className="text-muted">
        Identity changes are manual. Evidence approval records review only; it
        does not create accounts or merge profiles.
      </p>
      {failure && (
        <Alert variant="danger" role="alert">
          {failure}
        </Alert>
      )}
      {notice && <Alert variant="success">{notice}</Alert>}
      {canManage && (
        <Card body className="mb-4">
          <h2>Profile</h2>
          <Form onSubmit={(e) => void save(e)}>
            <Form.Group controlId="profile-name">
              <Form.Label>Display name</Form.Label>
              <Form.Control
                value={editName}
                onChange={(e) => setName(e.currentTarget.value)}
              />
            </Form.Group>
            <Form.Group controlId="profile-status">
              <Form.Label>Status</Form.Label>
              <Form.Control
                as="select"
                value={editStatus}
                onChange={(e) => setStatus(e.currentTarget.value)}
              >
                <option>ACTIVE</option>
                <option>INACTIVE</option>
                <option>UNKNOWN</option>
              </Form.Control>
            </Form.Group>
            <Button type="submit">Save profile</Button>
          </Form>
        </Card>
      )}
      <Card body className="mb-4">
        <h2>Site accounts and username history</h2>
        {!profile.accounts.length ? (
          <Alert variant="info">No site accounts yet.</Alert>
        ) : (
          <Table responsive striped>
            <thead>
              <tr>
                <th>Site</th>
                <th>Username</th>
                <th>Dates</th>
                <th>Status/source</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {profile.accounts.map((a) => (
                <tr key={a.id}>
                  <td>{a.site.name}</td>
                  <td>
                    {a.profileURL ? (
                      <ExternalLink href={a.profileURL}>
                        {a.handle}
                      </ExternalLink>
                    ) : (
                      a.handle
                    )}
                  </td>
                  <td>{camModelAccountPeriod(a.validFrom, a.validTo)}</td>
                  <td>
                    <Badge variant={a.validTo ? "secondary" : "success"}>
                      {a.validTo ? "HISTORICAL" : "CURRENT"}
                    </Badge>{" "}
                    {a.status} · {a.source}
                  </td>
                  <td>
                    {canManage && !a.validTo && (
                      <CamModelConfirmedAction
                        testID={"cam-model-retire-account-" + a.id}
                        title="Move username to history?"
                        description="This marks the current username historical. No identity is merged or deleted."
                        triggerLabel="Move to history"
                        confirmLabel="Move username to history"
                        triggerSize="sm"
                        triggerVariant="outline-secondary"
                        onConfirm={() => retireAccount(a.id)}
                      />
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
        {canManage &&
          (!data?.camModelSites.length ? (
            <Alert variant="warning">
              No sites are configured yet. Site administration must be completed
              before adding an account.
            </Alert>
          ) : (
            <Form inline onSubmit={(e) => void add(e)}>
              <Form.Control
                as="select"
                value={siteID}
                onChange={(e) => setSiteID(e.currentTarget.value)}
              >
                <option value="">Choose site</option>
                {data.camModelSites.map((s) => (
                  <option value={s.id} key={s.id}>
                    {s.name}
                  </option>
                ))}
              </Form.Control>
              <Form.Control
                className="ml-2"
                value={handle}
                onChange={(e) => setHandle(e.currentTarget.value)}
                placeholder="Username"
              />
              <Button className="ml-2" type="submit">
                Add account
              </Button>
            </Form>
          ))}
      </Card>
      <Card body className="mb-4" data-testid="cam-model-social-profiles">
        <h2>Social and media profiles</h2>
        <p className="text-muted">
          Typed profile links are separate from cam-site accounts and retain
          active/history status and provenance.
        </p>
        {!profile.socialProfiles.length ? (
          <Alert variant="info">No social or media profiles yet.</Alert>
        ) : (
          profile.socialProfiles.map((social) => (
            <div
              className="d-flex justify-content-between mb-2"
              key={social.id}
            >
              <span>
                {social.icon && <span className="mr-1">{social.icon}</span>}
                <ExternalLink href={social.profileURL}>
                  {social.handle
                    ? social.platform + " · @" + social.handle
                    : social.platform}
                </ExternalLink>{" "}
                <Badge variant={social.validTo ? "secondary" : "success"}>
                  {social.validTo ? "HISTORICAL" : "ACTIVE"}
                </Badge>{" "}
                · {social.source}
                {social.provenance ? " · " + social.provenance : ""}
              </span>
              {canManage && !social.validTo && (
                <CamModelConfirmedAction
                  testID={"cam-model-archive-social-" + social.id}
                  title="Move social/media profile to history?"
                  description="This archives this social/media profile link while preserving its provenance and history."
                  triggerLabel="Move to history"
                  confirmLabel="Move profile to history"
                  triggerSize="sm"
                  triggerVariant="outline-secondary"
                  onConfirm={() => archiveSocial(social.id)}
                />
              )}
            </div>
          ))
        )}
        {canManage && (
          <Form inline onSubmit={(e) => void addSocial(e)}>
            <Form.Control
              value={socialPlatform}
              onChange={(e) => setSocialPlatform(e.currentTarget.value)}
              placeholder="Platform (X, Telegram…)"
            />
            <Form.Control
              className="ml-2"
              value={socialHandle}
              onChange={(e) => setSocialHandle(e.currentTarget.value)}
              placeholder="Handle (optional)"
            />
            <Form.Control
              className="ml-2"
              value={socialURL}
              onChange={(e) => setSocialURL(e.currentTarget.value)}
              placeholder="https://profile…"
            />
            <Button className="ml-2" type="submit">
              Add profile
            </Button>
          </Form>
        )}
      </Card>
      {canManage && <CamGirlFinderSearchCard modelID={id} onIngest={refetch} />}
      <Card body>
        <h2>Provenance and review evidence</h2>
        {!profile.evidence.length ? (
          <Alert variant="info">No provider evidence has been observed.</Alert>
        ) : (
          profile.evidence.map((e) => (
            <Card body className="mb-2" key={e.id}>
              <div>
                <strong>{e.provider}</strong> · <code>{e.evidenceKey}</code>{" "}
                <Badge
                  variant={
                    e.reviewState === "PENDING"
                      ? "warning"
                      : e.reviewState === "APPROVED"
                      ? "success"
                      : "danger"
                  }
                >
                  {e.reviewState}
                </Badge>
              </div>
              <div>
                Observed {new Date(e.observedAt).toLocaleString()}{" "}
                {e.confidence != null && "· confidence " + e.confidence}
              </div>
              {e.sourceURL && (
                <ExternalLink href={e.sourceURL}>
                  Open source evidence
                </ExternalLink>
              )}
              {canManage && (
                <details>
                  <summary>Raw provider evidence</summary>
                  <pre>{e.payloadJSON}</pre>
                </details>
              )}
              {canManage && e.reviewState === "PENDING" && (
                <div className="mt-2">
                  <Button
                    size="sm"
                    variant="success"
                    onClick={() =>
                      void run(
                        () =>
                          review({
                            variables: { id: e.id, state: "APPROVED" },
                          }),
                        "Evidence approved; identity was not changed."
                      )
                    }
                  >
                    Approve evidence
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    className="ml-2"
                    onClick={() =>
                      void run(
                        () =>
                          review({
                            variables: { id: e.id, state: "REJECTED" },
                          }),
                        "Evidence rejected."
                      )
                    }
                  >
                    Reject evidence
                  </Button>
                </div>
              )}
            </Card>
          ))
        )}
      </Card>
    </div>
  );
};

export const CamModelsPage: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  return id ? <CamModelDetail id={id} /> : <CamModelList />;
};
