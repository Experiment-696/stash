import { faUserFriends, faVideo } from "@fortawesome/free-solid-svg-icons";
import { IconDefinition } from "@fortawesome/fontawesome-svg-core";

import { Icon } from "src/components/Shared/Icon";
export { CamModelsPage } from "src/components/CamModels/CamModelsPage";
export { ShowsPage } from "src/components/CamShows/ShowsPage";
import {
  getConfiguredTrustedRegistryItems,
  insertTrustedRegistryMenuItems,
  insertTrustedRegistryItems,
  isTrustedRegistryRouteEnabled,
  ITrustedRegistryItem,
} from "src/trustedExtensionsRegistry";

export const trustedExtensionsEnabled = true;

export interface ITrustedNavItem extends ITrustedRegistryItem {
  icon: IconDefinition;
}

const trustedNavItems: readonly ITrustedNavItem[] = Object.freeze([
  {
    id: "cam-shows.shows",
    menuKey: "shows",
    label: "Shows",
    path: "/shows",
    afterPath: "/scenes",
    hotkey: "g h",
    capability: "library.read",
    icon: faVideo,
  },
  {
    id: "cam-shows.models",
    menuKey: "cam-models",
    label: "Cam Models",
    path: "/cam-models",
    afterPath: "/performers",
    hotkey: "g c",
    capability: "library.read",
    icon: faUserFriends,
  },
]);

export function getTrustedNavItems(
  capabilities: readonly string[] | undefined,
  configuredMenuItems?: readonly string[] | null,
  enabled = trustedExtensionsEnabled
): readonly ITrustedNavItem[] {
  return getConfiguredTrustedRegistryItems(
    trustedNavItems,
    capabilities,
    configuredMenuItems,
    enabled
  );
}

export function getTrustedMenuItems(enabled = trustedExtensionsEnabled) {
  return enabled ? trustedNavItems : [];
}

export function insertTrustedMenuItems<T extends { id: string }>(
  stockItems: readonly T[],
  extensionItems: readonly ITrustedNavItem[]
) {
  return insertTrustedRegistryMenuItems(stockItems, extensionItems);
}

export function isTrustedRouteEnabled(
  path: string,
  capabilities: readonly string[] | undefined,
  enabled = trustedExtensionsEnabled
): boolean {
  return isTrustedRegistryRouteEnabled(
    trustedNavItems,
    path,
    capabilities,
    enabled
  );
}

export function insertTrustedNavItems<
  T extends { href: string; hotkey: string }
>(
  stockItems: readonly T[],
  extensionItems: readonly ITrustedNavItem[]
): Array<T | ITrustedNavItem> {
  return insertTrustedRegistryItems(stockItems, extensionItems);
}

export function TrustedNavIcon({ item }: { item: ITrustedNavItem }) {
  return <Icon icon={item.icon} />;
}
