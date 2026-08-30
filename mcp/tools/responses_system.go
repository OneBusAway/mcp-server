package tools

type CurrentTimeResponse struct {
	Time string `json:"time"`
}

type MetadataResponse struct {
	StaticGTFSLastUpdated string            `json:"static_gtfs_last_updated,omitempty"`
	RealtimeFeeds         map[string]string `json:"realtime_feeds,omitempty"`
}
