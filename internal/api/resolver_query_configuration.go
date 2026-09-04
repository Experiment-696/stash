package api

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/internal/authz"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/fsutil"
	"github.com/stashapp/stash/pkg/models"
	"golang.org/x/text/collate"
)

func (r *queryResolver) Configuration(ctx context.Context) (*ConfigResult, error) {
	result := makeConfigResult()
	principal, err := authz.PrincipalFromContext(ctx)
	if err == nil && principal.Role == authz.RoleAdmin {
		return result, nil
	}

	return makeRoleSafeConfigResult(result), nil
}

// makeRoleSafeConfigResult preserves the complete ConfigResult shape required
// by authenticated library UI consumers while removing administrator-only
// credentials, filesystem paths, executable customization, and plugin data.
func makeRoleSafeConfigResult(full *ConfigResult) *ConfigResult {
	safe := *full
	general := *full.General
	general.Stashes = make([]*config.StashConfig, len(full.General.Stashes))
	for i, stash := range full.General.Stashes {
		if stash == nil {
			continue
		}
		redacted := *stash
		redacted.Path = ""
		general.Stashes[i] = &redacted
	}
	general.DatabasePath = ""
	general.BackupDirectoryPath = ""
	general.DeleteTrashPath = ""
	general.GeneratedPath = ""
	general.MetadataPath = ""
	general.ConfigFilePath = ""
	general.ScrapersPath = ""
	general.PluginsPath = ""
	general.CachePath = ""
	general.BlobsPath = ""
	general.FfmpegPath = ""
	general.FfprobePath = ""
	general.APIKey = ""
	general.Username = ""
	general.Password = ""
	general.LogFile = nil
	general.CustomPerformerImageLocation = nil
	general.PythonPath = ""
	general.TranscodeInputArgs = []string{}
	general.TranscodeOutputArgs = []string{}
	general.LiveTranscodeInputArgs = []string{}
	general.LiveTranscodeOutputArgs = []string{}
	general.Excludes = []string{}
	general.ImageExcludes = []string{}
	general.ScraperPackageSources = []*models.PackageSource{}
	general.PluginPackageSources = []*models.PackageSource{}
	general.StashBoxes = append([]*models.StashBox(nil), full.General.StashBoxes...)
	for i, box := range general.StashBoxes {
		if box == nil {
			continue
		}
		redacted := *box
		redacted.APIKey = ""
		general.StashBoxes[i] = &redacted
	}
	safe.General = &general

	iface := *full.Interface
	iface.HandyKey = nil
	iface.CSS = nil
	iface.CSSEnabled = nil
	iface.Javascript = nil
	iface.JavascriptEnabled = nil
	iface.CustomLocales = nil
	iface.CustomLocalesEnabled = nil
	safe.Interface = &iface

	dlna := *full.Dlna
	dlna.WhitelistedIPs = []string{}
	dlna.Interfaces = []string{}
	safe.Dlna = &dlna

	scraping := *full.Scraping
	scraping.ScraperCDPPath = nil
	safe.Scraping = &scraping

	defaults := *full.Defaults
	if full.Defaults.Identify != nil {
		identifyDefaults := *full.Defaults.Identify
		identifyDefaults.SceneIDs = []string{}
		identifyDefaults.Paths = []string{}
		defaults.Identify = &identifyDefaults
	}
	safe.Defaults = &defaults

	safe.UI = roleSafeUIConfiguration(full.UI)
	safe.Plugins = map[string]map[string]interface{}{}
	return &safe
}

