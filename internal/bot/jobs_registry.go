package bot

import (
	"context"

	"github.com/Sigumaa/yururi_personal/internal/jobs"
)

func (a *App) registerJobHandlers() {
	a.scheduler.Register("codex_release_watch", jobHandlerFunc(a.handleReleaseWatchJob))
	a.scheduler.Register("codex_background_task", jobHandlerFunc(a.handleBackgroundCodexTaskJob))
	a.scheduler.Register("codex_periodic_task", jobHandlerFunc(a.handlePeriodicCodexTaskJob))
	a.scheduler.Register("url_watch", jobHandlerFunc(a.handleURLWatchJob))
	a.scheduler.Register("daily_summary", jobHandlerFunc(a.handleDailySummaryJob))
	a.scheduler.Register("weekly_review", jobHandlerFunc(a.handleWeeklyReviewJob))
	a.scheduler.Register("monthly_review", jobHandlerFunc(a.handleMonthlyReviewJob))
	a.scheduler.Register("growth_log", jobHandlerFunc(a.handleGrowthLogJob))
	a.scheduler.Register("open_loop_review", jobHandlerFunc(a.handleOpenLoopReviewJob))
	a.scheduler.Register("curiosity_review", jobHandlerFunc(a.handleCuriosityReviewJob))
	a.scheduler.Register("initiative_review", jobHandlerFunc(a.handleInitiativeReviewJob))
	a.scheduler.Register("soft_reminder_review", jobHandlerFunc(a.handleSoftReminderReviewJob))
	a.scheduler.Register("topic_synthesis_review", jobHandlerFunc(a.handleTopicSynthesisReviewJob))
	a.scheduler.Register("baseline_review", jobHandlerFunc(a.handleBaselineReviewJob))
	a.scheduler.Register("policy_synthesis_review", jobHandlerFunc(a.handlePolicySynthesisReviewJob))
	a.scheduler.Register("workspace_review", jobHandlerFunc(a.handleWorkspaceReviewJob))
	a.scheduler.Register("proposal_boundary_review", jobHandlerFunc(a.handleProposalBoundaryReviewJob))
	a.scheduler.Register("wake_summary", jobHandlerFunc(a.handleWakeSummaryJob))
	a.scheduler.Register("autonomy_pulse", jobHandlerFunc(a.handleAutonomyPulseJob))
	a.scheduler.Register("reminder", jobHandlerFunc(a.handleReminderJob))
	a.scheduler.Register("space_review", jobHandlerFunc(a.handleSpaceReviewJob))
	a.scheduler.Register("channel_curation", jobHandlerFunc(a.handleChannelCurationJob))
	a.scheduler.Register("decision_review", jobHandlerFunc(a.handleDecisionReviewJob))
	a.scheduler.Register("self_improvement_review", jobHandlerFunc(a.handleSelfImprovementReviewJob))
	a.scheduler.Register("channel_role_review", jobHandlerFunc(a.handleChannelRoleReviewJob))
}

type jobHandlerFunc func(ctx context.Context, job jobs.Job) (jobs.Result, error)

func (f jobHandlerFunc) Run(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return f(ctx, job)
}
