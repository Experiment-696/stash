export const csrfCookieName = "stash_csrf_v2";

export function getCSRFToken(cookieHeader = document.cookie): string | undefined {
  const prefix = `${csrfCookieName}=`;
  const entry = cookieHeader
    .split(";")
    .map((value) => value.trim())
    .find((value) => value.startsWith(prefix));
  return entry ? decodeURIComponent(entry.slice(prefix.length)) : undefined;
}

export function csrfHeaders(): Record<string, string> {
  const token = getCSRFToken();
  return token ? { "X-CSRF-Token": token } : {};
}
