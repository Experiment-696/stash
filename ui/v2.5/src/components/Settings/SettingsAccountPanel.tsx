import { gql, useMutation, useQuery } from "@apollo/client";
import React from "react";
import { Button, Form } from "react-bootstrap";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { homepageRoutes } from "src/core/homepagePreference";
import { MY_HOMEPAGE_QUERY } from "../FrontPage/HomepageLanding";
import { MY_THEME_QUERY } from "../PersonalThemeLoader";

const SET_MY_HOMEPAGE = gql`
  mutation SetMyHomepage($route: String!) {
    setMyHomepageRoute(route: $route) {
      homepageRoute
    }
  }
`;

const SET_MY_THEME = gql`
  mutation SetMyTheme($themeID: ID) {
    setMyTheme(themeID: $themeID) {
      themeID
    }
  }
`;

const homepageLabels: Record<(typeof homepageRoutes)[number], string> = {
  "/": "Home",
  "/scenes": "Scenes",
  "/performers": "Performers",
  "/studios": "Studios",
  "/galleries": "Galleries",
  "/images": "Images",
  "/groups": "Groups",
  "/tags": "Tags",
};

export const SettingsAccountPanel: React.FC = () => {
  const { data, loading } = useQuery(MY_HOMEPAGE_QUERY);
  const { data: themeData, loading: themesLoading } = useQuery(MY_THEME_QUERY);
  const [route, setRoute] = React.useState("/");
  const [themeID, setThemeID] = React.useState("");
  const [save, { loading: saving, error }] = useMutation(SET_MY_HOMEPAGE);
  const [saveTheme, { loading: savingTheme, error: themeError }] =
    useMutation(SET_MY_THEME);

  React.useEffect(() => {
    setRoute(data?.myPreferences.homepageRoute ?? "/");
  }, [data]);

  React.useEffect(() => {
    setThemeID(themeData?.myPreferences.themeID ?? "");
  }, [themeData]);

  if (loading || themesLoading) return <LoadingIndicator />;
  return (
    <div>
      <h2>Account preferences</h2>
      <Form.Group controlId="account-homepage-route">
        <Form.Label>Page to open from Home</Form.Label>
        <Form.Control
          as="select"
          value={route}
          onChange={(event) => setRoute(event.currentTarget.value)}
        >
          {homepageRoutes.map((value) => (
            <option key={value} value={value}>
              {homepageLabels[value]}
            </option>
          ))}
        </Form.Control>
      </Form.Group>
      <Button
        disabled={saving}
        onClick={() => save({ variables: { route }, refetchQueries: [MY_HOMEPAGE_QUERY] })}
      >
        Save
      </Button>
      {error && <div className="text-danger mt-2">Unable to save homepage preference.</div>}
      <hr />
      <Form.Group controlId="account-theme">
        <Form.Label>Theme</Form.Label>
        <Form.Control
          as="select"
          value={themeID}
          onChange={(event) => setThemeID(event.currentTarget.value)}
        >
          <option value="">Global theme</option>
          {(themeData?.availableThemes ?? []).map(
            (theme: { id: string; name: string }) => (
              <option key={theme.id} value={theme.id}>
                {theme.name}
              </option>
            )
          )}
        </Form.Control>
        <Form.Text muted>
          Themes are installed and enabled by an administrator.
        </Form.Text>
      </Form.Group>
      <Button
        disabled={savingTheme}
        onClick={() =>
          saveTheme({
            variables: { themeID: themeID || null },
            refetchQueries: [MY_THEME_QUERY],
          })
        }
      >
        Apply theme
      </Button>
      {themeError && (
        <div className="text-danger mt-2">Unable to apply theme.</div>
      )}
    </div>
  );
};
