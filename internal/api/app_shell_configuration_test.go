package api

import (
	"reflect"
	"sort"
	"testing"
)

// Keep this list exact: adding a shell field is a security-sensitive change
// because every active account with library.read can query this object.
func TestAppShellConfigurationHasOnlyApprovedFields(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{"root", reflect.TypeOf(AppShellConfiguration{}), []string{"Interface", "Status", "UI"}},
		{"interface", reflect.TypeOf(AppShellInterface{}), []string{"DisableCustomizations", "FunscriptOffset", "ImageLightbox", "Language", "SfwContentMode", "UseStashHostedFunscript"}},
		{"ui", reflect.TypeOf(AppShellUI{}), []string{
			"AbbreviateCounters", "AlwaysStartFromBeginning", "CompactExpandedDetails",
			"DisableMobileMediaAutoRotateEnabled", "Editing", "EnableChromecast",
			"EnableMovieBackgroundImage", "EnablePerformerBackgroundImage",
			"EnableStudioBackgroundImage", "EnableTagBackgroundImage", "FrontPageContent",
			"ImageWallOptions", "LastNoteSeen", "MinimumPlayPercent", "ShowAbLoopControls",
			"ShowAllDetails", "ShowChildStudioContent", "ShowChildTagContent",
			"ShowLinksOnPerformerCard", "ShowRangeMarkers", "TableColumns", "TaskDefaults",
			"Title", "TrackActivity", "TroubleshootingMode", "VrTag",
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := make([]string, 0, test.typ.NumField())
			for i := 0; i < test.typ.NumField(); i++ {
				got = append(got, test.typ.Field(i).Name)
			}
			sort.Strings(got)
			sort.Strings(test.want)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("shell fields = %v; want %v", got, test.want)
			}
		})
	}
}
