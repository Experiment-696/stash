import React, { Suspense, useEffect, useMemo, useState } from "react";
import {
  Route,
  Switch,
  useHistory,
  useLocation,
  useRouteMatch,
} from "react-router-dom";
import { IntlProvider, CustomFormats, FormattedMessage } from "react-intl";
import { Helmet } from "react-helmet";
import cloneDeep from "lodash-es/cloneDeep";
import mergeWith from "lodash-es/mergeWith";
import { ToastProvider } from "src/hooks/Toast";
import { LightboxProvider } from "src/hooks/Lightbox/context";
import { initPolyfills } from "src/polyfills";

import locales, { registerCountry } from "src/locales";
import { useConfigureUISetting } from "src/core/StashService";
import flattenMessages from "./utils/flattenMessages";
import * as yup from "yup";
import Mousetrap from "mousetrap";
import MousetrapPause from "mousetrap-pause";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { MainNavbar } from "./components/MainNavbar";
import { PageNotFound } from "./components/PageNotFound";
import * as GQL from "./core/generated-graphql";
import { makeTitleProps } from "./hooks/title";
import { LoadingIndicator } from "./components/Shared/LoadingIndicator";

import {
  ConfigurationProvider,
  useConfigurationContextOptional,
} from "./hooks/Config";
import { ManualProvider } from "./components/Help/context";
import { InteractiveProvider } from "./hooks/Interactive/context";
import { ReleaseNotesDialog } from "./components/Dialogs/ReleaseNotesDialog";
import { releaseNotes } from "./docs/en/ReleaseNotes";
import { getPlatformURL } from "./core/createClient";
import { lazyComponent } from "./utils/lazyComponent";
import { isPlatformUniquelyRenderedByApple } from "./utils/apple";
import Event from "./hooks/event";
import { migrationBootstrapConfiguration } from "./migrationBootstrapConfiguration";

import { PluginRoutes, PluginsLoader } from "./plugins";
import {
  CamModelsPage,
  isTrustedRouteEnabled,
  ShowsPage,
} from "./trustedExtensions";

// import plugin_api to run code
import "./pluginApi";
import { ConnectionMonitor } from "./ConnectionMonitor";
import { TroubleshootingModeOverlay } from "./components/TroubleshootingMode/TroubleshootingModeOverlay";
import { PatchFunction } from "./patch";
import { PersonalThemeLoader } from "./components/PersonalThemeLoader";

import moment from "moment/min/moment-with-locales";
import { ErrorMessage } from "./components/Shared/ErrorMessage";
import cx from "classnames";

const Performers = lazyComponent(
  () => import("./components/Performers/Performers")
);
const HomepageLanding = lazyComponent(
  () => import("./components/FrontPage/HomepageLanding")
);
const Scenes = lazyComponent(() => import("./components/Scenes/Scenes"));
const Settings = lazyComponent(() => import("./components/Settings/Settings"));
const Stats = lazyComponent(() => import("./components/Stats"));
const Studios = lazyComponent(() => import("./components/Studios/Studios"));
const Galleries = lazyComponent(
  () => import("./components/Galleries/Galleries")
);

const Groups = lazyComponent(() => import("./components/Groups/Groups"));
const Tags = lazyComponent(() => import("./components/Tags/Tags"));
const Images = lazyComponent(() => import("./components/Images/Images"));
const Setup = lazyComponent(() => import("./components/Setup/Setup"));
const Migrate = lazyComponent(() => import("./components/Setup/Migrate"));

const SceneFilenameParser = lazyComponent(
  () => import("./components/SceneFilenameParser/SceneFilenameParser")
);
const SceneDuplicateChecker = lazyComponent(
  () => import("./components/SceneDuplicateChecker/SceneDuplicateChecker")
);

const appleRendering = isPlatformUniquelyRenderedByApple();

initPolyfills();

MousetrapPause(Mousetrap);

const intlFormats: CustomFormats = {
  date: {
    long: { year: "numeric", month: "long", day: "numeric" },
  },
};

const defaultLocale = "en-GB";

function languageMessageString(language: string) {
  return language.replace(/-/, "");
}

const AppContainer: React.FC<React.PropsWithChildren<{}>> = PatchFunction(
  "App",
  (props: React.PropsWithChildren<{}>) => {
    return <>{props.children}</>;
  }
) as React.FC;

const MainContainer: React.FC = ({ children }) => {
  // use optional here because the configuration may have be loading or errored
  const { configuration } = useConfigurationContextOptional() || {};
  const { sfwContentMode } = configuration?.interface || {};

  return (
    <div
      className={cx("main container-fluid", {
        apple: appleRendering,
        "sfw-content-mode": sfwContentMode,
      })}
    >
      {children}
    </div>
  );
};