func roleSafeUIConfiguration(full map[string]interface{}) map[string]interface{} {
	safe := make(map[string]interface{})
	for _, key := range []string{
		"title", "lastNoteSeen", "maxOptionsShown", "ratingSystemOptions",
		"previewVolume", "abbreviateCounters", "compactExpandedDetails",
		"showAllDetails", "enableMovieBackgroundImage",
		"enablePerformerBackgroundImage", "enableStudioBackgroundImage",
		"enableTagBackgroundImage", "showChildStudioContent",
		"showChildTagContent", "showLinksOnPerformerCard",
		"showPerformerCardOnHover", "showTagCardOnHover",
		"imageWallOptions", "alwaysStartFromBeginning",
		"disableMobileMediaAutoRotateEnabled", "enableChromecast",
		"minimumPlayPercent", "showAbLoopControls", "showRangeMarkers",
		"trackActivity", "vrTag", "taskDefaults", "tableColumns",
		"frontPageContent", "editing", "troubleshootingMode",
		"taggerConfig", "pinnedFilters",
	} {
		if value, found := full[key]; found {
			safe[key] = value
		}
	}
	return safe
}

func (r *queryResolver) Directory(ctx context.Context, path, locale *string) (*Directory, error) {

	directory := &Directory{}
	var err error

	col := newCollator(locale, collate.IgnoreCase, collate.Numeric)

	var dirPath = ""
	if path != nil {
		dirPath = *path
	}
	currentDir := getDir(dirPath)
	directories, err := listDir(col, currentDir)
	if err != nil {
		return directory, err
	}

	directory.Path = currentDir
	directory.Parent = getParent(currentDir)
	directory.Directories = directories

	return directory, err
}

func getDir(path string) string {
	if path == "" {
		path = fsutil.GetHomeDirectory()
	}

	return path
}

func getParent(path string) *string {
	isRoot := path == "/"
	if isRoot {
		return nil
	} else {
		parentPath := filepath.Clean(path + "/..")
		return &parentPath
	}
}

func makeConfigResult() *ConfigResult {
	return &ConfigResult{
		General:   makeConfigGeneralResult(),
		Interface: makeConfigInterfaceResult(),
		Dlna:      makeConfigDLNAResult(),
		Scraping:  makeConfigScrapingResult(),
		Defaults:  makeConfigDefaultsResult(),
		UI:        makeConfigUIResult(),
	}
}

func makeConfigGeneralResult() *ConfigGeneralResult {
	config := config.GetInstance()
	logFile := config.GetLogFile()

	maxTranscodeSize := config.GetMaxTranscodeSize()
	maxStreamingTranscodeSize := config.GetMaxStreamingTranscodeSize()

	customPerformerImageLocation := config.GetCustomPerformerImageLocation()

	return &ConfigGeneralResult{
		Stashes:                       config.GetStashPaths(),
		DatabasePath:                  config.GetDatabasePath(),
		BackupDirectoryPath:           config.GetBackupDirectoryPath(),
		DeleteTrashPath:               config.GetDeleteTrashPath(),
		GeneratedPath:                 config.GetGeneratedPath(),
		MetadataPath:                  config.GetMetadataPath(),
		ConfigFilePath:                config.GetConfigFile(),
		ScrapersPath:                  config.GetScrapersPath(),
		PluginsPath:                   config.GetPluginsPath(),
		CachePath:                     config.GetCachePath(),
		BlobsPath:                     config.GetBlobsPath(),
		BlobsStorage:                  config.GetBlobsStorage(),
		FfmpegPath:                    config.GetFFMpegPath(),
		FfprobePath:                   config.GetFFProbePath(),
		CalculateMd5:                  config.IsCalculateMD5(),
		VideoFileNamingAlgorithm:      config.GetVideoFileNamingAlgorithm(),
		ParallelTasks:                 config.GetParallelTasks(),
		UseCustomSpriteInterval:       config.GetUseCustomSpriteInterval(),
		SpriteInterval:                config.GetSpriteInterval(),
		SpriteScreenshotSize:          config.GetSpriteScreenshotSize(),
		MinimumSprites:                config.GetMinimumSprites(),
		MaximumSprites:                config.GetMaximumSprites(),
		PreviewAudio:                  config.GetPreviewAudio(),
		PreviewSegments:               config.GetPreviewSegments(),
		PreviewSegmentDuration:        config.GetPreviewSegmentDuration(),
		PreviewExcludeStart:           config.GetPreviewExcludeStart(),
		PreviewExcludeEnd:             config.GetPreviewExcludeEnd(),
		PreviewPreset:                 config.GetPreviewPreset(),
		TranscodeHardwareAcceleration: config.GetTranscodeHardwareAcceleration(),
		MaxTranscodeSize:              &maxTranscodeSize,
		MaxStreamingTranscodeSize:     &maxStreamingTranscodeSize,
		WriteImageThumbnails:          config.IsWriteImageThumbnails(),
		CreateImageClipsFromVideos:    config.IsCreateImageClipsFromVideos(),
		GalleryCoverRegex:             config.GetGalleryCoverRegex(),
		APIKey:                        config.GetAPIKey(),
		Username:                      config.GetUsername(),
		Password:                      config.GetPasswordHash(),
		MaxSessionAge:                 config.GetMaxSessionAge(),
		LogFile:                       &logFile,
		LogOut:                        config.GetLogOut(),
		LogLevel:                      config.GetLogLevel(),
		LogAccess:                     config.GetLogAccess(),
		LogFileMaxSize:                config.GetLogFileMaxSize(),
		VideoExtensions:               config.GetVideoExtensions(),
		ImageExtensions:               config.GetImageExtensions(),
		GalleryExtensions:             config.GetGalleryExtensions(),
		CreateGalleriesFromFolders:    config.GetCreateGalleriesFromFolders(),
		Excludes:                      config.GetExcludes(),
		ImageExcludes:                 config.GetImageExcludes(),
		CustomPerformerImageLocation:  &customPerformerImageLocation,
		StashBoxes:                    config.GetStashBoxes(),
		PythonPath:                    config.GetPythonPath(),
		TranscodeInputArgs:            config.GetTranscodeInputArgs(),
		TranscodeOutputArgs:           config.GetTranscodeOutputArgs(),
		LiveTranscodeInputArgs:        config.GetLiveTranscodeInputArgs(),
		LiveTranscodeOutputArgs:       config.GetLiveTranscodeOutputArgs(),
		DrawFunscriptHeatmapRange:     config.GetDrawFunscriptHeatmapRange(),
		ScraperPackageSources:         config.GetScraperPackageSources(),
		PluginPackageSources:          config.GetPluginPackageSources(),
	}
}

