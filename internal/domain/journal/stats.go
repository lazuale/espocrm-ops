package journal

type ReadStats struct {
	TotalFilesSeen int `json:"total_files_seen"`
	LoadedEntries  int `json:"loaded_entries"`
	SkippedCorrupt int `json:"skipped_corrupt"`
}

type PruneStats struct {
	DeletedFiles int `json:"deleted_files"`
	RemovedDirs  int `json:"removed_dirs"`
}

type Summary struct {
	ReadStats  ReadStats  `json:"read"`
	PruneStats PruneStats `json:"prune,omitempty"`
}