function translateLanguageLocale(l: string) {
  // intl doesn't support all locales, so we need to map some to supported ones
  switch (l) {
    case "nn-NO":
      // use other Norwegian locale for intl
      return "nb-NO";
    default:
      return l;
  }
}

export const App: React.FC = () => {
  const [dismissedReleaseNotesThrough, setDismissedReleaseNotesThrough] =
    React.useState<number>();
  const migrationBootstrap = Boolean(
    document.querySelector('meta[name="stash-migration-csrf"]')
  );
  const migration = GQL.useMigrationStatusQuery({
    fetchPolicy: "no-cache",
    skip: !migrationBootstrap,
  });
  const shell = GQL.useAppShellConfigurationQuery({
    fetchPolicy: "no-cache",
    skip: migrationBootstrap,
  });
  const shellResult = shell.data?.appShellConfiguration;
  const migrationShell = migrationBootstrap;
  const { data: meData } = GQL.useMeQuery({
    fetchPolicy: "no-cache",
    skip: !shellResult || migrationShell,
  });
  const fullConfig = GQL.useConfigurationQuery({
    fetchPolicy: "no-cache",
    skip: !meData?.me || migrationShell,
  });
  const [saveUISetting] = useConfigureUISetting();
  const config = migrationShell
    ? {
        // The migration GraphQL allowlist intentionally excludes normal
        // configuration roots. Supply a fixed, non-secret client contract so
        // providers can render /migrate without dereferencing skipped query
        // data or inventing server state.
        data: { configuration: migrationBootstrapConfiguration },
        loading: migration.loading,
        error: migration.error,
      }
    : fullConfig;
  const shellStatus =
    migration.data?.migrationStatus.status ?? shellResult?.status;
  const systemStatusData = useMemo(
    () => (shellStatus ? { systemStatus: { status: shellStatus } } : undefined),
    [shellStatus]
  );

  const language =
    config.data?.configuration?.interface?.language ?? defaultLocale;
  const intlLanguage = translateLanguageLocale(language);

  // use en-GB as default messages if any messages aren't found in the chosen language
  const [messages, setMessages] = useState<{}>();
  const [customMessages, setCustomMessages] = useState<{}>();

  useEffect(() => {
    (async () => {
      try {
        const res = await fetch(getPlatformURL("customlocales"));
        if (res.ok) {
          setCustomMessages(await res.json());
        }
      } catch (err) {
        console.log(err);
      }
    })();
  }, []);

  useEffect(() => {
    const setLocale = async () => {
      const defaultMessageLanguage = languageMessageString(defaultLocale);
      const messageLanguage = languageMessageString(language);

      try {
        // register countries for the chosen language
        await registerCountry(language);

        const defaultMessages = (await locales[defaultMessageLanguage]())
          .default;
        const mergedMessages = cloneDeep(Object.assign({}, defaultMessages));
        const chosenLocale = locales[messageLanguage];
        const chosenMessages = chosenLocale
          ? (await chosenLocale()).default
          : defaultMessages;

        mergeWith(
          mergedMessages,
          chosenMessages,
          customMessages,
          (objVal, srcVal) => {
            if (srcVal === "") {
              return objVal;
            }
          }
        );

        const newMessages = flattenMessages(mergedMessages);

        yup.setLocale({
          mixed: {
            required: newMessages["validation.required"],
          },
        });

        setMessages(newMessages);
        moment.locale([language, defaultLocale]);
      } catch (err) {
        console.error(
          `Unable to load locale "${language}"; using ${defaultLocale}`,
          err
        );
        const fallbackMessages = (await locales[defaultMessageLanguage]())
          .default;
        setMessages(flattenMessages(cloneDeep(fallbackMessages)));
        moment.locale(defaultLocale);
      }
    };

    void setLocale();
  }, [customMessages, language]);

  const location = useLocation();
  const history = useHistory();
  const setupMatch = useRouteMatch(["/setup", "/migrate"]);

  // dispatch event when location changes
  useEffect(() => {
    Event.dispatch("location", "", { location });
  }, [location]);

  // redirect to setup or migrate as needed
  useEffect(() => {
    if (!systemStatusData) {
      return;
    }

    const { status } = systemStatusData.systemStatus;

    if (
      location.pathname !== "/setup" &&
      status === GQL.SystemStatusEnum.Setup
    ) {
      // redirect to setup page
      history.push("/setup");
    }

    if (
      location.pathname !== "/migrate" &&
      status === GQL.SystemStatusEnum.NeedsMigration
    ) {
      // redirect to migrate page
      history.replace("/migrate");
    }
  }, [systemStatusData, setupMatch, history, location]);

  function maybeRenderNavbar() {
    // don't render navbar for setup views
    if (!setupMatch) {
      return <MainNavbar />;
    }
  }

  function renderContent() {
    if (!systemStatusData) {
      return <LoadingIndicator />;
    }

    return (
      <ErrorBoundary>
        <Suspense fallback={<LoadingIndicator />}>
          <Switch>
            <Route exact path="/" component={HomepageLanding} />
            <Route path="/scenes" component={Scenes} />
            <Route path="/images" component={Images} />
            <Route path="/galleries" component={Galleries} />
            <Route path="/performers" component={Performers} />
            <Route path="/tags" component={Tags} />
            <Route path="/studios" component={Studios} />
            <Route path="/groups" component={Groups} />
            {isTrustedRouteEnabled("/shows", meData?.me.capabilities) && (
              <Route path="/shows/:id?" component={ShowsPage} />
            )}
            {isTrustedRouteEnabled("/cam-models", meData?.me.capabilities) && (
              <Route path="/cam-models/:id?" component={CamModelsPage} />
            )}
            <Route path="/stats" component={Stats} />
            <Route path="/settings" component={Settings} />
            <Route
              path="/sceneFilenameParser"
              component={SceneFilenameParser}
            />
            <Route
              path="/sceneDuplicateChecker"
              component={SceneDuplicateChecker}
            />
            <Route path="/setup" component={Setup} />
            <Route path="/migrate" component={Migrate} />
            <PluginRoutes />
            <Route component={PageNotFound} />
          </Switch>
        </Suspense>
      </ErrorBoundary>
    );
  }

  function maybeRenderReleaseNotes() {
    if (
      setupMatch ||
      !systemStatusData ||
      config.loading ||
      config.error ||
      meData?.me.role !== "ADMIN"
    ) {
      return;
    }

    const lastNoteSeen = config.data?.configuration?.ui?.lastNoteSeen;
    const notes = releaseNotes.filter((n) => {
      const seenThrough = Math.max(
        Number(lastNoteSeen) || 0,
        dismissedReleaseNotesThrough || 0
      );
      return n.date > seenThrough;
    });

    if (notes.length === 0) return;

    return (
      <ReleaseNotesDialog
        notes={notes}
        onClose={async () => {
          const seenThrough = notes[0].date;
          // Close immediately instead of waiting for the configuration cache
          // to be rewritten by the persistence mutation.
          setDismissedReleaseNotesThrough(seenThrough);
          await saveUISetting({
            variables: {
              key: "lastNoteSeen",
              value: seenThrough,
            },
          });
        }}
      />
    );
  }

  const title = config.data?.configuration?.ui?.title || "Stash";
  const titleProps = makeTitleProps(title);

  if (!messages) {
    return (
      <div className="d-flex vh-100 align-items-center justify-content-center">
        Loading Stash…
      </div>
    );
  }

  function renderSimple(content: React.ReactNode) {
    return (
      <IntlProvider
        locale={intlLanguage}
        messages={messages}
        formats={intlFormats}
      >
        <MainContainer>{content}</MainContainer>
      </IntlProvider>
    );
  }

  if (
    config.loading ||
    (!migrationShell && (!meData?.me || !config.data?.configuration))
  ) {
    return renderSimple(<LoadingIndicator />);
  }

  if (config.error) {
    return renderSimple(
      <ErrorMessage
        message={
          <FormattedMessage
            id="errors.loading_type"
            values={{ type: "configuration" }}
          />
        }
        error={config.error.message}
      />
    );
  }

  return (
    <ErrorBoundary>
      <IntlProvider
        locale={intlLanguage}
        messages={messages}
        formats={intlFormats}
      >
        <ToastProvider>
          <PluginsLoader
            enabled={meData?.me.role === "ADMIN"}
            disableCustomizations={
              config.data?.configuration?.interface?.disableCustomizations ??
              false
            }
          >
            <PersonalThemeLoader>
              <AppContainer>
                <ConfigurationProvider
                  configuration={config.data!.configuration}
                >
                  {maybeRenderReleaseNotes()}
                  <ConnectionMonitor />
                  {!setupMatch && <TroubleshootingModeOverlay />}
                  <Suspense fallback={<LoadingIndicator />}>
                    <LightboxProvider>
                      <ManualProvider>
                        <InteractiveProvider>
                          <Helmet {...titleProps} />
                          {maybeRenderNavbar()}
                          <MainContainer>{renderContent()}</MainContainer>
                        </InteractiveProvider>
                      </ManualProvider>
                    </LightboxProvider>
                  </Suspense>
                </ConfigurationProvider>
              </AppContainer>
            </PersonalThemeLoader>
          </PluginsLoader>
        </ToastProvider>
      </IntlProvider>
    </ErrorBoundary>
  );
};
