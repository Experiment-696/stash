import React from "react";
import { Tab, Nav, Row, Col, Form } from "react-bootstrap";
import { Redirect, useLocation } from "react-router-dom";
import { LinkContainer } from "react-router-bootstrap";
import { FormattedMessage } from "react-intl";
import { Helmet } from "react-helmet";
import { useTitleProps } from "src/hooks/title";
import { SettingsAboutPanel } from "./SettingsAboutPanel";
import { SettingsConfigurationPanel } from "./SettingsSystemPanel";
import { SettingsInterfacePanel } from "./SettingsInterfacePanel/SettingsInterfacePanel";
import { SettingsLogsPanel } from "./SettingsLogsPanel";
import { SettingsTasksPanel } from "./Tasks/SettingsTasksPanel";
import { SettingsPluginsPanel } from "./SettingsPluginsPanel";
import { SettingsScrapingPanel } from "./SettingsScrapingPanel";
import { SettingsToolsPanel } from "./SettingsToolsPanel";
import { SettingsServicesPanel } from "./SettingsServicesPanel";
import { SettingsContext, useSettings } from "./context";
import { SettingsLibraryPanel } from "./SettingsLibraryPanel";
import { SettingsSecurityPanel } from "./SettingsSecurityPanel";
import { SettingsUsersPanel } from "./SettingsUsersPanel";
import { SettingsAccountPanel } from "./SettingsAccountPanel";
import { useMeQuery } from "src/core/generated-graphql";
import Changelog from "../Changelog/Changelog";
import { TroubleshootingModeButton } from "../TroubleshootingMode/TroubleshootingModeButton";
import { useTroubleshootingMode } from "../TroubleshootingMode/useTroubleshootingMode";

const validTabs = [
  "account",
  "tasks",
  "library",
  "interface",
  "security",
  "users",
  "metadata-providers",
  "services",
  "system",
  "plugins",
  "logs",
  "tools",
  "changelog",
  "about",
] as const;
type TabKey = (typeof validTabs)[number];

const defaultTab: TabKey = "account";

function isTabKey(tab: string | null): tab is TabKey {
  return validTabs.includes(tab as TabKey);
}

