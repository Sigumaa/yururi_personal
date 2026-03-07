package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
	"github.com/Sigumaa/yururi_personal/internal/space"
)

func (a *App) handleSpaceReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	if a.discord == nil {
		return jobs.Result{Done: true}, fmt.Errorf("discord is not connected")
	}
	if strings.TrimSpace(job.ChannelID) == "" {
		return jobs.Result{Done: true}, fmt.Errorf("space review channel is required")
	}

	sinceHours := 168
	if raw, ok := job.Payload["since_hours"]; ok {
		switch value := raw.(type) {
		case float64:
			if value > 0 {
				sinceHours = int(value)
			}
		case int:
			if value > 0 {
				sinceHours = value
			}
		}
	}

	channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
	if err != nil {
		return jobs.Result{Done: false}, err
	}
	profiles, err := a.store.ListChannelProfiles(ctx)
	if err != nil {
		return jobs.Result{Done: false}, err
	}
	activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-time.Duration(sinceHours)*time.Hour), 256)
	if err != nil {
		return jobs.Result{Done: false}, err
	}

	report := "空間整理の候補を見てきましたよ。\n" + space.DescribeSpaceCandidates(channels, profiles, activity, a.loc)
	if _, err := a.discord.SendMessage(ctx, job.ChannelID, report); err != nil {
		return jobs.Result{Done: false}, err
	}

	return jobs.Result{
		NextRunAt:       time.Now().UTC().Add(mustDuration(job.ScheduleExpr, 24*time.Hour)),
		Done:            false,
		Details:         "space review sent",
		AlreadyNotified: true,
	}, nil
}
