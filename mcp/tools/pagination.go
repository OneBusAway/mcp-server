package tools

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	maximumPageSize   = 50
	maximumPageOffset = 5_000
)

var paginationSigningKey = newPaginationSigningKey()

// Page is the common bounded response for a tool that enumerates resources.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type pageCursor struct {
	Tool   string `json:"tool"`
	Offset int    `json:"offset"`
}

func newPaginationSigningKey() [32]byte {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		panic("could not create pagination signing key")
	}
	return key
}

func newPaginatedTool(name string, options ...mcp.ToolOption) mcp.Tool {
	options = append(options,
		mcp.WithNumber("limit", mcp.Description("Maximum results per page: 1-50.")),
		mcp.WithString("cursor", mcp.Description("Opaque cursor from a previous response.")),
	)
	return mcp.NewTool(name, options...)
}

func pageArguments(req mcp.CallToolRequest, tool string, fallback int) (int, int, error) {
	limit, err := optionalLimit(req, "limit", fallback, maximumPageSize)
	if err != nil {
		return 0, 0, err
	}
	cursor, present, err := optionalString(req, "cursor")
	if err != nil {
		return 0, 0, err
	}
	if !present || cursor == "" {
		return 0, limit, nil
	}

	parts := strings.Split(cursor, ".")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("cursor is invalid")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("cursor is invalid")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, cursorSignature(decoded)) {
		return 0, 0, fmt.Errorf("cursor is invalid")
	}
	var value pageCursor
	if err := json.Unmarshal(decoded, &value); err != nil || value.Tool != tool || value.Offset < 0 || value.Offset > maximumPageOffset {
		return 0, 0, fmt.Errorf("cursor is invalid")
	}
	return value.Offset, limit, nil
}

func paginate[T any](tool string, values []T, offset, limit int) (Page[T], bool) {
	if offset >= len(values) {
		return Page[T]{Items: []T{}}, false
	}
	end := min(offset+limit, len(values))
	page := Page[T]{Items: values[offset:end]}
	if end == len(values) {
		return page, false
	}
	payload, _ := json.Marshal(pageCursor{Tool: tool, Offset: end})
	page.NextCursor = base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(cursorSignature(payload))
	return page, true
}

func cursorSignature(payload []byte) []byte {
	mac := hmac.New(sha256.New, paginationSigningKey[:])
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}