const SettingTabs: React.FC<{ tab: TabKey; isAdmin: boolean }> = ({
  tab,
  isAdmin,
}) => {
  const { advancedMode, setAdvancedMode } = useSettings();
  const { isActive: troubleshootingModeActive } = useTroubleshootingMode();

  const titleProps = useTitleProps({ id: "settings" });
  return (
    <Tab.Container activeKey={tab} id="configuration-tabs">
      <Helmet {...titleProps} />
      <Row>
        <Col id="settings-menu-container" sm={3} md={3} xl={2}>
          <Nav variant="pills" className="flex-column">
            <Nav.Item>
              <LinkContainer to="/settings?tab=account">
                <Nav.Link eventKey="account">Account</Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=tasks">
                <Nav.Link eventKey="tasks">
                  <FormattedMessage id="config.categories.tasks" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            {isAdmin && (
              <Nav.Item>
                <LinkContainer to="/settings?tab=users">
                  <Nav.Link eventKey="users">Users</Nav.Link>
                </LinkContainer>
              </Nav.Item>
            )}
            <Nav.Item>
              <LinkContainer to="/settings?tab=library">
                <Nav.Link eventKey="library">
                  <FormattedMessage id="library" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=interface">
                <Nav.Link eventKey="interface">
                  <FormattedMessage id="config.categories.interface" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=security">
                <Nav.Link eventKey="security">
                  <FormattedMessage id="config.categories.security" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=metadata-providers">
                <Nav.Link eventKey="metadata-providers">
                  <FormattedMessage id="config.categories.metadata_providers" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=services">
                <Nav.Link eventKey="services">
                  <FormattedMessage id="config.categories.services" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=system">
                <Nav.Link eventKey="system">
                  <FormattedMessage id="config.categories.system" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=plugins">
                <Nav.Link eventKey="plugins">
                  <FormattedMessage id="config.categories.plugins" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=logs">
                <Nav.Link eventKey="logs">
                  <FormattedMessage id="config.categories.logs" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=tools">
                <Nav.Link eventKey="tools">
                  <FormattedMessage id="config.categories.tools" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=changelog">
                <Nav.Link eventKey="changelog">
                  <FormattedMessage id="config.categories.changelog" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <LinkContainer to="/settings?tab=about">
                <Nav.Link eventKey="about">
                  <FormattedMessage id="config.categories.about" />
                </Nav.Link>
              </LinkContainer>
            </Nav.Item>
            <Nav.Item>
              <div className="advanced-switch">
                <Form.Label htmlFor="advanced-settings">
                  <FormattedMessage id="config.advanced_mode" />
                </Form.Label>
                <Form.Switch
                  id="advanced-settings"
                  checked={advancedMode}
                  onChange={() => setAdvancedMode(!advancedMode)}
                />
              </div>
            </Nav.Item>
            {!troubleshootingModeActive && <TroubleshootingModeButton />}
            <hr className="d-sm-none" />
          </Nav>
        </Col>
        <Col
          id="settings-container"
          sm={{ offset: 3 }}
          md={{ offset: 3 }}
          xl={{ offset: 2 }}
        >
          <Tab.Content className="mx-auto">
            <Tab.Pane eventKey="account">
              <SettingsAccountPanel />
            </Tab.Pane>
            <Tab.Pane eventKey="library">
              <SettingsLibraryPanel />
            </Tab.Pane>
            <Tab.Pane eventKey="interface">
              <SettingsInterfacePanel />
            </Tab.Pane>
            <Tab.Pane eventKey="security">
              <SettingsSecurityPanel />
            </Tab.Pane>
            {isAdmin && (
              <Tab.Pane eventKey="users" unmountOnExit>
                <SettingsUsersPanel />
              </Tab.Pane>
            )}
            <Tab.Pane eventKey="tasks">
              <SettingsTasksPanel />
            </Tab.Pane>
            <Tab.Pane eventKey="services" unmountOnExit>
              <SettingsServicesPanel />
            </Tab.Pane>
            <Tab.Pane eventKey="tools" unmountOnExit>
              <SettingsToolsPanel />
            </Tab.Pane>
            <Tab.Pane eventKey="metadata-providers" unmountOnExit>
              <SettingsScrapingPanel />
            </Tab.Pane>
            <Tab.Pane eventKey="system">
              <SettingsConfigurationPanel />
            </Tab.Pane>
            <Tab.Pane eventKey="plugins" unmountOnExit>
              <SettingsPluginsPanel />
            </Tab.Pane>
            <Tab.Pane eventKey="logs" unmountOnExit>
              <SettingsLogsPanel />
            </Tab.Pane>
            <Tab.Pane eventKey="changelog" unmountOnExit>
              <Changelog />
            </Tab.Pane>
            <Tab.Pane eventKey="about" unmountOnExit>
              <SettingsAboutPanel />
            </Tab.Pane>
          </Tab.Content>
        </Col>
      </Row>
    </Tab.Container>
  );
};

export const Settings: React.FC = () => {
  const location = useLocation();
  const tab = new URLSearchParams(location.search).get("tab");
  const { data: meData, loading } = useMeQuery({ fetchPolicy: "no-cache" });
  const isAdmin = meData?.me.role === "ADMIN" && meData.me.status === "ACTIVE";

  if (loading) {
    return null;
  }

  if (tab === "cam-shows" && isAdmin) {
    return (
      <Redirect
        to={{
          ...location,
          search: "tab=library",
          hash: "#cam-show-classification",
        }}
      />
    );
  }

  if (
    !isTabKey(tab) ||
    (!isAdmin && tab !== "account" && tab !== "tasks")
  ) {
    return (
      <Redirect
        to={{
          ...location,
          search: `tab=${defaultTab}`,
        }}
      />
    );
  }

  return (
    <SettingsContext enabled={isAdmin}>
      <SettingTabs tab={tab} isAdmin={isAdmin} />
    </SettingsContext>
  );
};

export default Settings;
