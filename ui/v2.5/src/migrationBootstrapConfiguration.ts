import * as GQL from "./core/generated-graphql";

// Fixed client-only configuration for the purpose-bound migration shell.
// Values here are inert UI defaults, not observations about server state.
export const migrationBootstrapConfiguration = Object.freeze({
  general: {
    apiKey: "",
    stashes: [],
  },
  defaults: {},
  interface: {
    disableCustomizations: true,
    handyKey: null,
    funscriptOffset: 0,
    imageLightbox: {
      scrollAttemptsBeforeChange: 0,
    },
    language: "en-GB",
    sfwContentMode: false,
    useStashHostedFunscript: false,
    disableDropdownCreate: {
      performer: true,
      tag: true,
      studio: true,
      movie: true,
      gallery: true,
    },
  },
  plugins: {},
  ui: {},
}) as unknown as GQL.ConfigDataFragment;
