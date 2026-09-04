#!/usr/bin/env python3
"""Generate the reviewed GraphQL authorization policy from explicit classifications."""
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCHEMA = ROOT / "graphql/schema/schema.graphql"
OUTPUT = ROOT / "internal/authz/graphql_policy.json"

GROUPS = {
	("GRAPHQL_QUERY", "library.read", False): "findFile findFiles findFolder findFolders findScene findSceneByHash findScenes findScenesByPathRegex findDuplicateScenes sceneStreams parseSceneFilenames findSceneMarkers findImage findImages findPerformer findPerformers findStudio findStudios findMovie findMovies findGroup findGroups findGallery findGalleries findTag findTags markerWall sceneWall markerStrings stats sceneMarkerTags allScenes allSceneMarkers allImages allGalleries allPerformers allTags allStudios allMovies version latestversion appShellConfiguration configuration",
	("GRAPHQL_QUERY", "account.self.read", True): "findSavedFilter findSavedFilters findDefaultFilter myAPITokens myPreferences",
	("GRAPHQL_QUERY", "account.self.read", False): "me availableThemes",
    ("GRAPHQL_QUERY", "audit.read", False): "auditEvents",
    ("GRAPHQL_QUERY", "scraper.run", False): "listScrapers scrapeSingleScene scrapeMultiScenes scrapeSingleStudio scrapeSingleTag scrapeSinglePerformer scrapeMultiPerformers scrapeSingleGallery scrapeSingleMovie scrapeSingleGroup scrapeSingleImage scrapeURL scrapePerformerURL scrapeSceneURL scrapeGalleryURL scrapeImageURL scrapeMovieURL scrapeGroupURL validateStashBoxCredentials",
    ("GRAPHQL_QUERY", "extension.read", False): "plugins pluginTasks installedPackages availablePackages",
    ("GRAPHQL_QUERY", "system.status.read", False): "directory systemStatus dlnaStatus logs",
    ("GRAPHQL_QUERY", "job.read", False): "jobQueue findJob",
    ("GRAPHQL_QUERY", "account.manage", False): "users",
    ("GRAPHQL_QUERY", "public.bootstrap", False): "bootstrapConfiguration",
    ("GRAPHQL_MUTATION", "public.bootstrap", False): "setup bootstrapFirstAdmin bootstrapConfigureUI",
    ("GRAPHQL_MUTATION", "metadata.write", False): "sceneCreate sceneUpdate bulkSceneUpdate scenesUpdate sceneGenerateScreenshot sceneMarkerCreate sceneMarkerUpdate bulkSceneMarkerUpdate imageUpdate bulkImageUpdate imagesUpdate galleryCreate galleryUpdate bulkGalleryUpdate galleriesUpdate addGalleryImages removeGalleryImages setGalleryCover resetGalleryCover galleryChapterCreate galleryChapterUpdate performerCreate performerUpdate bulkPerformerUpdate studioCreate studioUpdate bulkStudioUpdate movieCreate movieUpdate bulkMovieUpdate groupCreate groupUpdate bulkGroupUpdate addGroupSubGroups removeGroupSubGroups reorderSubGroups tagCreate tagUpdate bulkTagUpdate",
    ("GRAPHQL_MUTATION", "activity.self.write", True): "sceneIncrementO sceneDecrementO sceneAddO sceneDeleteO sceneResetO sceneSaveActivity sceneResetActivity sceneIncrementPlayCount sceneAddPlay sceneDeletePlay sceneResetPlayCount imageIncrementO imageDecrementO imageResetO",
    ("GRAPHQL_MUTATION", "preference.self.write", True): "saveFilter destroySavedFilter setDefaultFilter setMyHomepageRoute setMyTheme performerSetFavorite performerSetRating sceneSetRating",
    ("GRAPHQL_MUTATION", "account.self.write", True): "createMyAPIToken revokeMyAPIToken",
    ("GRAPHQL_MUTATION", "library.destructive", False): "sceneMerge sceneDestroy scenesDestroy sceneMarkerDestroy sceneMarkersDestroy imageDestroy imagesDestroy galleryDestroy galleryChapterDestroy performerDestroy performersDestroy performerMerge studioDestroy studiosDestroy movieDestroy moviesDestroy groupDestroy groupsDestroy tagDestroy tagsDestroy tagsMerge",
    ("GRAPHQL_MUTATION", "filesystem.write", False): "sceneAssignFile moveFiles deleteFiles destroyFiles revealFileInFileManager revealFolderInFileManager",
    ("GRAPHQL_MUTATION", "hash.manage", False): "fileSetFingerprints",
    ("GRAPHQL_MUTATION", "system.configure", False): "configureGeneral configureInterface configureDLNA configureScraping configureDefaults configurePlugin configureUI configureUISetting generateAPIKey migrate downloadFFMpeg enableDLNA disableDLNA addTempDLNAIP removeTempDLNAIP",
    ("GRAPHQL_MUTATION", "data.admin", False): "exportObjects importObjects metadataImport metadataExport migrateHashNaming migrateSceneScreenshots migrateBlobs anonymiseDatabase optimiseDatabase backupDatabase",
    ("GRAPHQL_MUTATION", "automation.run", False): "metadataScan metadataGenerate metadataAutoTag metadataClean metadataCleanGenerated metadataIdentify submitStashBoxFingerprints submitStashBoxSceneDraft submitStashBoxPerformerDraft stashBoxBatchPerformerTag stashBoxBatchStudioTag stashBoxBatchTagTag",
    ("GRAPHQL_MUTATION", "extension.manage", False): "reloadScrapers setPluginsEnabled runPluginTask runPluginOperation reloadPlugins installPackages updatePackages uninstallPackages",
    ("GRAPHQL_MUTATION", "job.manage", False): "stopJob stopAllJobs",
    ("GRAPHQL_MUTATION", "database.sql", False): "querySQL execSQL",
    ("GRAPHQL_MUTATION", "account.manage", False): "createUser updateUserAccess resetUserPassword revokeUserSessions revokeUserAPITokens",
    ("GRAPHQL_SUBSCRIPTION", "job.read", False): "jobsSubscribe",
    ("GRAPHQL_SUBSCRIPTION", "system.status.read", False): "loggingSubscribe",
    ("GRAPHQL_SUBSCRIPTION", "library.read", False): "scanCompleteSubscribe",
}


