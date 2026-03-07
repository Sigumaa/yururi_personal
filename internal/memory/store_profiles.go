package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (s *Store) UpsertChannelProfile(ctx context.Context, profile ChannelProfile) error {
	meta, err := json.Marshal(emptyMap(profile.Metadata))
	if err != nil {
		return fmt.Errorf("marshal profile metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO channel_profiles
		(channel_id, name, kind, reply_aggressiveness, autonomy_level, summary_cadence, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			name = excluded.name,
			kind = excluded.kind,
			reply_aggressiveness = excluded.reply_aggressiveness,
			autonomy_level = excluded.autonomy_level,
			summary_cadence = excluded.summary_cadence,
			metadata_json = excluded.metadata_json
	`, profile.ChannelID, profile.Name, profile.Kind, profile.ReplyAggressiveness, profile.AutonomyLevel, profile.SummaryCadence, string(meta))
	if err != nil {
		return fmt.Errorf("upsert channel profile: %w", err)
	}
	return nil
}

func (s *Store) GetChannelProfile(ctx context.Context, channelID string) (ChannelProfile, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT channel_id, name, kind, reply_aggressiveness, autonomy_level, summary_cadence, metadata_json
		FROM channel_profiles
		WHERE channel_id = ?
	`, channelID)
	var (
		profile ChannelProfile
		rawMeta string
	)
	err := row.Scan(&profile.ChannelID, &profile.Name, &profile.Kind, &profile.ReplyAggressiveness, &profile.AutonomyLevel, &profile.SummaryCadence, &rawMeta)
	if errorsIsNoRows(err) {
		return ChannelProfile{}, false, nil
	}
	if err != nil {
		return ChannelProfile{}, false, fmt.Errorf("get channel profile: %w", err)
	}
	_ = json.Unmarshal([]byte(rawMeta), &profile.Metadata)
	return profile, true, nil
}

func (s *Store) ListChannelProfiles(ctx context.Context) ([]ChannelProfile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT channel_id, name, kind, reply_aggressiveness, autonomy_level, summary_cadence, metadata_json
		FROM channel_profiles
		ORDER BY name ASC, channel_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list channel profiles: %w", err)
	}
	defer rows.Close()

	var out []ChannelProfile
	for rows.Next() {
		var (
			profile ChannelProfile
			rawMeta string
		)
		if err := rows.Scan(&profile.ChannelID, &profile.Name, &profile.Kind, &profile.ReplyAggressiveness, &profile.AutonomyLevel, &profile.SummaryCadence, &rawMeta); err != nil {
			return nil, fmt.Errorf("scan channel profile: %w", err)
		}
		_ = json.Unmarshal([]byte(rawMeta), &profile.Metadata)
		out = append(out, profile)
	}
	return out, rows.Err()
}

func (s *Store) ChannelActivitySince(ctx context.Context, start time.Time, limit int) ([]ChannelActivity, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT channel_id, channel_name, COUNT(*) AS message_count, MAX(created_at) AS last_message_at
		FROM raw_messages
		WHERE created_at >= ?
		GROUP BY channel_id, channel_name
		ORDER BY message_count DESC, last_message_at DESC
		LIMIT ?
	`, start.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, fmt.Errorf("channel activity since: %w", err)
	}
	defer rows.Close()

	var out []ChannelActivity
	for rows.Next() {
		var (
			activity    ChannelActivity
			lastMessage string
		)
		if err := rows.Scan(&activity.ChannelID, &activity.ChannelName, &activity.MessageCount, &lastMessage); err != nil {
			return nil, fmt.Errorf("scan channel activity: %w", err)
		}
		activity.LastMessageAt = mustParseTime(lastMessage)
		out = append(out, activity)
	}
	return out, rows.Err()
}
