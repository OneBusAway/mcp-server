package tools

import (
	"fmt"
	"oba-mcp/validation"

	"github.com/mark3labs/mcp-go/mcp"
)

func entityIDArgument(req mcp.CallToolRequest, name string) (string, error) {
	value, err := req.RequireString(name)
	if err != nil {
		return "", fmt.Errorf("%s is required", name)
	}
	return validation.EntityID(name, value)
}

func optionalEntityID(req mcp.CallToolRequest, name string) (string, error) {
	value, present, err := optionalString(req, name)
	if err != nil || !present {
		return "", err
	}
	if value == "" {
		return "", nil
	}
	return validation.EntityID(name, value)
}

func optionalString(req mcp.CallToolRequest, name string) (string, bool, error) {
	value, present := req.GetArguments()[name]
	if !present {
		return "", false, nil
	}
	result, ok := value.(string)
	if !ok {
		return "", true, fmt.Errorf("%s must be a string", name)
	}
	return result, true, nil
}

func searchArgument(req mcp.CallToolRequest) (string, error) {
	value, err := req.RequireString("query")
	if err != nil {
		return "", fmt.Errorf("query is required")
	}
	return validation.Search(value)
}

func numberArgument(req mcp.CallToolRequest, name string, required bool) (float64, bool, error) {
	value, present := req.GetArguments()[name]
	if !present {
		if required {
			return 0, false, fmt.Errorf("%s is required", name)
		}
		return 0, false, nil
	}
	number, ok := value.(float64)
	if !ok {
		return 0, true, fmt.Errorf("%s must be a number", name)
	}
	return number, true, nil
}

func requiredCoordinates(req mcp.CallToolRequest) (float64, float64, error) {
	lat, _, err := numberArgument(req, "lat", true)
	if err != nil {
		return 0, 0, err
	}
	lat, err = validation.Latitude(lat)
	if err != nil {
		return 0, 0, err
	}
	lon, _, err := numberArgument(req, "lon", true)
	if err != nil {
		return 0, 0, err
	}
	lon, err = validation.Longitude(lon)
	if err != nil {
		return 0, 0, err
	}
	return lat, lon, nil
}

func optionalRadius(req mcp.CallToolRequest, fallback, maximum int) (int, error) {
	value, present, err := numberArgument(req, "radius", false)
	if err != nil {
		return 0, err
	}
	if !present {
		return fallback, nil
	}
	if value == 0 {
		return 0, fmt.Errorf("radius must be a whole number between 1 and %d meters", maximum)
	}
	return validation.RadiusMeters(value, fallback, maximum)
}

func optionalLimit(req mcp.CallToolRequest, name string, fallback, maximum int) (int, error) {
	value, present, err := numberArgument(req, name, false)
	if err != nil {
		return 0, err
	}
	if !present {
		return fallback, nil
	}
	if value == 0 {
		return 0, fmt.Errorf("%s must be a whole number between 1 and %d", name, maximum)
	}
	return validation.WholeNumber(name, value, fallback, 1, maximum)
}

func optionalWindow(req mcp.CallToolRequest, name string, fallback, maximum int) (int, error) {
	value, present, err := numberArgument(req, name, false)
	if err != nil {
		return 0, err
	}
	if !present {
		return fallback, nil
	}
	return validation.WindowMinutes(name, value, fallback, maximum)
}

func optionalTimestamp(req mcp.CallToolRequest, name string) (int64, bool, error) {
	value, present, err := numberArgument(req, name, false)
	if err != nil || !present {
		return 0, present, err
	}
	result, err := validation.TimestampMillis(name, value)
	return result, true, err
}

func requiredTimestamp(req mcp.CallToolRequest, name string) (int64, error) {
	value, _, err := numberArgument(req, name, true)
	if err != nil {
		return 0, err
	}
	return validation.TimestampMillis(name, value)
}

func optionalSpan(req mcp.CallToolRequest, name string) (float64, bool, error) {
	value, present, err := numberArgument(req, name, false)
	if err != nil || !present {
		return 0, present, err
	}
	result, err := validation.Span(name, value)
	return result, true, err
}

func optionalDate(req mcp.CallToolRequest) (string, error) {
	value, present, err := optionalString(req, "date")
	if err != nil || !present {
		return "", err
	}
	if value == "" {
		return "", nil
	}
	return validation.Date(value)
}

func optionalBool(req mcp.CallToolRequest, name string, fallback bool) (bool, error) {
	value, present := req.GetArguments()[name]
	if !present {
		return fallback, nil
	}
	result, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return result, nil
}
