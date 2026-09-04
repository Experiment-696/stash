import * as GQL from "src/core/generated-graphql";

export function useRoleCapabilities() {
  const { data } = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const isAdmin = data?.me.role === "ADMIN";
  return {
    isAdmin,
    canEditMetadata: isAdmin || data?.me.role === "MODERATOR",
  };
}
