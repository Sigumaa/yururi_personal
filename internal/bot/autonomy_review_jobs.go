package bot

import (
	"context"
	"errors"
	"time"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func (a *App) handleOpenLoopReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "open_loop", 72*time.Hour, func(ctx context.Context) (string, bool, error) {
		loops, err := a.store.ListFacts(ctx, "open_loop", 12)
		if err != nil {
			return "", false, err
		}
		if len(loops) == 0 {
			return "", false, nil
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
		if err != nil {
			return "", false, err
		}
		return buildOpenLoopReviewPrompt(loops, recentOwnerMessages), true, nil
	})
}

func (a *App) handleCuriosityReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "curiosity_review", 24*time.Hour, func(ctx context.Context) (string, bool, error) {
		curiosities, err := a.store.ListFacts(ctx, "curiosity", 12)
		if err != nil {
			return "", false, err
		}
		openLoops, err := a.store.ListFacts(ctx, "open_loop", 8)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 12)
		if err != nil {
			return "", false, err
		}
		return buildCuriosityReviewPrompt(curiosities, openLoops, recentOwnerMessages), true, nil
	})
}

func (a *App) handleInitiativeReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "initiative_review", 48*time.Hour, func(ctx context.Context) (string, bool, error) {
		initiatives, err := a.store.ListFacts(ctx, "initiative", 12)
		if err != nil {
			return "", false, err
		}
		candidates, err := a.store.ListFacts(ctx, "automation_candidate", 12)
		if err != nil {
			return "", false, err
		}
		openLoops, err := a.store.ListFacts(ctx, "open_loop", 8)
		if err != nil {
			return "", false, err
		}
		contextGaps, err := a.store.ListFacts(ctx, "context_gap", 8)
		if err != nil {
			return "", false, err
		}
		return buildInitiativeReviewPrompt(initiatives, candidates, openLoops, contextGaps), true, nil
	})
}

func (a *App) handleSoftReminderReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "soft_reminder_review", 24*time.Hour, func(ctx context.Context) (string, bool, error) {
		reminders, err := a.store.ListFacts(ctx, "soft_reminder", 12)
		if err != nil {
			return "", false, err
		}
		routines, err := a.store.ListFacts(ctx, "routine", 8)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
		if err != nil {
			return "", false, err
		}
		return buildSoftReminderReviewPrompt(reminders, routines, recentOwnerMessages), true, nil
	})
}

func (a *App) handleTopicSynthesisReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "topic_synthesis_review", 72*time.Hour, func(ctx context.Context) (string, bool, error) {
		topics, err := a.store.ListFacts(ctx, "topic_thread", 12)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 16)
		if err != nil {
			return "", false, err
		}
		summaries, err := a.store.RecentSummaries(ctx, "weekly", 4)
		if err != nil {
			return "", false, err
		}
		return buildTopicSynthesisReviewPrompt(topics, recentOwnerMessages, summaries), true, nil
	})
}

func (a *App) handleBaselineReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "baseline_review", 72*time.Hour, func(ctx context.Context) (string, bool, error) {
		baselines, err := a.store.ListFacts(ctx, "behavior_baseline", 12)
		if err != nil {
			return "", false, err
		}
		deviations, err := a.store.ListFacts(ctx, "behavior_deviation", 12)
		if err != nil {
			return "", false, err
		}
		routines, err := a.store.ListFacts(ctx, "routine", 8)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
		if err != nil {
			return "", false, err
		}
		return buildBaselineReviewPrompt(baselines, deviations, routines, recentOwnerMessages), true, nil
	})
}

func (a *App) handlePolicySynthesisReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "policy_synthesis_review", 96*time.Hour, func(ctx context.Context) (string, bool, error) {
		learnedPolicies, err := a.store.ListFacts(ctx, "learned_policy", 16)
		if err != nil {
			return "", false, err
		}
		decisions, err := a.store.ListFacts(ctx, "decision", 12)
		if err != nil {
			return "", false, err
		}
		misfires, err := a.store.ListFacts(ctx, "misfire", 12)
		if err != nil {
			return "", false, err
		}
		reflections, err := a.store.RecentSummaries(ctx, "reflection", 8)
		if err != nil {
			return "", false, err
		}
		return buildPolicySynthesisReviewPrompt(learnedPolicies, decisions, misfires, reflections), true, nil
	})
}

