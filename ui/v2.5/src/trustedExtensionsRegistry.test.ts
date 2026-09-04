import assert from "node:assert/strict";
import test from "node:test";

import {
  getEnabledTrustedRegistryItems,
  getConfiguredTrustedRegistryItems,
  insertTrustedRegistryMenuItems,
  insertTrustedRegistryItems,
  isTrustedRegistryRouteEnabled,
  resolveTrustedRegistryMenuSelection,
  serializeTrustedRegistryMenuSelection,
  trustedRegistryConfigurationMarker,
} from "./trustedExtensionsRegistry.js";

const stock = [
  { href: "/scenes", hotkey: "g s" },
  { href: "/performers", hotkey: "g p" },
];

const extension = (overrides: Record<string, string> = {}) => ({
  id: "cam-shows.shows",
  menuKey: "shows",
  label: "Shows",
  path: "/shows",
  afterPath: "/scenes",
  hotkey: "g h",
  capability: "library.read",
  ...overrides,
});

const trustedItems = [
  extension(),
  extension({
    id: "cam-shows.models",
    menuKey: "cam-models",
    label: "Cam Models",
    path: "/cam-models",
    afterPath: "/performers",
    hotkey: "g c",
  }),
];

test("fails closed when disabled, anonymous, or missing capability", () => {
  assert.deepEqual(
    getEnabledTrustedRegistryItems(trustedItems, ["library.read"], false),
    []
  );
  assert.deepEqual(
    getEnabledTrustedRegistryItems(trustedItems, undefined, true),
    []
  );
  assert.deepEqual(
    getEnabledTrustedRegistryItems(trustedItems, ["metadata.write"], true),
    []
  );
});

test("returns only items allowed by server capabilities", () => {
  const mixed = [
    ...trustedItems,
    extension({
      id: "admin",
      path: "/admin",
      hotkey: "g a",
      capability: "system.configure",
    }),
  ];
  assert.deepEqual(
    getEnabledTrustedRegistryItems(mixed, ["library.read"], true).map(
      (item) => item.path
    ),
    ["/shows", "/cam-models"]
  );
});

test("legacy menu configuration defaults trusted entries visible", () => {
  assert.deepEqual(
    getConfiguredTrustedRegistryItems(
      trustedItems,
      ["library.read"],
      ["scenes", "performers"],
      true
    ).map((item) => item.menuKey),
    ["shows", "cam-models"]
  );
  assert.deepEqual(
    resolveTrustedRegistryMenuSelection(
      trustedItems,
      ["scenes", "performers"],
      true
    ),
    ["scenes", "performers", "shows", "cam-models"]
  );
});

test("persisted selection supports independent toggles including both off", () => {
  const persisted = serializeTrustedRegistryMenuSelection(
    trustedItems,
    ["scenes", "cam-models"],
    true
  );
  assert.deepEqual(persisted, [
    "scenes",
    "cam-models",
    trustedRegistryConfigurationMarker,
  ]);
  assert.deepEqual(
    getConfiguredTrustedRegistryItems(
      trustedItems,
      ["library.read"],
      persisted,
      true
    ).map((item) => item.menuKey),
    ["cam-models"]
  );
  const bothOff = serializeTrustedRegistryMenuSelection(
    trustedItems,
    ["scenes"],
    true
  );
  assert.deepEqual(
    getConfiguredTrustedRegistryItems(
      trustedItems,
      ["library.read"],
      bothOff,
      true
    ),
    []
  );
});

test("disable or uninstall strips trusted-only configuration metadata", () => {
  assert.deepEqual(
    serializeTrustedRegistryMenuSelection(
      trustedItems,
      ["scenes", "shows", "cam-models", trustedRegistryConfigurationMarker],
      false
    ),
    ["scenes"]
  );
});

test("settings order uses the same trusted registry anchors and labels", () => {
  assert.deepEqual(
    insertTrustedRegistryMenuItems(
      [
        { id: "scenes", headingID: "scenes" },
        { id: "performers", headingID: "performers" },
      ],
      trustedItems
    ),
    [
      { id: "scenes", headingID: "scenes" },
      { id: "shows", heading: "Shows" },
      { id: "performers", headingID: "performers" },
      { id: "cam-models", heading: "Cam Models" },
    ]
  );
});

test("route predicate has exact parity with visible navigation", () => {
  for (const enabled of [true, false]) {
    for (const capabilities of [
      undefined,
      [],
      ["metadata.write"],
      ["library.read"],
    ] as Array<readonly string[] | undefined>) {
      const visiblePaths = getEnabledTrustedRegistryItems(
        trustedItems,
        capabilities,
        enabled
      ).map((item) => item.path);
      for (const path of ["/shows", "/cam-models", "/unknown"]) {
        assert.equal(
          isTrustedRegistryRouteEnabled(
            trustedItems,
            path,
            capabilities,
            enabled
          ),
          visiblePaths.includes(path)
        );
      }
    }
  }
});

test("inserts same-anchor items in declaration order", () => {
  const items = insertTrustedRegistryItems(stock, [
    extension(),
    extension({ id: "second", path: "/second", hotkey: "g 2" }),
  ]);
  assert.deepEqual(
    items.map((item) => ("href" in item ? item.href : item.path)),
    ["/scenes", "/shows", "/second", "/performers"]
  );
});

for (const [name, items, message] of [
  [
    "id",
    [extension(), extension({ path: "/other", hotkey: "g o" })],
    "duplicate id",
  ],
  [
    "path",
    [extension(), extension({ id: "other", hotkey: "g o" })],
    "duplicate path",
  ],
  ["stock path", [extension({ path: "/scenes" })], "duplicate path"],
  [
    "hotkey",
    [extension(), extension({ id: "other", path: "/other" })],
    "duplicate hotkey",
  ],
  ["stock hotkey", [extension({ hotkey: "g s" })], "duplicate hotkey"],
] as const) {
  test(`rejects duplicate ${name} with an observable diagnostic`, () => {
    assert.throws(
      () => insertTrustedRegistryItems(stock, items),
      new RegExp(message)
    );
  });
}
