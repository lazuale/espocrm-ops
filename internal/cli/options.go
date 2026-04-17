package cli

import "os"

type GlobalOptions struct {
	JSON       bool
	JournalDir string
}

func defaultGlobalOptions() GlobalOptions {
	return GlobalOptions{
		JournalDir: defaultJournalDir(),
	}
}

func defaultJournalDir() string {
	if v := os.Getenv("ESPOPS_JOURNAL_DIR"); v != "" {
		return v
	}

	return "/tmp/espops-journal"
}