func makeConfigInterfaceResult() *ConfigInterfaceResult {
	config := config.GetInstance()
	menuItems := config.GetMenuItems()
	soundOnPreview := config.GetSoundOnPreview()
	wallShowTitle := config.GetWallShowTitle()
	showScrubber := config.GetShowScrubber()
	wallPlayback := config.GetWallPlayback()
	noBrowser := config.GetNoBrowser()
	notificationsEnabled := config.GetNotificationsEnabled()
	maximumLoopDuration := config.GetMaximumLoopDuration()
	autostartVideo := config.GetAutostartVideo()
	autostartVideoOnPlaySelected := config.GetAutostartVideoOnPlaySelected()
	continuePlaylistDefault := config.GetContinuePlaylistDefault()
	showStudioAsText := config.GetShowStudioAsText()
	css := config.GetCSS()
	cssEnabled := config.GetCSSEnabled()
	javascript := config.GetJavascript()
	javascriptEnabled := config.GetJavascriptEnabled()
	customLocales := config.GetCustomLocales()
	customLocalesEnabled := config.GetCustomLocalesEnabled()
	disableCustomizations := config.GetDisableCustomizations()
	language := config.GetLanguage()
	handyKey := config.GetHandyKey()
	scriptOffset := config.GetFunscriptOffset()
	useStashHostedFunscript := config.GetUseStashHostedFunscript()
	imageLightboxOptions := config.GetImageLightboxOptions()
	disableDropdownCreate := config.GetDisableDropdownCreate()

	return &ConfigInterfaceResult{
		SfwContentMode:               config.GetSFWContentMode(),
		MenuItems:                    menuItems,
		SoundOnPreview:               &soundOnPreview,
		WallShowTitle:                &wallShowTitle,
		WallPlayback:                 &wallPlayback,
		ShowScrubber:                 &showScrubber,
		MaximumLoopDuration:          &maximumLoopDuration,
		NoBrowser:                    &noBrowser,
		NotificationsEnabled:         &notificationsEnabled,
		AutostartVideo:               &autostartVideo,
		ShowStudioAsText:             &showStudioAsText,
		AutostartVideoOnPlaySelected: &autostartVideoOnPlaySelected,
		ContinuePlaylistDefault:      &continuePlaylistDefault,
		CSS:                          &css,
		CSSEnabled:                   &cssEnabled,
		Javascript:                   &javascript,
		JavascriptEnabled:            &javascriptEnabled,
		CustomLocales:                &customLocales,
		CustomLocalesEnabled:         &customLocalesEnabled,
		DisableCustomizations:        &disableCustomizations,
		Language:                     &language,

		ImageLightbox: &imageLightboxOptions,

		DisableDropdownCreate: disableDropdownCreate,

		HandyKey:                &handyKey,
		FunscriptOffset:         &scriptOffset,
		UseStashHostedFunscript: &useStashHostedFunscript,
	}
}

