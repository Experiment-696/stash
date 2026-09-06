import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import React from "react";
import renderer from "react-test-renderer";
import { ConfigurationProvider, useConfigurationContext } from "./hooks/Config";
import { migrationBootstrapConfiguration } from "./migrationBootstrapConfiguration";

const migrationStatus = { status: "NEEDS_MIGRATION" } as const;
const MigrationRouteProbe = () => {
  const { configuration } = useConfigurationContext();
  if (
    migrationStatus.status !== "NEEDS_MIGRATION" ||
    configuration.interface.language !== "en-GB"
  ) {
    return null;
  }
  return React.createElement(
    React.Fragment,
    null,
    React.createElement("h1", null, "Migration required"),
    React.createElement("button", null, "Perform schema migration")
  );
};

test("migration configuration renders through the real provider without server data", () => {
  const Consumer = () => {
    const { configuration } = useConfigurationContext();
    return React.createElement(
      "span",
      null,
      `${configuration.interface.language}:${configuration.general.stashes.length}`
    );
  };

  const tree = renderer.create(
    React.createElement(
      ConfigurationProvider,
      { configuration: migrationBootstrapConfiguration },
      React.createElement(Consumer)
    )
  );
  assert.equal(tree.root.findByType("span").children.join(""), "en-GB:0");
  assert.equal(
    migrationBootstrapConfiguration.interface.disableCustomizations,
    true
  );
  assert.deepEqual(migrationBootstrapConfiguration.plugins, {});
  assert.equal(migrationBootstrapConfiguration.general.apiKey, "");
});

test("migrationStatus-only shell renders heading and action through fixed configuration", () => {
  const tree = renderer.create(
    React.createElement(
      ConfigurationProvider,
      { configuration: migrationBootstrapConfiguration },
      React.createElement(MigrationRouteProbe)
    )
  );
  assert.equal(
    tree.root.findByType("h1").children.join(""),
    "Migration required"
  );
  assert.equal(
    tree.root.findByType("button").children.join(""),
    "Perform schema migration"
  );
});

test("missing migration configuration reproduces the blank-shell TypeError", () => {
  const priorError = console.error;
  console.error = () => {};
  try {
    assert.throws(
      () =>
        renderer.create(
          React.createElement(
            ConfigurationProvider,
            { configuration: undefined as never },
            React.createElement(MigrationRouteProbe)
          )
        ),
      /Cannot read properties of undefined \(reading 'interface'\)/
    );
  } finally {
    console.error = priorError;
  }
});

test("App selects fixed migration configuration while skipping normal configuration roots", () => {
  const source = readFileSync(new URL("./App.tsx", import.meta.url), "utf8");
  assert.match(
    source,
    /useAppShellConfigurationQuery\(\{[\s\S]*?skip: migrationBootstrap,/
  );
  assert.match(
    source,
    /useConfigurationQuery\(\{[\s\S]*?skip: !meData\?\.me \|\| migrationShell,/
  );
  assert.match(
    source,
    /data: \{ configuration: migrationBootstrapConfiguration \}/
  );
  assert.doesNotMatch(source, /data: shellConfiguration\s*\?/);
  assert.match(source, /useMigrationStatusQuery\(\{/);
});