def schema_roots():
    text = SCHEMA.read_text(encoding="utf-8")
    found = {}
    kinds = {"Query": "GRAPHQL_QUERY", "Mutation": "GRAPHQL_MUTATION", "Subscription": "GRAPHQL_SUBSCRIPTION"}
    for root, kind in kinds.items():
        body = re.search(rf"type {root}\s*\{{(.*?)\n\}}", text, re.S)
        if not body:
            raise SystemExit(f"missing schema root: {root}")
        for name in re.findall(r"^\s{2}([A-Za-z_][A-Za-z0-9_]*)\s*(?:\(|:)", body.group(1), re.M):
            found[(kind, name)] = True
    return found


def main():
    schema = schema_roots()
    classified = {}
    for (kind, capability, owner_scoped), names in GROUPS.items():
        for name in names.split():
            key = (kind, name)
            if key in classified:
                raise SystemExit(f"duplicate classification: {kind}:{name}")
            classified[key] = {"kind": kind, "name": name, "capability": capability, "owner_scoped": owner_scoped}
    missing = sorted(set(schema) - set(classified))
    unknown = sorted(set(classified) - set(schema))
    placeholders = [x for x in classified.values() if not x["capability"] or "placeholder" in x["capability"].lower() or "todo" in x["capability"].lower()]
    if missing or unknown or placeholders or len(classified) != 233:
        raise SystemExit(f"invalid policy: count={len(classified)} missing={missing} unknown={unknown} placeholders={placeholders}")
    payload = {"schema_version": "1", "surfaces": sorted(classified.values(), key=lambda x: (x["kind"], x["name"]))}
    OUTPUT.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
    print("validated 233 unique schema-backed entries; placeholders=0")


if __name__ == "__main__":
    main()
