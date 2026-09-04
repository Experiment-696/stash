package api

import (
	"reflect"
	"sort"
	"testing"

	"github.com/stashapp/stash/internal/identify"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func assertExactFieldContract(t *testing.T, value interface{}, preserved, redacted []string) {
	t.Helper()
	typ := reflect.TypeOf(value)
	classified := append(append([]string{}, preserved...), redacted...)
	sort.Strings(classified)
	actual := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		actual = append(actual, typ.Field(i).Name)
	}
	sort.Strings(actual)
	if !reflect.DeepEqual(actual, classified) {
		t.Fatalf(
			"runtime-safe configuration contract drift for %s\nactual: %v\nclassified: %v",
			typ.Name(),
			actual,
			classified,
		)
	}
}

func TestRoleSafeConfigurationEveryGeneratedFieldIsClassified(t *testing.T) {
	assertExactFieldContract(t, ConfigResult{}, []string{
		"Defaults", "Dlna", "General", "Interface", "Scraping", "UI",
	}, []string{"Plugins"})
	assertExactFieldContract(t, ConfigGeneralResult{}, []string{
		"BlobsStorage", "CalculateMd5", "CreateGalleriesFromFolders",
		"CreateImageClipsFromVideos", "DrawFunscriptHeatmapRange",
		"GalleryCoverRegex", "GalleryExtensions", "ImageExtensions",
		"LogAccess", "LogFileMaxSize", "LogLevel", "LogOut",
		"MaxSessionAge", "MaximumSprites", "MaxStreamingTranscodeSize",
		"MaxTranscodeSize", "MinimumSprites", "ParallelTasks",
		"PreviewAudio", "PreviewExcludeEnd", "PreviewExcludeStart",
		"PreviewPreset", "PreviewSegmentDuration", "PreviewSegments",
		"SpriteInterval", "SpriteScreenshotSize", "StashBoxes",
		"TranscodeHardwareAcceleration", "UseCustomSpriteInterval",
		"VideoExtensions", "VideoFileNamingAlgorithm", "WriteImageThumbnails",
	}, []string{
		"APIKey", "BackupDirectoryPath", "BlobsPath", "CachePath",
		"ConfigFilePath", "CustomPerformerImageLocation", "DatabasePath",
		"DeleteTrashPath", "Excludes", "FfmpegPath", "FfprobePath",
		"GeneratedPath", "ImageExcludes", "LiveTranscodeInputArgs",
		"LiveTranscodeOutputArgs", "LogFile", "MetadataPath", "Password",
		"PluginPackageSources", "PluginsPath", "PythonPath",
		"ScraperPackageSources", "ScrapersPath", "Stashes",
		"TranscodeInputArgs", "TranscodeOutputArgs", "Username",
	})
	assertExactFieldContract(t, ConfigInterfaceResult{}, []string{
		"AutostartVideo", "AutostartVideoOnPlaySelected",
		"ContinuePlaylistDefault", "DisableCustomizations",
		"DisableDropdownCreate", "FunscriptOffset", "ImageLightbox",
		"Language", "MaximumLoopDuration", "MenuItems", "NoBrowser",
		"NotificationsEnabled", "SfwContentMode", "ShowScrubber",
		"ShowStudioAsText", "SoundOnPreview", "UseStashHostedFunscript",
		"WallPlayback", "WallShowTitle",
	}, []string{
		"CSS", "CSSEnabled", "CustomLocales", "CustomLocalesEnabled",
		"HandyKey", "Javascript", "JavascriptEnabled",
	})
	assertExactFieldContract(t, ConfigDLNAResult{}, []string{
		"Enabled", "Port", "ServerName", "VideoSortOrder",
	}, []string{"Interfaces", "WhitelistedIPs"})
	assertExactFieldContract(t, ConfigScrapingResult{}, []string{
		"ExcludeTagPatterns", "ScraperCertCheck", "ScraperUserAgent",
	}, []string{"ScraperCDPPath"})
}

