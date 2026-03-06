package bot

import "time"

const (
	defaultWatchSchedule         = "6h"
	defaultAutonomyPulseSchedule = "7m"
)

const (
	defaultSchedulerPollInterval = 30 * time.Second
	defaultWakeSummaryThreshold  = 4 * time.Hour
	defaultAutonomyPulseInterval = 7 * time.Minute
)
