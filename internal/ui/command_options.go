package ui

import "time"

// CommandOptionsChangedMsg captures command-specific view option changes.
type CommandOptionsChangedMsg struct {
	WatchInterval time.Duration
	Accept        bool
	Close         bool
	SaveDefault   bool
}
