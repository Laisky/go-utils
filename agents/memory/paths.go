package memory

import (
	"fmt"
	"path"
	"time"
)

// sessionBasePath builds standardized memory directory path per session.
func sessionBasePath(sessionID string) string {
	return "/memory/" + sessionID
}

// legacyLogPath returns the legacy append-only log path for a session.
func legacyLogPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), legacyLogFileName)
}

// legacyContextPath returns the legacy runtime context path for a session.
func legacyContextPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), legacyContextFileName)
}

// legacyFactsPath returns the legacy facts file path for a session.
func legacyFactsPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), legacyMemoryFactsFileName)
}

// legacyMetaPath returns the legacy metadata path for a session.
func legacyMetaPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), legacyMetaFileName)
}

// metaStatePath returns the canonical metadata state file path.
func metaStatePath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), "meta", "state.json")
}

// metaPolicyPath returns the canonical metadata policy file path.
func metaPolicyPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), "meta", "policy.json")
}

// metaWatermarksPath returns the canonical metadata watermarks path.
func metaWatermarksPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), "meta", "watermarks.json")
}

// runtimeContextPath returns the canonical runtime context path.
func runtimeContextPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), "runtime", "context", "current.jsonl")
}

// latestCompactPointerPath returns the latest compact pointer path.
func latestCompactPointerPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), "runtime", "context", "latest_compact_pointer.json")
}

// eventsRawRootPath returns the root path for raw event logs.
func eventsRawRootPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), "events", "raw")
}

// eventsCompactRootPath returns the root path for compact summary events.
func eventsCompactRootPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), "events", "compact")
}

// eventsArchiveRootPath returns the root path for archived event shards.
func eventsArchiveRootPath(sessionID string) string {
	return path.Join(sessionBasePath(sessionID), "events", "archive")
}

// tierRootPath returns the root path for one memory tier.
func tierRootPath(sessionID, tier string) string {
	return path.Join(sessionBasePath(sessionID), "memory_tiers", tier)
}

// rawLogShardPath returns the daily shard path for append-only raw logs.
func rawLogShardPath(sessionID string, now time.Time) string {
	day := now.UTC()
	return path.Join(
		eventsRawRootPath(sessionID),
		day.Format("2006"),
		day.Format("01"),
		day.Format("02"),
		fmt.Sprintf("log-%s.jsonl", day.Format("20060102")),
	)
}

// compactShardPath returns the daily shard path for compact summary events.
func compactShardPath(sessionID string, now time.Time) string {
	day := now.UTC()
	return path.Join(
		eventsCompactRootPath(sessionID),
		day.Format("2006"),
		day.Format("01"),
		day.Format("02"),
		fmt.Sprintf("compact-%s.jsonl", day.Format("20060102")),
	)
}

// archiveShardPath returns the archive path for one raw shard.
func archiveShardPath(sessionID string, shardDate time.Time, rawName string) string {
	day := shardDate.UTC()
	return path.Join(
		eventsArchiveRootPath(sessionID),
		day.Format("2006"),
		day.Format("01"),
		day.Format("02"),
		rawName+".zst",
	)
}

// tierFactsShardPath returns a tier-specific sharded facts file path.
func tierFactsShardPath(sessionID, tier string, now time.Time) string {
	ts := now.UTC()
	switch tier {
	case memoryTierL0:
		return path.Join(
			tierRootPath(sessionID, tier),
			ts.Format("2006"),
			ts.Format("01"),
			fmt.Sprintf("facts-%s.jsonl", ts.Format("200601")),
		)
	case memoryTierL1:
		return path.Join(
			tierRootPath(sessionID, tier),
			ts.Format("2006"),
			ts.Format("01"),
			fmt.Sprintf("facts-%s.jsonl", ts.Format("20060102")),
		)
	default:
		year, week := ts.ISOWeek()
		return path.Join(
			tierRootPath(sessionID, memoryTierL2),
			fmt.Sprintf("%04d", year),
			fmt.Sprintf("%02d", week),
			fmt.Sprintf("facts-%04d-W%02d.jsonl", year, week),
		)
	}
}

// knownSummaryDirs returns all directories that should contain summary files.
func knownSummaryDirs(sessionID string) []string {
	base := sessionBasePath(sessionID)
	return []string{
		base,
		path.Join(base, "meta"),
		path.Join(base, "events"),
		path.Join(base, "events", "raw"),
		path.Join(base, "events", "compact"),
		path.Join(base, "events", "archive"),
		path.Join(base, "memory_tiers"),
		tierRootPath(sessionID, memoryTierL0),
		tierRootPath(sessionID, memoryTierL1),
		tierRootPath(sessionID, memoryTierL2),
		path.Join(base, "runtime"),
		path.Join(base, "runtime", "context"),
	}
}