func makeConfigDLNAResult() *ConfigDLNAResult {
	config := config.GetInstance()

	return &ConfigDLNAResult{
		ServerName:     config.GetDLNAServerName(),
		Enabled:        config.GetDLNADefaultEnabled(),
		Port:           config.GetDLNAPort(),
		WhitelistedIPs: config.GetDLNADefaultIPWhitelist(),
		Interfaces:     config.GetDLNAInterfaces(),
		VideoSortOrder: config.GetVideoSortOrder(),
	}
}

func makeConfigScrapingResult() *ConfigScrapingResult {
	config := config.GetInstance()

	scraperUserAgent := config.GetScraperUserAgent()
	scraperCDPPath := config.GetScraperCDPPath()

	return &ConfigScrapingResult{
		ScraperUserAgent:   &scraperUserAgent,
		ScraperCertCheck:   config.GetScraperCertCheck(),
		ScraperCDPPath:     &scraperCDPPath,
		ExcludeTagPatterns: config.GetScraperExcludeTagPatterns(),
	}
}

func makeConfigDefaultsResult() *ConfigDefaultSettingsResult {
	config := config.GetInstance()
	deleteFileDefault := config.GetDeleteFileDefault()
	deleteGeneratedDefault := config.GetDeleteGeneratedDefault()

	return &ConfigDefaultSettingsResult{
		Identify:        config.GetDefaultIdentifySettings(),
		Scan:            config.GetDefaultScanSettings(),
		AutoTag:         config.GetDefaultAutoTagSettings(),
		Generate:        config.GetDefaultGenerateSettings(),
		DeleteFile:      &deleteFileDefault,
		DeleteGenerated: &deleteGeneratedDefault,
	}
}

func makeConfigUIResult() map[string]interface{} {
	return config.GetInstance().GetUIConfiguration()
}

func (r *queryResolver) ValidateStashBoxCredentials(ctx context.Context, input config.StashBoxInput) (*StashBoxValidationResult, error) {
	box := models.StashBox{Endpoint: input.Endpoint, APIKey: input.APIKey}
	client := r.newStashBoxClient(box)

	user, err := client.GetUser(ctx)

	valid := user != nil && user.Me != nil
	var status string
	if valid {
		status = fmt.Sprintf("Successfully authenticated as %s", user.Me.Name)
	} else {
		errorStr := strings.ToLower(err.Error())
		switch {
		case strings.Contains(errorStr, "doctype"):
			// Index file returned rather than graphql
			status = "Invalid endpoint"
		case strings.Contains(errorStr, "request failed"):
			status = "No response from server"
		case strings.Contains(errorStr, "invalid character") ||
			strings.Contains(errorStr, "illegal base64 data") ||
			strings.Contains(errorStr, "unexpected end of json input") ||
			strings.Contains(errorStr, "token contains an invalid number of segments"):
			status = "Malformed API key."
		case strings.Contains(errorStr, "signature is invalid"):
			status = "Invalid or expired API key."
		default:
			status = fmt.Sprintf("Unknown error: %s", err)
		}
	}

	result := StashBoxValidationResult{
		Valid:  valid,
		Status: status,
	}

	return &result, nil
}
