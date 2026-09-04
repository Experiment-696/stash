import { gql, useQuery } from "@apollo/client";
import React from "react";
import { Redirect } from "react-router-dom";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { safeHomepageDestination } from "src/core/homepagePreference";
import FrontPage from "./FrontPage";

export const MY_HOMEPAGE_QUERY = gql`
  query MyHomepage {
    myPreferences {
      homepageRoute
    }
  }
`;

interface MyHomepageData {
  myPreferences: { homepageRoute: string };
}

export const HomepageLanding: React.FC = () => {
  const { data, loading, error } = useQuery<MyHomepageData>(MY_HOMEPAGE_QUERY, {
    fetchPolicy: "network-only",
  });

  if (loading) return <LoadingIndicator />;
  const route = safeHomepageDestination(
    error ? "/" : data?.myPreferences.homepageRoute
  );
  // This component is mounted only for the exact root route, so redirecting to
  // any other allowlisted route happens once and cannot form a redirect loop.
  return route === "/" ? <FrontPage /> : <Redirect to={route} />;
};

export default HomepageLanding;
