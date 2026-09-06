import { gql, useQuery } from "@apollo/client";
import React from "react";
import { getPlatformURL } from "src/core/createClient";
import { selectedThemeStylesheetPath } from "src/core/themePreference";
import { useCSS } from "src/hooks/useScript";

export const MY_THEME_QUERY = gql`
  query MyTheme {
    myPreferences {
      themeID
    }
    availableThemes {
      id
      name
    }
  }
`;

interface MyThemeData {
  myPreferences: { themeID?: string | null };
  availableThemes: Array<{ id: string; name: string }>;
}

export const PersonalThemeLoader: React.FC<React.PropsWithChildren<{}>> = ({
  children,
}) => {
  const { data, loading, error } = useQuery<MyThemeData>(MY_THEME_QUERY, {
    fetchPolicy: "network-only",
  });
  const path = loading || error
    ? undefined
    : selectedThemeStylesheetPath(
        data?.myPreferences.themeID,
        data?.availableThemes
      );
  const stylesheet = path ? getPlatformURL(path).toString() : "";
  useCSS(stylesheet, !!stylesheet);

  return <>{children}</>;
};
