package journalstore

import domainjournal "github.com/lazuale/espocrm-ops/internal/domain/journal"

type Writer interface {
	Write(entry domainjournal.Entry) error
}
