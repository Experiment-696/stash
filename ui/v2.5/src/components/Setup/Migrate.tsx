import React, { useMemo, useState } from "react";
import { Button, Card, Container } from "react-bootstrap";
import { FormattedMessage } from "react-intl";
import { useHistory } from "react-router-dom";
import { baseURL } from "src/core/createClient";
import * as GQL from "src/core/generated-graphql";
import {
  useMigrationStatus,
  mutateMigrate,
  postMigrate,
} from "src/core/StashService";
import { migrationNotes } from "src/docs/en/MigrationNotes";
import { ExternalLink } from "../Shared/ExternalLink";
import { LoadingIndicator } from "../Shared/LoadingIndicator";
import { MarkdownPage } from "../Shared/MarkdownPage";

export const Migrate: React.FC = () => {
  const history = useHistory();

  const { data: migrationStatus, loading } = useMigrationStatus();

  const [migrateLoading, setMigrateLoading] = useState(false);
  const [migrateError, setMigrateError] = useState("");

  // if database path includes path separators, then this is passed through
  // to the migration path. Extract the base name of the database file.
  const discordLink = (
    <ExternalLink href="https://discord.gg/2TsNFKt">Discord</ExternalLink>
  );
  const githubLink = (
    <ExternalLink href="https://github.com/stashapp/stash/issues">
      <FormattedMessage id="setup.github_repository" />
    </ExternalLink>
  );

  const status = migrationStatus?.migrationStatus;

  const maybeMigrationNotes = useMemo(() => {
    if (
      !status ||
      status.currentSchema === undefined ||
      status.requiredSchema === undefined
    )
      return;

    const notes = [];
    for (let i = status.currentSchema + 1; i <= status.requiredSchema; ++i) {
      const note = migrationNotes[i];
      if (note) {
        notes.push(note);
      }
    }

    if (notes.length === 0) return;

    return (
      <div className="migration-notes">
        <h2>
          <FormattedMessage id="setup.migrate.migration_notes" />
        </h2>
        <div>
          {notes.map((n, i) => (
            <div key={i}>
              <MarkdownPage page={n} />
            </div>
          ))}
        </div>
      </div>
    );
  }, [status]);

  // only display setup wizard if system is not setup
  if (loading || !migrationStatus || !status) {
    return <LoadingIndicator />;
  }

  if (migrateLoading) {
    return (
      <div className="migrate-loading-status">
        <h4>
          <LoadingIndicator inline small message="" />
          <span>
            <FormattedMessage id="setup.migrate.migrating_database" />
          </span>
        </h4>
      </div>
    );
  }

  if (status.status !== GQL.SystemStatusEnum.NeedsMigration) {
    // redirect to main page
    history.replace("/");
    return <LoadingIndicator />;
  }

  async function onMigrate() {
    try {
      setMigrateLoading(true);
      setMigrateError("");

      // migrate now uses the job manager
      await mutateMigrate();
      postMigrate();
      window.location.assign(`${baseURL}login`);
    } catch (e) {
      if (e instanceof Error) setMigrateError(e.message ?? e.toString());
      setMigrateLoading(false);
    }
  }

  function maybeRenderError() {
    if (!migrateError) {
      return;
    }

    return (
      <section>
        <h2 className="text-danger">
          <FormattedMessage id="setup.migrate.migration_failed" />
        </h2>

        <p>
          <FormattedMessage id="setup.migrate.migration_failed_error" />
        </p>

        <Card>
          <pre>{migrateError}</pre>
        </Card>

        <p>
          <FormattedMessage
            id="setup.migrate.migration_failed_help"
            values={{ discordLink, githubLink }}
          />
        </p>
      </section>
    );
  }

  return (
    <Container>
      <h1 className="text-center mb-3">
        <FormattedMessage id="setup.migrate.migration_required" />
      </h1>
      <Card>
        <section>
          <p>
            <FormattedMessage
              id="setup.migrate.schema_too_old"
              values={{
                databaseSchema: <strong>{status.currentSchema}</strong>,
                appSchema: <strong>{status.requiredSchema}</strong>,
                strong: (chunks: string) => <strong>{chunks}</strong>,
                code: (chunks: string) => <code>{chunks}</code>,
              }}
            />
          </p>

          <p className="lead text-center my-5">
            <FormattedMessage id="setup.migrate.migration_irreversible_warning" />
          </p>

          <p>A managed database backup will be created before migration.</p>
        </section>

        {maybeMigrationNotes}

        <section>
          <div className="d-flex justify-content-center">
            <Button variant="primary mx-2 p-5" onClick={() => onMigrate()}>
              <FormattedMessage id="setup.migrate.perform_schema_migration" />
            </Button>
          </div>
        </section>

        {maybeRenderError()}
      </Card>
    </Container>
  );
};

export default Migrate;
