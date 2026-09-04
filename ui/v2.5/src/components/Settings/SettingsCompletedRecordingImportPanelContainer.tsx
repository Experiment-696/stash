import React from "react";
import { Alert, Spinner } from "react-bootstrap";
import * as GQL from "../../core/generated-graphql";
import { useSettings } from "./context";
import { canManageCamModels } from "../CamModels/camModelUi";
import { SettingsCompletedRecordingImportView } from "./SettingsCompletedRecordingImportPanel";

export const SettingsCompletedRecordingImportPanel: React.FC = () => {
  const { general } = useSettings();
  const me = GQL.useMeQuery({ fetchPolicy: "no-cache" });
  const canManage = canManageCamModels(me.data?.me.capabilities);
  const config = GQL.useCompletedRecordingImportConfigQuery({
    fetchPolicy: "network-only",
    skip: !canManage,
  });
  const [configureMutation] =
    GQL.useCompletedRecordingImportConfigureMutation();
  const [previewMutation] = GQL.useCompletedRecordingPreviewMutation();
  const [applyMutation] = GQL.useCompletedRecordingApplyMutation();
  if (me.loading) {
    return (
      <div role="status">
        <Spinner animation="border" size="sm" /> Loading completed recording
        import configuration…
      </div>
    );
  }
  if (!canManage) return null;
  if (config.loading) {
    return (
      <div role="status">
        <Spinner animation="border" size="sm" /> Loading completed recording
        import configuration…
      </div>
    );
  }
  if (config.error || !config.data) {
    return (
      <Alert variant="danger" role="alert">
        Unable to load completed recording import configuration.
      </Alert>
    );
  }
  return (
    <SettingsCompletedRecordingImportView
      roots={general.stashes ?? []}
      enabled={config.data.completedRecordingImportConfig.enabled}
      configuredRoot={config.data.completedRecordingImportConfig.root}
      configure={async (input) => {
        const response = await configureMutation({ variables: { input } });
        if (!response.data)
          throw new Error("Configuration returned no result.");
        return response.data.completedRecordingImportConfigure;
      }}
      preview={async (input) => {
        const response = await previewMutation({ variables: { input } });
        if (!response.data) throw new Error("Preview returned no result.");
        return response.data.completedRecordingPreview;
      }}
      apply={async (input) => {
        const response = await applyMutation({ variables: { input } });
        if (!response.data) throw new Error("Apply returned no result.");
        return response.data.completedRecordingApply;
      }}
    />
  );
};
