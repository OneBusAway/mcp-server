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
	resp, err := h.client.GetAgencies()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if resp.Code != 200 {
		return mcp.NewToolResultError(resp.Text), nil
	}
	if len(resp.Data.List) == 0 {
		return mcp.NewToolResultText("No agencies found."), nil
	}

	// Agency details (name, url, etc.) live in references, keyed by id
	agencyDetails := make(map[string]client.Agency, len(resp.Data.References.Agencies))
	for _, agency := range resp.Data.References.Agencies {
		agencyDetails[agency.ID] = agency
	}

	results := make([]AgencySummary, 0, len(resp.Data.List))
	for _, entry := range resp.Data.List {
		id := entry.AgencyID
		det := agencyDetails[id]
		results = append(results, AgencySummary{
			ID:   id,
			Name: det.Name, URL: det.URL, Lat: entry.Lat, Lon: entry.Lon,
		})
	}

	out, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Found %d agencies:\n%s", len(results), out)), nil
}

func (h *Handler) getAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := entityIDArgument(req, "agency_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	resp, err := h.client.GetAgency(agencyID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if resp.Code != 200 {
		return mcp.NewToolResultError(resp.Text), nil
	}
	entry := resp.Data.Entry
	if entry.ID == "" {
		return mcp.NewToolResultText(fmt.Sprintf("No agency found with ID %q.", agencyID)), nil
	}

	out, _ := json.MarshalIndent(AgencyResponse{
		ID: entry.ID, Name: entry.Name, URL: entry.URL, Phone: entry.Phone, Timezone: entry.Timezone,
	}, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("Agency %s:\n%s", agencyID, out)), nil
}
