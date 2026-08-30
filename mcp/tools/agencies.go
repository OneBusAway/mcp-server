package tools

import (
	"context"
	"fmt"
	"oba-mcp/client"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (h *Handler) registerAgencyTools(s *server.MCPServer) {
	s.AddTool(
		newPaginatedTool("get_agencies",
			mcp.WithDescription("List all transit agencies in this system with coverage area. Call this first when you don't know the agency_id."),
			mcp.WithOutputSchema[SuccessEnvelope[Page[AgencySummary]]](),
		),
		h.getAgencies,
	)

	s.AddTool(
		mcp.NewTool("get_agency",
			mcp.WithDescription("Get details for a single agency by ID: name, timezone, phone, URL."),
			mcp.WithString("agency_id", mcp.Required(), mcp.Description("Agency ID (e.g. 'unitrans')")),
			mcp.WithOutputSchema[SuccessEnvelope[AgencyResponse]](),
		),
		h.getAgency,
	)
}

func (h *Handler) getAgencies(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	offset, limit, err := pageArguments(req, "get_agencies", 20)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	resp, err := h.client.GetAgencies(ctx)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	if resp.Code != 200 {
		return toResult(errorResult("UPSTREAM_BAD_RESPONSE")), nil
	}
	if len(resp.Data.List) == 0 {
		return toResult(textResult("No agencies found.")), nil
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

	page, truncated := paginate("get_agencies", results, offset, limit)
	return toResult(withCache(dataResultWithTruncation(fmt.Sprintf("Found %d agencies:\n", len(page.Items)), page, truncated), string(resp.CacheState))), nil
}

func (h *Handler) getAgency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	agencyID, err := entityIDArgument(req, "agency_id")
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}

	resp, err := h.client.GetAgency(ctx, agencyID)
	if err != nil {
		return toResult(errorResult(err.Error())), nil
	}
	if resp.Code != 200 {
		return toResult(errorResult("UPSTREAM_BAD_RESPONSE")), nil
	}
	entry := resp.Data.Entry
	if entry.ID == "" {
		return toResult(textResult(fmt.Sprintf("No agency found with ID %q.", agencyID))), nil
	}

	return toResult(withCache(dataResult(fmt.Sprintf("Agency %s:\n", agencyID), AgencyResponse{
		ID: entry.ID, Name: entry.Name, URL: entry.URL, Phone: entry.Phone, Timezone: entry.Timezone,
	}), string(resp.CacheState))), nil
}
