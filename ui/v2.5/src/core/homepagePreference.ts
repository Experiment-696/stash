export const homepageRoutes = [
  "/",
  "/scenes",
  "/performers",
  "/studios",
  "/galleries",
  "/images",
  "/groups",
  "/tags",
] as const;

export function safeHomepageDestination(route: unknown): string {
  return typeof route === "string" &&
    (homepageRoutes as readonly string[]).includes(route)
    ? route
    : "/";
}
