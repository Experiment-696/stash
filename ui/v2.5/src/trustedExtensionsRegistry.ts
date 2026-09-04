export interface ITrustedRegistryItem {
  id: string;
  menuKey: string;
  label: string;
  path: string;
  afterPath: string;
  hotkey: string;
  capability: string;
}

export interface ITrustedRegistryMenuItem {
  id: string;
}

export const trustedRegistryConfigurationMarker =
  "cam-shows.navigation.configured.v1";

interface IStockRegistryItem {
  href: string;
  hotkey: string;
}

export function getEnabledTrustedRegistryItems<
  T extends Pick<ITrustedRegistryItem, "capability">
>(
  items: readonly T[],
  capabilities: readonly string[] | undefined,
  enabled: boolean
): readonly T[] {
  if (!enabled || !capabilities) return [];
  return items.filter((item) => capabilities.includes(item.capability));
}

export function getConfiguredTrustedRegistryItems<
  T extends Pick<ITrustedRegistryItem, "capability" | "menuKey">
>(
  items: readonly T[],
  capabilities: readonly string[] | undefined,
  configuredMenuItems: readonly string[] | null | undefined,
  enabled: boolean
): readonly T[] {
  const capableItems = getEnabledTrustedRegistryItems(
    items,
    capabilities,
    enabled
  );
  if (
    !configuredMenuItems ||
    !configuredMenuItems.includes(trustedRegistryConfigurationMarker)
  ) {
    return capableItems;
  }
  return capableItems.filter((item) =>
    configuredMenuItems.includes(item.menuKey)
  );
}

export function resolveTrustedRegistryMenuSelection<
  T extends Pick<ITrustedRegistryItem, "menuKey">
>(
  items: readonly T[],
  configuredMenuItems: readonly string[] | null | undefined,
  enabled: boolean
): string[] | undefined {
  if (!configuredMenuItems) return undefined;
  const result = configuredMenuItems.filter(
    (item) => item !== trustedRegistryConfigurationMarker
  );
  if (
    enabled &&
    !configuredMenuItems.includes(trustedRegistryConfigurationMarker)
  ) {
    for (const item of items) {
      if (!result.includes(item.menuKey)) result.push(item.menuKey);
    }
  }
  return result;
}

export function serializeTrustedRegistryMenuSelection<
  T extends Pick<ITrustedRegistryItem, "menuKey">
>(items: readonly T[], selectedMenuItems: readonly string[], enabled: boolean) {
  const trustedKeys = new Set(items.map((item) => item.menuKey));
  const result = selectedMenuItems.filter(
    (item) =>
      item !== trustedRegistryConfigurationMarker &&
      (enabled || !trustedKeys.has(item))
  );
  if (enabled) result.push(trustedRegistryConfigurationMarker);
  return result;
}

export function insertTrustedRegistryMenuItems<
  T extends ITrustedRegistryMenuItem,
  E extends Pick<ITrustedRegistryItem, "menuKey" | "label" | "afterPath">
>(stockItems: readonly T[], extensionItems: readonly E[]) {
  const result: Array<T | { id: string; heading: string }> = [...stockItems];
  for (const item of extensionItems) {
    const afterKey = item.afterPath.replace(/^\//, "");
    const anchor = result.findIndex((entry) => entry.id === afterKey);
    result.splice(anchor >= 0 ? anchor + 1 : result.length, 0, {
      id: item.menuKey,
      heading: item.label,
    });
  }
  return result;
}

export function isTrustedRegistryRouteEnabled(
  items: readonly ITrustedRegistryItem[],
  path: string,
  capabilities: readonly string[] | undefined,
  enabled: boolean
): boolean {
  return getEnabledTrustedRegistryItems(items, capabilities, enabled).some(
    (item) => item.path === path
  );
}

export function insertTrustedRegistryItems<
  T extends IStockRegistryItem,
  E extends ITrustedRegistryItem
>(stockItems: readonly T[], extensionItems: readonly E[]): Array<T | E> {
  const result: Array<T | E> = [...stockItems];
  const seenIDs = new Set<string>();
  const seenPaths = new Set(stockItems.map((item) => item.href));
  const seenHotkeys = new Set(stockItems.map((item) => item.hotkey));

  for (const item of extensionItems) {
    if (seenIDs.has(item.id)) {
      throw new Error(`Trusted extension duplicate id: ${item.id}`);
    }
    seenIDs.add(item.id);
    if (seenPaths.has(item.path)) {
      throw new Error(`Trusted extension duplicate path: ${item.path}`);
    }
    seenPaths.add(item.path);
    if (seenHotkeys.has(item.hotkey)) {
      throw new Error(`Trusted extension duplicate hotkey: ${item.hotkey}`);
    }
    seenHotkeys.add(item.hotkey);

    const anchor = result.findIndex((entry) =>
      "href" in entry
        ? entry.href === item.afterPath
        : entry.path === item.afterPath
    );
    let insertionIndex = anchor >= 0 ? anchor + 1 : result.length;
    while (insertionIndex < result.length) {
      const followingItem = result[insertionIndex];
      if (!("afterPath" in followingItem) || followingItem.afterPath !== item.afterPath) {
        break;
      }
      insertionIndex += 1;
    }
    result.splice(insertionIndex, 0, item);
  }

  return result;
}
