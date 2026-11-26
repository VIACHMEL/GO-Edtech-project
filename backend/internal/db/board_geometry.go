package db

import (
	"encoding/json"
	"math"
	"net/url"
	"strings"
	"unicode"
)

const (
	boardCoordinateLimit = 100_000
	boardAngleLimit      = 4 * math.Pi
	boardPointLimit      = 8192
	boardLinkLimit       = 2048
)

func validBoardGeometry(element boardElement) bool {
	for _, coordinate := range []struct {
		key      string
		min, max float64
	}{
		{"x", -boardCoordinateLimit, boardCoordinateLimit},
		{"y", -boardCoordinateLimit, boardCoordinateLimit},
		{"width", 0, boardCoordinateLimit},
		{"height", 0, boardCoordinateLimit},
		{"angle", -boardAngleLimit, boardAngleLimit},
	} {
		if raw, set := element.fields[coordinate.key]; set && !boardNumberInRange(raw, coordinate.min, coordinate.max) {
			return false
		}
	}
	if raw, set := element.fields["link"]; set && !validBoardLink(raw) {
		return false
	}
	if element.Type == "text" {
		if raw, set := element.fields["text"]; set {
			var text *string
			if json.Unmarshal(raw, &text) != nil || text == nil || strings.ContainsRune(*text, '\x00') {
				return false
			}
		}
	}
	if raw, set := element.fields["points"]; set && !validBoardPoints(raw) {
		return false
	}
	if raw, set := element.fields["pressures"]; set && element.Type == "freedraw" {
		var pressures *[]*float64
		if json.Unmarshal(raw, &pressures) != nil || pressures == nil || len(*pressures) > boardPointLimit {
			return false
		}
		for _, pressure := range *pressures {
			if pressure == nil || *pressure < 0 || *pressure > 1 {
				return false
			}
		}
	}
	return true
}

func boardNumberInRange(raw json.RawMessage, min, max float64) bool {
	var number *float64
	return json.Unmarshal(raw, &number) == nil && number != nil && *number >= min && *number <= max
}

func validBoardPoints(raw json.RawMessage) bool {
	var points *[][]*float64
	if json.Unmarshal(raw, &points) != nil || points == nil || len(*points) > boardPointLimit {
		return false
	}
	for _, point := range *points {
		if len(point) != 2 || point[0] == nil || point[1] == nil ||
			math.Abs(*point[0]) > boardCoordinateLimit || math.Abs(*point[1]) > boardCoordinateLimit {
			return false
		}
	}
	return true
}

func validBoardLink(raw json.RawMessage) bool {
	var value string
	if json.Unmarshal(raw, &value) != nil || len(value) > boardLinkLimit ||
		strings.ContainsRune(value, '\\') || strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}
	link, err := url.Parse(value)
	if err != nil {
		return false
	}
	if (link.Scheme == "https" || link.Scheme == "http") && link.Hostname() != "" && link.User == nil {
		return true
	}
	return link.Scheme == "" && link.Host == "" && strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//")
}
