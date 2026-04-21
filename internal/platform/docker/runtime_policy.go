package docker

import "slices"

const DefaultOperationalReadinessTimeoutSeconds = 300

var operationalAppServices = []string{
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

func OperationalAppServices() []string {
	return append([]string(nil), operationalAppServices...)
}

func RunningOperationalAppServices(services []string) []string {
	items := make([]string, 0, len(operationalAppServices))
	for _, service := range operationalAppServices {
		if slices.Contains(services, service) {
			items = append(items, service)
		}
	}

	return items
}

func OperationalAppServicesRunning(services []string) bool {
	return len(RunningOperationalAppServices(services)) != 0
}
