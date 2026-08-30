package tools

import "testing"

func TestPaginate(t *testing.T) {
	values := []string{"a", "b", "c"}
	page, truncated := paginate("search_stops", values, 0, 2)
	if !truncated || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatalf("first page = %#v, truncated = %t", page, truncated)
	}

	offset, limit, err := pageArguments(toolRequest(map[string]any{"cursor": page.NextCursor}), "search_stops", 10)
	if err != nil {
		t.Fatalf("pageArguments returned error: %v", err)
	}
	if offset != 2 || limit != 10 {
		t.Fatalf("offset/limit = %d/%d, want 2/10", offset, limit)
	}

	if _, _, err := pageArguments(toolRequest(map[string]any{"cursor": page.NextCursor}), "search_routes", 10); err == nil {
		t.Fatal("cursor for a different tool was accepted")
	}
	if _, _, err := pageArguments(toolRequest(map[string]any{"cursor": page.NextCursor + "x"}), "search_stops", 10); err == nil {
		t.Fatal("tampered cursor was accepted")
	}
}
