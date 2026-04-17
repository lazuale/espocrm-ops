package clock

import "time"

type Clock interface {
	Now() time.Time
}

type DefaultClock struct{}

func (DefaultClock) Now() time.Time {
	return time.Now().UTC()
}

var current Clock = DefaultClock{}

func Now() time.Time {
	return current.Now()
}

func SetForTest(next Clock) func() {
	previous := current
	current = next
	return func() {
		current = previous
	}
}
