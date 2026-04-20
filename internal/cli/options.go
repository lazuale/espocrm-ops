package cli

type GlobalOptions struct {
	JSON       bool
	JournalDir string
}

const defaultJournalDir = "/tmp/espops-journal"

func defaultGlobalOptions() GlobalOptions {
	return GlobalOptions{
		JournalDir: defaultJournalDir,
	}
}
