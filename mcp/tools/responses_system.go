package tools

type CurrentTimeResponse struct {
	TimeMS      int64  `json:"time_ms"`
	TimeDisplay string `json:"time_display"`
}

type ServerConfigResponse struct {
	ServerType        string `json:"server_type"`
	BundleID          string `json:"bundle_id,omitempty"`
	BundleName        string `json:"bundle_name,omitempty"`
	ServiceDateFromMS int64  `json:"service_date_from_ms,omitempty"`
	ServiceDateToMS   int64  `json:"service_date_to_ms,omitempty"`
	Version           string `json:"version,omitempty"`
	CommitID          string `json:"commit_id,omitempty"`
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