func TestRoleSafeConfigurationPreservesRuntimeShapeAndRedactsSecrets(t *testing.T) {
	config.InitializeEmpty()

	logPath := "/secret/stash.log"
	full := makeConfigResult()
	full.General.Stashes = []*config.StashConfig{{
		Path:         "/secret/library",
		ExcludeVideo: true,
	}}
	full.General.StashBoxes = []*models.StashBox{{
		Name:     "StashDB",
		Endpoint: "https://stashdb.org/graphql",
		APIKey:   "secret",
	}}
	full.Interface.MenuItems = []string{"scenes", "performers"}
	full.Interface.ImageLightbox = &config.ConfigImageLightboxResult{
		ScrollAttemptsBeforeChange: 3,
	}
	full.Interface.DisableDropdownCreate = &config.ConfigDisableDropdownCreate{
		Performer: true,
	}
	full.Defaults.Identify = &identify.Options{
		Sources:  []*identify.Source{},
		SceneIDs: []string{"42"},
		Paths:    []string{"/secret/library"},
	}
	full.General.APIKey = "legacy-secret"
	full.General.Password = "password-hash"
	full.General.DatabasePath = "/secret/stash.db"
	full.General.LogFile = &logPath
	full.Scraping.ScraperCDPPath = &logPath
	full.Dlna.WhitelistedIPs = []string{"192.0.2.1"}
	full.UI["title"] = "Visible title"
	full.UI["unreviewedSecret"] = "secret"
	handyKey := "handy-secret"
	full.Interface.HandyKey = &handyKey
	full.Plugins = map[string]map[string]interface{}{
		"secret-plugin": {"token": "secret"},
	}

	safe := makeRoleSafeConfigResult(full)
	if safe.General == nil || safe.Interface == nil || safe.Defaults == nil ||
		safe.Dlna == nil || safe.Scraping == nil || safe.UI == nil ||
		safe.Plugins == nil {
		t.Fatal("role-safe configuration omitted a required runtime container")
	}
	if safe.General.Stashes == nil || safe.General.StashBoxes == nil {
		t.Fatal("role-safe configuration omitted a required runtime array")
	}
	if len(safe.General.Stashes) != 1 ||
		safe.General.Stashes[0].Path != "" ||
		!safe.General.Stashes[0].ExcludeVideo {
		t.Fatal("role-safe configuration did not preserve stash shape while redacting its path")
	}
	if len(safe.Interface.MenuItems) != 2 ||
		safe.Interface.ImageLightbox == nil ||
		safe.Interface.ImageLightbox.ScrollAttemptsBeforeChange != 3 ||
		safe.Interface.DisableDropdownCreate == nil ||
		!safe.Interface.DisableDropdownCreate.Performer ||
		safe.Defaults.Identify == nil ||
		safe.Defaults.Identify.Sources == nil ||
		safe.Defaults.Identify.SceneIDs == nil ||
		safe.Defaults.Identify.Paths == nil {
		t.Fatal("role-safe configuration lost nonempty nested runtime data")
	}
	if len(safe.General.StashBoxes) != 1 ||
		safe.General.StashBoxes[0].Name != "StashDB" ||
		safe.General.StashBoxes[0].Endpoint != "https://stashdb.org/graphql" {
		t.Fatal("role-safe configuration lost non-secret stash-box identity")
	}
	if safe.General.StashBoxes[0].APIKey != "" ||
		safe.General.APIKey != "" ||
		safe.General.Password != "" ||
		safe.General.DatabasePath != "" ||
		safe.General.LogFile != nil ||
		safe.Interface.HandyKey != nil ||
		safe.Scraping.ScraperCDPPath != nil ||
		len(safe.Defaults.Identify.SceneIDs) != 0 ||
		len(safe.Defaults.Identify.Paths) != 0 ||
		len(safe.Dlna.WhitelistedIPs) != 0 ||
		safe.UI["title"] != "Visible title" ||
		safe.UI["unreviewedSecret"] != nil ||
		len(safe.Plugins) != 0 {
		t.Fatal("role-safe configuration exposed an administrator-only value")
	}
	if full.General.StashBoxes[0].APIKey != "secret" {
		t.Fatal("role-safe projection mutated the full Admin configuration")
	}
	if full.General.Stashes[0].Path != "/secret/library" {
		t.Fatal("role-safe projection mutated the full Admin stash path")
	}
	if len(full.Defaults.Identify.Paths) != 1 ||
		full.Defaults.Identify.Paths[0] != "/secret/library" {
		t.Fatal("role-safe projection mutated full Admin identify defaults")
	}
}