func (a *App) handleWorkspaceReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "workspace_review", 48*time.Hour, func(ctx context.Context) (string, bool, error) {
		workspaceNotes, err := a.store.ListFacts(ctx, "workspace_note", 16)
		if err != nil {
			return "", false, err
		}
		initiatives, err := a.store.ListFacts(ctx, "initiative", 12)
		if err != nil {
			return "", false, err
		}
		topics, err := a.store.ListFacts(ctx, "topic_thread", 12)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 12)
		if err != nil {
			return "", false, err
		}
		return buildWorkspaceReviewPrompt(workspaceNotes, initiatives, topics, recentOwnerMessages), true, nil
	})
}

func (a *App) handleProposalBoundaryReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "proposal_boundary_review", 96*time.Hour, func(ctx context.Context) (string, bool, error) {
		proposalBoundaries, err := a.store.ListFacts(ctx, "proposal_boundary", 16)
		if err != nil {
			return "", false, err
		}
		initiatives, err := a.store.ListFacts(ctx, "initiative", 12)
		if err != nil {
			return "", false, err
		}
		decisions, err := a.store.ListFacts(ctx, "decision", 12)
		if err != nil {
			return "", false, err
		}
		misfires, err := a.store.ListFacts(ctx, "misfire", 12)
		if err != nil {
			return "", false, err
		}
		contextGaps, err := a.store.ListFacts(ctx, "context_gap", 8)
		if err != nil {
			return "", false, err
		}
		return buildProposalBoundaryReviewPrompt(proposalBoundaries, initiatives, decisions, misfires, contextGaps), true, nil
	})
}

func (a *App) handleChannelCurationJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "space", 168*time.Hour, func(ctx context.Context) (string, bool, error) {
		if a.discord == nil {
			return "", false, errors.New("discord is not connected")
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return "", false, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return "", false, err
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-14*24*time.Hour), 256)
		if err != nil {
			return "", false, err
		}
		return buildChannelCurationPrompt(channels, profiles, activity), true, nil
	})
}

func (a *App) handleDecisionReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "decision_review", 120*time.Hour, func(ctx context.Context) (string, bool, error) {
		decisions, err := a.store.ListFacts(ctx, "decision", 16)
		if err != nil {
			return "", false, err
		}
		recentOwnerMessages, err := a.store.RecentMessagesByAuthor(ctx, a.cfg.Discord.OwnerUserID, "", 10)
		if err != nil {
			return "", false, err
		}
		return buildDecisionReviewPrompt(decisions, recentOwnerMessages), true, nil
	})
}

func (a *App) handleSelfImprovementReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "self_improvement", 168*time.Hour, func(ctx context.Context) (string, bool, error) {
		candidates, err := a.store.ListFacts(ctx, "automation_candidate", 16)
		if err != nil {
			return "", false, err
		}
		reflections, err := a.store.RecentSummaries(ctx, "reflection", 8)
		if err != nil {
			return "", false, err
		}
		growth, err := a.store.RecentSummaries(ctx, "growth", 8)
		if err != nil {
			return "", false, err
		}
		return buildSelfImprovementReviewPrompt(candidates, reflections, growth), true, nil
	})
}

func (a *App) handleChannelRoleReviewJob(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return a.runReviewJob(ctx, job, "channel_role_review", 168*time.Hour, func(ctx context.Context) (string, bool, error) {
		if a.discord == nil {
			return "", false, errors.New("discord is not connected")
		}
		channels, err := a.discord.ListChannels(ctx, a.cfg.Discord.GuildID)
		if err != nil {
			return "", false, err
		}
		profiles, err := a.store.ListChannelProfiles(ctx)
		if err != nil {
			return "", false, err
		}
		activity, err := a.store.ChannelActivitySince(ctx, time.Now().UTC().Add(-14*24*time.Hour), 256)
		if err != nil {
			return "", false, err
		}
		return buildChannelRoleReviewPrompt(channels, profiles, activity), true, nil
	})
}
