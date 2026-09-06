export interface SelectableTheme {
  id: string;
  name: string;
}

export function selectedThemeID(
  requestedID: string | null | undefined,
  availableThemes: readonly SelectableTheme[] | null | undefined
): string | undefined {
  if (!requestedID) return undefined;
  return availableThemes?.some((theme) => theme.id === requestedID)
    ? requestedID
    : undefined;
}

export function selectedThemeStylesheetPath(
  requestedID: string | null | undefined,
  availableThemes: readonly SelectableTheme[] | null | undefined
): string | undefined {
  const id = selectedThemeID(requestedID, availableThemes);
  return id ? "theme.css" : undefined;
}
