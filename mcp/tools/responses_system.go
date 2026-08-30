package tools

type CurrentTimeResponse struct {
	TimeMS      int64  `json:"time_ms"`
	TimeDisplay string `json:"time_display"`
}

type MetadataResponse struct {
	StaticGTFSLastUpdatedMS int64                    `json:"static_gtfs_last_updated_ms,omitempty"`
	RealtimeFeeds           map[string]FeedFreshness `json:"realtime_feeds,omitempty"`
}

type FeedFreshness struct {
	UpdatedAtMS int64  `json:"updated_at_ms"`
	AgeSeconds  int64  `json:"age_seconds"`
	Status      string `json:"status"`
}
