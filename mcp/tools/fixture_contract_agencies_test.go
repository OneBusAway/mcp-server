package tools

import "testing"

func TestGetAgenciesContract(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/agencies-with-coverage.json": `{"code":200,"data":{
			"list":[{"agencyId":"test","lat":27.9488,"lon":-82.4582}],
			"references":{"agencies":[{"id":"test","name":"Test Agency","url":"https://example.com","timezone":"America/Los_Angeles"}]}
		}}`,
	})

	result := invokeHandler(t, handler.getAgencies, map[string]any{})
	page := dataAs[Page[AgencySummary]](t, result)
	if len(page.Items) != 1 {
		t.Fatalf("agencies = %#v, want one fixture agency", page.Items)
	}
	agency := page.Items[0]
	if agency.ID != "test" || agency.Name != "Test Agency" || agency.URL != "https://example.com" {
		t.Fatalf("agency identity/name/url mapping wrong: %#v", agency)
	}
	if agency.Lat != 27.9488 || agency.Lon != -82.4582 {
		t.Fatalf("agency coordinates mapping wrong: %#v", agency)
	}
	if got := meta(t, result).Cache; got != "miss" {
		t.Fatalf("cache = %q, want miss", got)
	}
}

// TestGetAgenciesEmptyReturnsStructuredPage pins the empty-list contract for
// get_agencies so a zero-agency system yields a typed empty page.
func TestGetAgenciesEmptyReturnsStructuredPage(t *testing.T) {
	handler, _ := fixtureHandler(t, map[string]string{
		"/api/where/agencies-with-coverage.json": envelopeList(`[]`),
	})
	result := invokeHandler(t, handler.getAgencies, map[string]any{})
	page := dataAs[Page[AgencySummary]](t, result)
	if page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("empty get_agencies page.Items = %#v, want empty non-nil slice", page.Items)
	}
}

func TestGetAgencyContract(t *testing.T) {
	handler, upstream := fixtureHandler(t, map[string]string{
		"/api/where/agency/test.json": envelopeEntry(`{"id":"test","name":"Test Agency","url":"https://example.com","phone":"555-0100","timezone":"America/Los_Angeles"}`),
	})

	result := invokeHandler(t, handler.getAgency, map[string]any{"agency_id": "test"})
	agency := dataAs[AgencyResponse](t, result)
	if agency.ID != "test" || agency.Name != "Test Agency" || agency.URL != "https://example.com" {
		t.Fatalf("agency identity mapping wrong: %#v", agency)
	}
	if agency.Phone != "555-0100" || agency.Timezone != "America/Los_Angeles" {
		t.Fatalf("agency phone/timezone mapping wrong: %#v", agency)
	}
	assertRequestPath(t, upstream, "/api/where/agency/test.json", nil)
}
