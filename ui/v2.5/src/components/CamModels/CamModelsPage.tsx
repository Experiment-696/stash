import React, { useState } from "react";
import {
  Alert,
  Badge,
  Button,
  Card,
  Form,
  Spinner,
  Tab,
  Table,
  Tabs,
} from "react-bootstrap";
import { Link, useHistory, useParams } from "react-router-dom";
import { ExternalLink } from "src/components/Shared/ExternalLink";
import { FavoriteIcon } from "src/components/Shared/FavoriteIcon";
import {
  GridCard,
  useCardWidth,
  useContainerDimensions,
} from "src/components/Shared/GridCard/GridCard";
import { RatingBanner } from "src/components/Shared/RatingBanner";
import { RatingSystem } from "src/components/Shared/Rating/RatingSystem";
import { DetailImage } from "src/components/Shared/DetailImage";
import { BackgroundImage } from "src/components/Shared/DetailsPage/BackgroundImage";
import { DetailTitle } from "src/components/Shared/DetailsPage/DetailTitle";
import { HeaderImage } from "src/components/Shared/DetailsPage/HeaderImage";
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

const CamSiteEditor: React.FC<{
  site?: GQL.CamModelSiteDataFragment;
  saved: () => Promise<unknown>;
}> = ({ site, saved }) => {
  const [name, setName] = useState(site?.name ?? "");
  const [baseURL, setBaseURL] = useState(site?.baseURL ?? "");
  const [externalKey, setExternalKey] = useState(site?.externalKey ?? "");
  const [icon, setIcon] = useState(site?.icon ?? "");
  const [enabled, setEnabled] = useState(site?.enabled ?? true);
  const [save, state] = GQL.useCamModelSiteSaveMutation();
  return (
    <Form
      className="d-flex flex-wrap align-items-center mb-2"
      onSubmit={async (event) => {
        event.preventDefault();
        await save({
          variables: {
            input: {
              id: site?.id,
              name: name.trim(),
              baseURL: baseURL.trim() || null,
              externalKey: externalKey.trim() || null,
              icon: icon.trim() || null,
              enabled,
            },
          },
        });
        await saved();
        if (!site) {
          setName("");
          setBaseURL("");
          setExternalKey("");
          setIcon("");
        }
      }}
    >
      {icon && (
        <img src={icon} alt="" width={24} height={24} className="mr-2" />
      )}
      <Form.Control
        className="mr-2 mb-1"
        value={name}
        onChange={(e) => setName(e.currentTarget.value)}
        placeholder="Site name"
        required
      />
      <Form.Control
        className="mr-2 mb-1"
        value={baseURL}
        onChange={(e) => setBaseURL(e.currentTarget.value)}
        placeholder="https://site.example"
      />
      <Form.Control
        className="mr-2 mb-1"
        value={externalKey}
        onChange={(e) => setExternalKey(e.currentTarget.value)}
        placeholder="Provider key"
      />
      <Form.Control
        className="mr-2 mb-1"
        value={icon}
        onChange={(e) => setIcon(e.currentTarget.value)}
        placeholder="Icon path"
      />
      <Form.Check
        className="mr-2"
        checked={enabled}
        onChange={(e) => setEnabled(e.currentTarget.checked)}
        label="Enabled"
      />
      <Button type="submit" size="sm" disabled={state.loading}>
        {site ? "Save" : "Add site"}
      </Button>
    </Form>
  );
};

