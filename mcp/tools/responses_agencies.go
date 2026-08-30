package tools

// AgencySummary is the public response item returned by get_agencies.
// These types define the MCP-facing contract, independently of OBA's raw API
// response shape.
type AgencySummary struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	URL  string  `json:"url,omitempty"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// AgencyResponse is the public response returned by get_agency.
type AgencyResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}
