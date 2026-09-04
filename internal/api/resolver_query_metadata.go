package api

import (
	"context"
	"fmt"
	"runtime"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
)

func shellUIValue(ui map[string]interface{}, key string) interface{} {
	if ui == nil {
		return nil
	}
	return ui[key]
}

func (r *queryResolver) AppShellConfiguration(ctx context.Context) (*AppShellConfiguration, error) {
	c := config.GetInstance()
	fullInterface := makeConfigInterfaceResult()
	ui := c.GetUIConfiguration()
	return &AppShellConfiguration{
		Status: manager.GetInstance().GetSystemStatus().Status,
		Interface: &AppShellInterface{
			SfwContentMode: c.GetSFWContentMode(), DisableCustomizations: c.GetDisableCustomizations(), Language: c.GetLanguage(),
			ImageLightbox: fullInterface.ImageLightbox, FunscriptOffset: fullInterface.FunscriptOffset,
			UseStashHostedFunscript: fullInterface.UseStashHostedFunscript,
		},
		UI: &AppShellUI{
			Title: shellUIValue(ui, "title"), LastNoteSeen: shellUIValue(ui, "lastNoteSeen"),
			AbbreviateCounters: shellUIValue(ui, "abbreviateCounters"), CompactExpandedDetails: shellUIValue(ui, "compactExpandedDetails"),
			ShowAllDetails: shellUIValue(ui, "showAllDetails"), EnableMovieBackgroundImage: shellUIValue(ui, "enableMovieBackgroundImage"),
			EnablePerformerBackgroundImage: shellUIValue(ui, "enablePerformerBackgroundImage"), EnableStudioBackgroundImage: shellUIValue(ui, "enableStudioBackgroundImage"),
			EnableTagBackgroundImage: shellUIValue(ui, "enableTagBackgroundImage"), ShowChildStudioContent: shellUIValue(ui, "showChildStudioContent"),
			ShowChildTagContent: shellUIValue(ui, "showChildTagContent"), ShowLinksOnPerformerCard: shellUIValue(ui, "showLinksOnPerformerCard"),
			ImageWallOptions: shellUIValue(ui, "imageWallOptions"), AlwaysStartFromBeginning: shellUIValue(ui, "alwaysStartFromBeginning"),
			DisableMobileMediaAutoRotateEnabled: shellUIValue(ui, "disableMobileMediaAutoRotateEnabled"), EnableChromecast: shellUIValue(ui, "enableChromecast"),
			MinimumPlayPercent: shellUIValue(ui, "minimumPlayPercent"), ShowAbLoopControls: shellUIValue(ui, "showAbLoopControls"),
			ShowRangeMarkers: shellUIValue(ui, "showRangeMarkers"), TrackActivity: shellUIValue(ui, "trackActivity"), VrTag: shellUIValue(ui, "vrTag"),
			TaskDefaults: shellUIValue(ui, "taskDefaults"), TableColumns: shellUIValue(ui, "tableColumns"), FrontPageContent: shellUIValue(ui, "frontPageContent"),
			Editing: shellUIValue(ui, "editing"), TroubleshootingMode: shellUIValue(ui, "troubleshootingMode"),
		},
	}, nil
}

func (r *queryResolver) BootstrapConfiguration(ctx context.Context) (*BootstrapConfiguration, error) {
	return &BootstrapConfiguration{Status: manager.GetInstance().GetSystemStatus().Status, Os: runtime.GOOS}, nil
}

func (r *queryResolver) SystemStatus(ctx context.Context) (*manager.SystemStatus, error) {
	return manager.GetInstance().GetSystemStatus(), nil
}

func (r *queryResolver) MigrationStatus(ctx context.Context) (*MigrationStatus, error) {
	mgr := manager.GetInstance()
	if !migrationWindowOpen(mgr.Database) {
		return nil, fmt.Errorf("database migration is not required")
	}
	return &MigrationStatus{
		Status:              manager.SystemStatusEnumNeedsMigration,
		CurrentSchema:       int(mgr.Database.Version()),
		RequiredSchema:      int(mgr.Database.AppSchemaVersion()),
		BackupWillBeCreated: true,
	}, nil
}
