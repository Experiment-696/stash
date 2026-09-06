import React, { useState } from "react";
import { useIntl } from "react-intl";
import { LoadingIndicator } from "src/components/Shared/LoadingIndicator";
import { LibraryTasks } from "./LibraryTasks";
import { DataManagementTasks } from "./DataManagementTasks";
import { PluginTasks } from "./PluginTasks";
import { JobTable } from "./JobTable";
import * as GQL from "src/core/generated-graphql";

export const SettingsTasksPanel: React.FC = () => {
  const intl = useIntl();
  const { data: meData } = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const isAdmin = meData?.me.role === "ADMIN";
  const [isBackupRunning, setIsBackupRunning] = useState<boolean>(false);
  const [isAnonymiseRunning, setIsAnonymiseRunning] = useState<boolean>(false);

  if (isBackupRunning) {
    return (
      <LoadingIndicator
        message={intl.formatMessage({ id: "config.tasks.backing_up_database" })}
      />
    );
  }

  if (isAnonymiseRunning) {
    return (
      <LoadingIndicator
        message={intl.formatMessage({
          id: "config.tasks.anonymising_database",
        })}
      />
    );
  }

  return (
    <div id="tasks-panel">
      {isAdmin && (
        <div className="tasks-panel-queue">
          <h1>{intl.formatMessage({ id: "config.tasks.job_queue" })}</h1>
          <JobTable />
        </div>
      )}

      {isAdmin && (
        <div className="tasks-panel-tasks">
          <LibraryTasks />
          <hr />
          <DataManagementTasks
            setIsBackupRunning={setIsBackupRunning}
            setIsAnonymiseRunning={setIsAnonymiseRunning}
          />
          <hr />
          <PluginTasks />
        </div>
      )}
    </div>
  );
};
