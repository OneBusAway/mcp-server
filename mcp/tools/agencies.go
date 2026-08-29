package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerAgencyTools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("get_agencies",
			mcp.WithDescription("List all transit agencies in this system with coverage area. Call this first when you don't know the agency_id."),
		),
		h.getAgencies,
	)

	s.AddTool(
		mcp.NewTool("get_agency",
			mcp.WithDescription("Get details for a single agency by ID: name, timezone, phone, URL."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
		),
		h.getAgency,
	)
}

func (h *Handler) getAgencies(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := h.client.Get("/api/where/agencies-with-coverage.json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	list := client.AsSlice(data["list"])
	if len(list) == 0 {
		return mcp.NewToolResultText("No agencies found."), nil
	}

	// Agency details (name, url, etc.) live in references, keyed by id
	agencyDetails := map[string]map[string]any{}
	if refs, ok := data["references"].(map[string]any); ok {
		for _, a := range client.AsSlice(refs["agencies"]) {
			agency, ok := a.(map[string]any)
			if !ok {
				continue
			}
			agencyDetails[client.StrVal(agency["id"])] = agency
		}
	}

	type result struct {
		ID   string  `json:"id"`
		Name string  `json:"name"`
		URL  string  `json:"url,omitempty"`
		Lat  float64 `json:"lat"`
		Lon  float64 `json:"lon"`
	}

	results := make([]result, 0, len(list))
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := client.StrVal(entry["agencyId"])
		det := agencyDetails[id]
		results = append(results, result{
			ID:   id,
			Name: client.StrVal(det["name"]),
			URL:  client.StrVal(det["url"]),
			Lat:  client.FloatVal(entry["lat"]),
			Lon:  client.FloatVal(entry["lon"]),
		})
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Found %d agencies:\n%s", len(results), out)), nil
}

func (h *Handler) getAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := req.RequireString("agency_id")
	if err != nil || agencyID == "" {
		return mcp.NewToolResultError("agency_id is required"), nil
	}

	resp, err := h.client.Get("/api/where/agency/"+agencyID+".json", nil)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := client.Data(resp)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	entry, _ := data["entry"].(map[string]any)
	if entry == nil {
		return mcp.NewToolResultText(fmt.Sprintf("No agency found with ID %q.", agencyID)), nil
	}

	type result struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		URL      string `json:"url,omitempty"`
		Phone    string `json:"phone,omitempty"`
		Timezone string `json:"timezone,omitempty"`
	}

	out, _ := json.MarshalIndent(result{
		ID:       client.StrVal(entry["id"]),
		Name:     client.StrVal(entry["name"]),
		URL:      client.StrVal(entry["url"]),
		Phone:    client.StrVal(entry["phone"]),
		Timezone: client.StrVal(entry["timezone"]),
	}, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Agency %s:\n%s", agencyID, out)), nil
}
