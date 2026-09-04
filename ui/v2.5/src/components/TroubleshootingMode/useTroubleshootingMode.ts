import { useState, useRef, useEffect } from "react";
import {
  useConfigureInterface,
  useConfigureGeneral,
  useConfiguration,
} from "src/core/StashService";
import * as GQL from "src/core/generated-graphql";
import { useConfigurationContext } from "src/hooks/Config";

const ORIGINAL_LOG_LEVEL_KEY = "troubleshootingMode_originalLogLevel";

export function useTroubleshootingMode() {
  const [isLoading, setIsLoading] = useState(false);
  const isMounted = useRef(true);

  const { configuration: shellConfiguration } = useConfigurationContext();
  const { data: meData, loading: meLoading } = GQL.useMeQuery({
    fetchPolicy: "no-cache",
  });
  const isAdmin = meData?.me.role === "ADMIN";
  const { data: config } = useConfiguration(meLoading || !isAdmin);
  const [configureInterface] = useConfigureInterface();
  const [configureGeneral] = useConfigureGeneral();

  const isActive =
    isAdmin &&
    (shellConfiguration.interface?.disableCustomizations ?? false);
  const currentLogLevel = config?.configuration?.general?.logLevel || "Info";

  useEffect(() => {
    return () => {
      isMounted.current = false;
    };
  }, []);

  async function enable() {
    setIsLoading(true);
    try {
      // Store original log level for restoration later
      localStorage.setItem(ORIGINAL_LOG_LEVEL_KEY, currentLogLevel);

      // Enable troubleshooting mode and set log level to Debug
      await Promise.all([
        configureInterface({
          variables: { input: { disableCustomizations: true } },
        }),
        configureGeneral({
          variables: { input: { logLevel: "Debug" } },
        }),
      ]);

      window.location.reload();
    } catch (e) {
      if (isMounted.current) {
        setIsLoading(false);
      }
      throw e;
    }
  }

  async function disable() {
    setIsLoading(true);
    try {
      // Restore original log level
      const originalLogLevel =
        localStorage.getItem(ORIGINAL_LOG_LEVEL_KEY) || "Info";

      // Disable troubleshooting mode and restore log level
      await Promise.all([
        configureInterface({
          variables: { input: { disableCustomizations: false } },
        }),
        configureGeneral({
          variables: { input: { logLevel: originalLogLevel } },
        }),
      ]);

      // Clean up localStorage
      localStorage.removeItem(ORIGINAL_LOG_LEVEL_KEY);

      window.location.reload();
    } catch (e) {
      if (isMounted.current) {
        setIsLoading(false);
      }
      throw e;
    }
  }

  return { isActive, isAdmin, isLoading, enable, disable };
}
