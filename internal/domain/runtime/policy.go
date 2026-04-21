package runtime

import "slices"

const DefaultReadinessTimeoutSeconds = 300

var appServices = []string{
	"espocrm",
	"espocrm-daemon",
	"espocrm-websocket",
}

func AppServices() []string {
	return append([]string(nil), appServices...)
}

func RunningAppServices(services []string) []string {
	items := make([]string, 0, len(appServices))
	for _, service := range appServices {
		if slices.Contains(services, service) {
			items = append(items, service)
		}
	}

	return items
}

func AppServicesRunning(services []string) bool {
	return len(RunningAppServices(services)) != 0
}
