export const canManageCamModels = (capabilities?: readonly string[] | null) =>
  capabilities?.includes("data.admin") ?? false;

export const camModelProfileValidation = (name: string) =>
  name.trim() ? undefined : "Display name is required.";

export const camModelAccountValidation = (siteID: string, handle: string) => {
  if (!siteID) return "Choose a site.";
  if (!handle.trim()) return "Username is required.";
  return undefined;
};

export const camModelDate = (value?: string | null) =>
  value ? new Date(value).toLocaleDateString() : "Unknown";

export const camModelAccountPeriod = (
  from?: string | null,
  to?: string | null
) => camModelDate(from) + " – " + (to ? camModelDate(to) : "Current");

export const camModelError = (error: unknown) =>
  error instanceof Error
    ? error.message
    : "The operation could not be completed.";
