package backup

import "time"

var nowFunc = func() time.Time {
	return time.Now().UTC()
}

func nowUTC() time.Time {
	return nowFunc().UTC()
}

func SetNowForTest(next func() time.Time) func() {
	previous := nowFunc
	nowFunc = next
	return func() {
		nowFunc = previous
	}
}