const CamSiteManager: React.FC<{
  sites: GQL.CamModelSiteDataFragment[];
  saved: () => Promise<unknown>;
}> = ({ sites, saved }) => (
  <Card body className="mb-4" data-testid="cam-site-manager">
    <h2>Cam Sites</h2>
    <p className="text-muted">
      Sites are shared. Each Cam Model can have a different username on every
      site.
    </p>
    {sites.map((site) => (
      <CamSiteEditor key={site.id} site={site} saved={saved} />
    ))}
    <hr />
    <CamSiteEditor saved={saved} />
  </Card>
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
  const [gridRef, { width: gridWidth }] = useContainerDimensions();
  const cardWidth = useCardWidth(gridWidth, 1, [240, 300, 375, 470]);
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
        Use CamGirlFinder from a model profile to add aliases and profile details.
      </p>
      {failure && (
        <Alert variant="danger" role="alert">
          {failure}
        </Alert>
      )}
      {canManage && (
        <>
          <details className="mb-4">
            <summary className="btn btn-secondary">Manage Cam Sites</summary>
            <CamSiteManager sites={data?.camModelSites ?? []} saved={refetch} />
          </details>
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
        </>
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
        <div className="row justify-content-center" ref={gridRef}>
          {profiles.map((profile) => (
            <GridCard
              key={profile.id}
              className="performer-card zoom-1 cam-model-card"
              url={"/cam-models/" + profile.id}
              width={cardWidth}
              title={
                <span className="performer-name">{profile.displayName}</span>
              }
              image={
                profile.image ? (
                  <img
                    loading="lazy"
                    className="performer-card-image"
                    alt={profile.displayName}
                    src={profile.image}
                  />
                ) : (
                  <div
                    className="performer-card-image d-flex align-items-center justify-content-center bg-secondary"
                    role="img"
                    aria-label={`${profile.displayName} has no image`}
                  >
                    <span className="display-1 text-muted">?</span>
                  </div>
                )
              }
              overlays={
                <>
                  <FavoriteIcon
                    favorite={profile.favorite}
                    onToggleFavorite={(value) =>
                      void toggleFavorite(profile.id, value, profile.rating100)
                    }
                    size="2x"
                    className={`hide-not-favorite${
                      favoriteState.loading ? " disabled" : ""
                    }`}
                  />
                  {!!profile.rating100 && (
                    <RatingBanner rating={profile.rating100} />
                  )}
                </>
              }
              details={
                <>
                  <Badge
                    variant={
                      profile.status === "ACTIVE" ? "success" : "secondary"
                    }
                  >
                    {profile.status}
                  </Badge>
                  <div className="mt-2">
                    {profile.accounts.length} site account
                    {profile.accounts.length === 1 ? "" : "s"}
                  </div>
                </>
              }
            />
          ))}
        </div>
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
  const [scrapeProfile, scrapeState] = GQL.useCamModelProfileScrapeMutation();
  const [createSocial] = GQL.useCamModelSocialProfileCreateMutation();
  const [retireSocial] = GQL.useCamModelSocialProfileRetireMutation();
  const [setUserState, favoriteState] = GQL.useCamModelSetUserStateMutation();
  const [name, setName] = useState<string>();
  const [status, setStatus] = useState<string>();
  const [image, setImage] = useState<string>();
  const [notes, setNotes] = useState<string>();
  const [location, setLocation] = useState<string>();
  const [age, setAge] = useState<string>();
  const [isEditing, setIsEditing] = useState(false);
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
    editStatus = status ?? profile.status,
    editImage = image ?? profile.image ?? "",
    editNotes = notes ?? profile.notes ?? "",
    editLocation = location ?? profile.location ?? "",
    editAge = age ?? (profile.age ? String(profile.age) : "");
  async function save(e: React.FormEvent) {
    e.preventDefault();
    const validation = camModelProfileValidation(editName);
    if (validation) {
      setFailure(validation);
      return;
    }
    const saved = await run(
      () =>
        update({
          variables: {
            input: {
              id,
              displayName: editName.trim(),
              image: editImage.trim() || null,
              notes: editNotes.trim() || null,
              location: editLocation.trim() || null,
              age: editAge.trim() ? Number(editAge) : null,
              performerID: profile!.performerID,
              status: editStatus,
            },
          },
        }),
      "Profile updated."
    );
    if (saved) setIsEditing(false);
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
  async function scrapeAccount(accountID: string) {
    const ok = await run(
      () => scrapeProfile({ variables: { accountID } }),
      "Public profile information imported. Existing values were preserved."
    );
    if (!ok) throw new Error("profile scrape failed");
  }
  return (
    <div id="performer-page" className="row" data-testid="cam-model-detail">
      <div className="detail-header">
        <BackgroundImage
          imagePath={profile.image ?? undefined}
          show={!isEditing}
        />
        <div className="detail-container">
          <HeaderImage encodingImage={false}>
            {!!profile.image && (
              <DetailImage
                className="performer"
                src={profile.image}
                alt={profile.displayName}
              />
            )}
          </HeaderImage>
          <div className="row">
            <div className="performer-head col">
              <Link to="/cam-models">← All Cam Models</Link>
              <DetailTitle
                name={profile.displayName}
                classNamePrefix="performer"
              >
                <span className="name-icons">
                  <FavoriteIcon
                    favorite={profile.favorite}
                    onToggleFavorite={(value) => void toggleFavorite(value)}
                    className={favoriteState.loading ? "disabled" : undefined}
                  />
                </span>
              </DetailTitle>
              <div className="quality-group mb-3">
                <RatingSystem
                  value={profile.rating100}
                  onSetRating={(rating100) =>
                    void setUserState({
                      variables: { id, favorite: profile.favorite, rating100 },
                    }).then(() => refetch())
                  }
                  clickToRate
                  withoutContext
                />
                <span className="ml-2 text-muted">Your rating</span>
              </div>
              {!isEditing && (
                <div className="mb-3">
                  <Badge
                    variant={
                      profile.status === "ACTIVE" ? "success" : "secondary"
                    }
                  >
                    {profile.status}
                  </Badge>
                  {profile.notes && (
                    <p className="mt-2 mb-0">{profile.notes}</p>
                  )}
                  {(profile.location || profile.age) && (
                    <p className="mt-2 mb-0 text-muted">
                      {[profile.location, profile.age ? `Age ${profile.age}` : null]
                        .filter(Boolean)
                        .join(" · ")}
                    </p>
                  )}
                </div>
              )}
              <p className="text-muted">
                Site-specific usernames and aliases identify the same Cam Model
                across different cam sites.
              </p>
              {failure && (
                <Alert variant="danger" role="alert">
                  {failure}
                </Alert>
              )}
              {notice && <Alert variant="success">{notice}</Alert>}
              {canManage && !isEditing && (
                <Button className="mb-3" onClick={() => setIsEditing(true)}>
                  Edit
                </Button>
              )}
              {canManage && isEditing && (
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
                  <Form.Group controlId="profile-image">
                    <Form.Label>Image URL</Form.Label>
                    <Form.Control
                      value={editImage}
                      onChange={(e) => setImage(e.currentTarget.value)}
                      placeholder="https://…"
                    />
                  </Form.Group>
                  <Form.Group controlId="profile-notes">
                    <Form.Label>Notes</Form.Label>
                    <Form.Control
                      as="textarea"
                      value={editNotes}
                      onChange={(e) => setNotes(e.currentTarget.value)}
                    />
                  </Form.Group>
                  <Form.Group controlId="profile-location">
                    <Form.Label>Location</Form.Label>
                    <Form.Control
                      value={editLocation}
                      onChange={(e) => setLocation(e.currentTarget.value)}
                    />
                  </Form.Group>
                  <Form.Group controlId="profile-age">
                    <Form.Label>Age</Form.Label>
                    <Form.Control
                      type="number"
                      min={18}
                      max={120}
                      value={editAge}
                      onChange={(e) => setAge(e.currentTarget.value)}
                    />
                  </Form.Group>
                  <Button type="submit">Save</Button>{" "}
                  <Button
                    variant="secondary"
                    onClick={() => setIsEditing(false)}
                  >
                    Cancel
                  </Button>
                </Form>
              )}
            </div>
          </div>
        </div>
      </div>
      <div className="detail-body">
        <div className="performer-body">
          <Tabs id="cam-model-tabs" defaultActiveKey="aliases" mountOnEnter>
            <Tab
              eventKey="aliases"
              title={`Site aliases (${profile.accounts.length})`}
            >
              <Card body className="mb-4">
                <h2>Site aliases and username history</h2>
                {!profile.accounts.length ? (
                  <Alert variant="info">No site accounts yet.</Alert>
                ) : (
                  <Table responsive striped>
                    <thead>
                      <tr>
                        <th>Site</th>
                        <th>Site alias</th>
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
                          <td>
                            {camModelAccountPeriod(a.validFrom, a.validTo)}
                          </td>
                          <td>
                            <Badge
                              variant={a.validTo ? "secondary" : "success"}
                            >
                              {a.validTo ? "HISTORICAL" : "CURRENT"}
                            </Badge>{" "}
                            {a.status} · {a.source}
                          </td>
                          <td>
                            {canManage && a.profileURL && !a.validTo && (
                              <Button
                                size="sm"
                                className="mr-2"
                                disabled={scrapeState.loading}
                                onClick={() => void scrapeAccount(a.id)}
                              >
                                {scrapeState.loading
                                  ? "Scraping…"
                                  : "Scrape public profile"}
                              </Button>
                            )}
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
                      No sites are configured yet. Site administration must be
                      completed before adding an account.
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
                        placeholder="Username / alias"
                      />
                      <Button className="ml-2" type="submit">
                        Add site alias
                      </Button>
                    </Form>
                  ))}
              </Card>
            </Tab>
            <Tab
              eventKey="social"
              title={`Social profiles (${profile.socialProfiles.length})`}
            >
              <Card
                body
                className="mb-4"
                data-testid="cam-model-social-profiles"
              >
                <h2>Social and media profiles</h2>
                <p className="text-muted">
                  Typed profile links are separate from cam-site accounts and
                  retain active/history status and provenance.
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
                        {social.icon && (
                          <span className="mr-1">{social.icon}</span>
                        )}
                        <ExternalLink href={social.profileURL}>
                          {social.handle
                            ? social.platform + " · @" + social.handle
                            : social.platform}
                        </ExternalLink>{" "}
                        <Badge
                          variant={social.validTo ? "secondary" : "success"}
                        >
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
            </Tab>
            {canManage && (
              <Tab eventKey="finder" title="CamGirlFinder">
                <CamGirlFinderSearchCard modelID={id} onIngest={refetch} />
              </Tab>
            )}
          </Tabs>
        </div>
      </div>
    </div>
  );
};

export const CamModelsPage: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  return id ? <CamModelDetail id={id} /> : <CamModelList />;
};
